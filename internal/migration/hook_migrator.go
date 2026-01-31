package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/symlink"
)

// HookMigrator handles the migration of hooks to the hub
type HookMigrator struct {
	paths    *config.Paths
	symMgr   *symlink.Manager
	rollback *Rollback
}

// NewHookMigrator creates a new hook migrator
func NewHookMigrator(paths *config.Paths, rollback *Rollback) *HookMigrator {
	return &HookMigrator{
		paths:    paths,
		symMgr:   symlink.New(),
		rollback: rollback,
	}
}

// MigratedHook represents a successfully migrated hook
type MigratedHook struct {
	Name         string           // Hook folder name in hub
	HubPath      string           // Full path to hook folder in hub
	OriginalPath string           // Original file path
	HooksJSON    *config.HooksJSON // The hooks.json manifest (official format)
	HookType     config.HookType  // Hook event type
}

// MigrateHooks migrates all hooks according to the plan
func (m *HookMigrator) MigrateHooks(plan *HookMigrationPlan, profileDir string) ([]MigratedHook, error) {
	var migrated []MigratedHook

	hooksHubDir := m.paths.HubItemDir(config.HubHooks)
	if err := os.MkdirAll(hooksHubDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create hooks hub dir: %w", err)
	}

	// Track used names to avoid conflicts
	usedNames := make(map[string]int)

	// Migrate inside hooks (always migrate)
	for _, hook := range plan.Inside {
		migratedHook, err := m.migrateHook(hook, usedNames, hooksHubDir)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate inside hook: %w", err)
		}
		migrated = append(migrated, *migratedHook)
	}

	// Migrate inline hooks (extract to hub)
	for _, hook := range plan.Inline {
		migratedHook, err := m.migrateInlineHook(hook, usedNames, hooksHubDir)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate inline hook: %w", err)
		}
		migrated = append(migrated, *migratedHook)
	}

	// Migrate outside hooks based on user decisions
	for _, decision := range plan.Decisions {
		switch decision.Choice {
		case HookChoiceCopy:
			migratedHook, err := m.migrateHook(decision.Hook, usedNames, hooksHubDir)
			if err != nil {
				return nil, fmt.Errorf("failed to migrate outside hook: %w", err)
			}
			migrated = append(migrated, *migratedHook)
		case HookChoiceSkip:
			// Do nothing - hook will be removed from settings
		case HookChoiceKeep:
			// Create a manifest that points to external file
			migratedHook, err := m.migrateExternalHook(decision.Hook, usedNames, hooksHubDir)
			if err != nil {
				return nil, fmt.Errorf("failed to create external hook reference: %w", err)
			}
			migrated = append(migrated, *migratedHook)
		}
	}

	// Create symlinks in profile hooks directory
	if err := m.createProfileSymlinks(migrated, profileDir); err != nil {
		return nil, err
	}

	return migrated, nil
}

// migrateHook migrates a single hook (inside or copied outside) to the hub
func (m *HookMigrator) migrateHook(hook ClassifiedHook, usedNames map[string]int, hooksHubDir string) (*MigratedHook, error) {
	name := m.uniqueName(GenerateHookName(hook.ExtractedHook), usedNames)
	hookDir := filepath.Join(hooksHubDir, name)

	// Create hook directory and scripts subdirectory
	scriptsDir := filepath.Join(hookDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return nil, err
	}
	m.rollback.AddDir(hookDir)

	// Determine the script file name
	scriptName := filepath.Base(hook.FilePath)
	srcPath := expandHome(hook.FilePath)
	dstPath := filepath.Join(scriptsDir, scriptName)

	// Copy the script file (or move if inside)
	if hook.Location == HookLocationInside {
		if err := moveItem(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("failed to move hook file: %w", err)
		}
		m.rollback.AddMove(dstPath, srcPath)
	} else {
		if err := copyRecursive(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("failed to copy hook file: %w", err)
		}
	}

	// Build command path using CLAUDE_PLUGIN_ROOT for portability
	commandPath := "${CLAUDE_PLUGIN_ROOT}/scripts/" + scriptName
	if hook.Interpreter != "" {
		commandPath = hook.Interpreter + " " + commandPath
	}

	// Create hooks.json manifest (official format)
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	hooksJSON := config.NewHooksJSON()
	hooksJSON.AddHook(hook.HookType, hook.Matcher, commandPath, timeout)

	if err := m.saveHooksJSON(hooksJSON, hookDir); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: hook.FilePath,
		HooksJSON:    hooksJSON,
		HookType:     hook.HookType,
	}, nil
}

// migrateInlineHook creates a hook from an inline script
func (m *HookMigrator) migrateInlineHook(hook ClassifiedHook, usedNames map[string]int, hooksHubDir string) (*MigratedHook, error) {
	name := m.uniqueName(GenerateHookName(hook.ExtractedHook), usedNames)
	hookDir := filepath.Join(hooksHubDir, name)

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return nil, err
	}
	m.rollback.AddDir(hookDir)

	// Create hooks.json manifest with inline command
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	hooksJSON := config.NewHooksJSON()
	hooksJSON.AddHook(hook.HookType, hook.Matcher, hook.Command, timeout)

	if err := m.saveHooksJSON(hooksJSON, hookDir); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: "",
		HooksJSON:    hooksJSON,
		HookType:     hook.HookType,
	}, nil
}

// migrateExternalHook creates a manifest that references an external file
func (m *HookMigrator) migrateExternalHook(hook ClassifiedHook, usedNames map[string]int, hooksHubDir string) (*MigratedHook, error) {
	name := m.uniqueName(GenerateHookName(hook.ExtractedHook), usedNames)
	hookDir := filepath.Join(hooksHubDir, name)

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return nil, err
	}
	m.rollback.AddDir(hookDir)

	// Create hooks.json manifest with absolute path to external file
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	hooksJSON := config.NewHooksJSON()
	hooksJSON.AddHook(hook.HookType, hook.Matcher, expandHome(hook.FilePath), timeout)

	if err := m.saveHooksJSON(hooksJSON, hookDir); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: hook.FilePath,
		HooksJSON:    hooksJSON,
		HookType:     hook.HookType,
	}, nil
}

// createProfileSymlinks creates symlinks in the profile hooks directory
func (m *HookMigrator) createProfileSymlinks(migrated []MigratedHook, profileDir string) error {
	profileHooksDir := filepath.Join(profileDir, string(config.HubHooks))
	if err := os.MkdirAll(profileHooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create profile hooks dir: %w", err)
	}

	for _, hook := range migrated {
		linkPath := filepath.Join(profileHooksDir, hook.Name)
		if err := m.symMgr.Create(linkPath, hook.HubPath); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", hook.Name, err)
		}
	}

	return nil
}

// saveHooksJSON saves a hooks.json file to the specified hook directory
func (m *HookMigrator) saveHooksJSON(hooksJSON *config.HooksJSON, hookDir string) error {
	hooksPath := filepath.Join(hookDir, "hooks.json")
	data, err := json.MarshalIndent(hooksJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks.json: %w", err)
	}

	return os.WriteFile(hooksPath, data, 0644)
}

// uniqueName generates a unique name by appending a suffix if needed
func (m *HookMigrator) uniqueName(base string, used map[string]int) string {
	name := base
	if count, exists := used[base]; exists {
		name = fmt.Sprintf("%s-%d", base, count+1)
	}
	used[base]++
	return name
}

// moveItem is a helper that wraps the package-level moveItem function
func moveItem(src, dst string) error {
	// Try rename first (fastest, works on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + remove
	if err := copyRecursive(src, dst); err != nil {
		return err
	}

	return os.RemoveAll(src)
}

// BuildSettingsCommandFromHooksJSON builds the command string for settings.json from HooksJSON
// Replaces ${CLAUDE_PLUGIN_ROOT} with actual path for settings.json
func BuildSettingsCommandFromHooksJSON(command string, hookDir string) string {
	// If command uses CLAUDE_PLUGIN_ROOT, replace with actual path
	if strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}") {
		home, _ := os.UserHomeDir()
		absPath := hookDir

		// Replace home directory with $HOME for portability
		if home != "" && strings.HasPrefix(absPath, home) {
			absPath = "$HOME" + absPath[len(home):]
		}

		return strings.ReplaceAll(command, "${CLAUDE_PLUGIN_ROOT}", absPath)
	}

	// If command is absolute path (external reference), use as-is
	if strings.HasPrefix(command, "/") {
		return command
	}

	// Otherwise it's a relative command or inline
	return command
}

// BuildSettingsCommand builds the command string for settings.json (legacy HookManifest support)
// Uses $HOME-based absolute path for portability across machines
// Works with both ~/.claude symlink and direct CLAUDE_CONFIG_DIR usage
func BuildSettingsCommand(manifest *HookManifest, profileHooksDir string) string {
	if manifest.Inline != "" {
		return manifest.Inline
	}

	var absPath string

	// If command is absolute path (external reference), use as-is
	if strings.HasPrefix(manifest.Command, "/") {
		absPath = manifest.Command
	} else {
		// Build absolute path using the profile's hooks directory
		// Replace home dir with $HOME for portability
		home, _ := os.UserHomeDir()
		absPath = filepath.Join(profileHooksDir, manifest.Name, manifest.Command)

		// Replace home directory with $HOME for portability
		if home != "" && strings.HasPrefix(absPath, home) {
			absPath = "$HOME" + absPath[len(home):]
		}
	}

	// Prepend interpreter if specified
	if manifest.Interpreter != "" {
		return manifest.Interpreter + " " + absPath
	}

	return absPath
}
