package migration

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// HookFormatMigrator handles migration of hook.yaml to hooks.json format
type HookFormatMigrator struct {
	paths *config.Paths
}

// NewHookFormatMigrator creates a new hook format migrator
func NewHookFormatMigrator(paths *config.Paths) *HookFormatMigrator {
	return &HookFormatMigrator{paths: paths}
}

// NeedsMigration checks if any hooks need format migration
func (m *HookFormatMigrator) NeedsMigration() bool {
	hooksDir := filepath.Join(m.paths.HubDir, string(config.HubHooks))

	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		hookDir := filepath.Join(hooksDir, entry.Name())

		// Check if hook.yaml exists but hooks.json doesn't
		yamlPath := filepath.Join(hookDir, "hook.yaml")
		jsonPath := filepath.Join(hookDir, "hooks.json")

		if _, err := os.Stat(yamlPath); err == nil {
			if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
				return true
			}
		}
	}

	return false
}

// MigrateHookFormats converts all hook.yaml files to hooks.json format
func (m *HookFormatMigrator) MigrateHookFormats() (int, error) {
	count := 0
	hooksDir := filepath.Join(m.paths.HubDir, string(config.HubHooks))

	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		hookDir := filepath.Join(hooksDir, entry.Name())
		migrated, err := m.migrateHookDir(hookDir, entry.Name())
		if err != nil {
			return count, err
		}
		if migrated {
			count++
		}
	}

	return count, nil
}

// migrateHookDir converts a single hook from hook.yaml to hooks.json
func (m *HookFormatMigrator) migrateHookDir(hookDir, _ string) (bool, error) {
	yamlPath := filepath.Join(hookDir, "hook.yaml")
	jsonPath := filepath.Join(hookDir, "hooks.json")

	// Skip if hooks.json already exists
	if _, err := os.Stat(jsonPath); err == nil {
		return false, nil
	}

	// Skip if hook.yaml doesn't exist
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Parse hook.yaml
	var manifest hub.HookManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return false, err
	}

	// Build command path
	command := manifest.Command
	if manifest.Inline != "" {
		command = manifest.Inline
	} else if command != "" && !strings.HasPrefix(command, "/") && !strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}") {
		// Convert relative path to use CLAUDE_PLUGIN_ROOT
		// Move script to scripts/ subdirectory if needed
		scriptsDir := filepath.Join(hookDir, "scripts")
		oldScriptPath := filepath.Join(hookDir, manifest.Command)
		newScriptPath := filepath.Join(scriptsDir, manifest.Command)

		// Check if script exists at old location
		if _, err := os.Stat(oldScriptPath); err == nil {
			// Create scripts directory and move file
			if err := os.MkdirAll(scriptsDir, 0755); err != nil {
				return false, err
			}
			if err := os.Rename(oldScriptPath, newScriptPath); err != nil {
				return false, err
			}
		}

		command = "${CLAUDE_PLUGIN_ROOT}/scripts/" + manifest.Command
		if manifest.Interpreter != "" {
			command = manifest.Interpreter + " " + command
		}
	}

	// Create hooks.json
	timeout := manifest.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	hooksJSON := config.NewHooksJSON()
	hooksJSON.AddHook(manifest.Type, manifest.Matcher, command, timeout)

	// Save hooks.json
	if err := hub.SaveHooksJSON(hookDir, hooksJSON); err != nil {
		return false, err
	}

	// Remove old hook.yaml after successful migration
	if err := os.Remove(yamlPath); err != nil {
		// Non-fatal - hooks.json was created successfully
	}

	return true, nil
}
