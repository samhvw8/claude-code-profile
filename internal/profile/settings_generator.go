package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// GenerateSettingsHooks generates the hooks section for settings.json from linked hub hooks
// Uses $HOME-based absolute paths for portability
func GenerateSettingsHooks(paths *config.Paths, profileDir string, manifest *Manifest) (map[string][]map[string]interface{}, error) {
	hooks := make(map[string][]map[string]interface{})
	home, _ := os.UserHomeDir()
	profileHooksDir := filepath.Join(profileDir, "hooks")

	for _, hookName := range manifest.Hub.Hooks {
		hookManifest, err := hub.GetHookManifest(paths.HubDir, hookName)
		if err != nil {
			// Skip hooks that don't have a manifest
			continue
		}

		// Build the command path
		var command string
		if hookManifest.Inline != "" {
			command = hookManifest.Inline
		} else if len(hookManifest.Command) > 0 && hookManifest.Command[0] == '/' {
			// Absolute path - use as-is
			command = hookManifest.Command
		} else {
			// Build absolute path using profile's hooks directory
			absPath := filepath.Join(profileHooksDir, hookName, hookManifest.Command)
			// Replace home dir with $HOME for portability
			if home != "" && strings.HasPrefix(absPath, home) {
				command = "$HOME" + absPath[len(home):]
			} else {
				command = absPath
			}
		}

		// Prepend interpreter if specified
		if hookManifest.Interpreter != "" && hookManifest.Inline == "" {
			command = hookManifest.Interpreter + " " + command
		}

		timeout := hookManifest.Timeout
		if timeout == 0 {
			timeout = config.DefaultHookTimeout()
		}

		entry := map[string]interface{}{
			"hooks": []map[string]interface{}{{
				"command": command,
				"timeout": timeout,
				"type":    "command",
			}},
		}
		if hookManifest.Matcher != "" {
			entry["matcher"] = hookManifest.Matcher
		}

		hookType := string(hookManifest.Type)
		hooks[hookType] = append(hooks[hookType], entry)
	}

	return hooks, nil
}

// RegenerateSettings regenerates settings.json with updated hook paths and setting fragments
// Note: This rebuilds settings from fragments, removing keys that are no longer in any fragment
func RegenerateSettings(paths *config.Paths, profileDir string, manifest *Manifest) error {
	settingsPath := filepath.Join(profileDir, "settings.json")

	// Start with empty settings - we'll build from fragments
	settings := make(map[string]interface{})

	// Read existing settings to preserve non-fragment keys (like hooks from previous runs)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		var existingSettings map[string]interface{}
		if err := json.Unmarshal(data, &existingSettings); err == nil {
			// Only preserve hooks - fragments are the source of truth for other keys
			if hooks, ok := existingSettings["hooks"]; ok {
				settings["hooks"] = hooks
			}
		}
	}

	// Merge setting fragments from hub - these define which keys should exist
	if len(manifest.Hub.SettingFragments) > 0 {
		fragmentSettings, err := mergeSettingFragments(paths.HubDir, manifest.Hub.SettingFragments)
		if err != nil {
			return err
		}
		for key, value := range fragmentSettings {
			settings[key] = value
		}
	}

	// Generate hooks section
	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		return err
	}

	// Only update hooks if there are any
	if len(hooks) > 0 {
		settings["hooks"] = hooks
	}

	// Write back with pretty printing (no HTML escaping)
	return writeJSONFile(settingsPath, settings)
}

// settingFragment represents a single setting fragment (local copy to avoid import cycle)
type settingFragment struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Key         string      `yaml:"key"`
	Value       interface{} `yaml:"value"`
}

// mergeSettingFragments merges multiple fragments into a settings map
func mergeSettingFragments(hubDir string, fragmentNames []string) (map[string]interface{}, error) {
	settings := make(map[string]interface{})

	for _, name := range fragmentNames {
		fragmentPath := filepath.Join(hubDir, string(config.HubSettingFragments), name+".yaml")

		data, err := os.ReadFile(fragmentPath)
		if err != nil {
			return nil, err
		}

		var fragment settingFragment
		if err := yaml.Unmarshal(data, &fragment); err != nil {
			return nil, err
		}

		settings[fragment.Key] = fragment.Value
	}

	return settings, nil
}

// writeJSONFile writes data as JSON without HTML escaping
func writeJSONFile(path string, data interface{}) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}
