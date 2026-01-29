package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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
	Name         string          // Hook folder name in hub
	HubPath      string          // Full path to hook folder in hub
	OriginalPath string          // Original file path
	Manifest     *HookManifest   // The hook.yaml manifest
	HookType     config.HookType // Hook event type
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

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return nil, err
	}
	m.rollback.AddDir(hookDir)

	// Determine the script file name
	scriptName := filepath.Base(hook.FilePath)
	srcPath := expandHome(hook.FilePath)
	dstPath := filepath.Join(hookDir, scriptName)

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

	// Create hook.yaml manifest
	manifest := &HookManifest{
		Name:        name,
		Type:        hook.HookType,
		Timeout:     hook.Timeout,
		Command:     scriptName, // Relative to hook folder
		Interpreter: hook.Interpreter,
		Matcher:     hook.Matcher,
	}

	if manifest.Timeout == 0 {
		manifest.Timeout = config.DefaultHookTimeout()
	}

	manifestPath := filepath.Join(hookDir, "hook.yaml")
	if err := m.saveManifest(manifest, manifestPath); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: hook.FilePath,
		Manifest:     manifest,
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

	// Create hook.yaml manifest with inline command
	manifest := &HookManifest{
		Name:    name,
		Type:    hook.HookType,
		Timeout: hook.Timeout,
		Inline:  hook.Command, // Store the inline command
		Matcher: hook.Matcher,
	}

	if manifest.Timeout == 0 {
		manifest.Timeout = config.DefaultHookTimeout()
	}

	manifestPath := filepath.Join(hookDir, "hook.yaml")
	if err := m.saveManifest(manifest, manifestPath); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: "",
		Manifest:     manifest,
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

	// Create hook.yaml manifest with absolute path
	manifest := &HookManifest{
		Name:    name,
		Type:    hook.HookType,
		Timeout: hook.Timeout,
		Command: expandHome(hook.FilePath), // Keep absolute path
		Matcher: hook.Matcher,
	}

	if manifest.Timeout == 0 {
		manifest.Timeout = config.DefaultHookTimeout()
	}

	manifestPath := filepath.Join(hookDir, "hook.yaml")
	if err := m.saveManifest(manifest, manifestPath); err != nil {
		return nil, err
	}

	return &MigratedHook{
		Name:         name,
		HubPath:      hookDir,
		OriginalPath: hook.FilePath,
		Manifest:     manifest,
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

// saveManifest saves a hook manifest to the given path
func (m *HookMigrator) saveManifest(manifest *HookManifest, path string) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal hook manifest: %w", err)
	}

	return os.WriteFile(path, data, 0644)
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

// BuildSettingsCommand builds the command string for settings.json
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
