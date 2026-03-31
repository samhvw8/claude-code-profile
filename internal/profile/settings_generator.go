package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

// GenerateSettingsHooks generates the hooks section for settings.json from linked hub hooks
// Uses $HOME-based absolute paths for portability
func GenerateSettingsHooks(paths *config.Paths, profileDir string, manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error) {
	hooks := make(map[config.HookType][]config.SettingsHookEntry)
	profileHooksDir := filepath.Join(profileDir, "hooks")

	for _, hookName := range manifest.Hub.Hooks {
		hookDir := filepath.Join(profileHooksDir, hookName)

		// Try hooks.json first (official format)
		hooksJSON, err := hub.GetHooksJSON(hookDir)
		if err == nil && hooksJSON != nil {
			processHooksJSON(hooksJSON, hookDir, hooks)
			continue
		}

		// Fall back to hook.yaml (legacy format via GetHookManifest)
		hookManifest, err := hub.GetHookManifest(paths.HubDir, hookName)
		if err != nil {
			// Skip hooks that don't have a manifest
			continue
		}

		processLegacyHook(hookManifest, profileHooksDir, hookName, hooks)
	}

	return hooks, nil
}

// processHooksJSON processes hooks.json format entries
func processHooksJSON(hooksJSON *config.HooksJSON, hookDir string, hooks map[config.HookType][]config.SettingsHookEntry) {
	for hookType, entries := range hooksJSON.Hooks {
		for _, hookEntry := range entries {
			for _, cmd := range hookEntry.Hooks {
				command := resolvePluginRootPath(cmd.Command, hookDir)
				timeout := cmd.Timeout
				if timeout == 0 {
					timeout = config.DefaultHookTimeout()
				}

				entry := config.NewSettingsHookEntry(hookEntry.Matcher, command, timeout)
				// Preserve the original type if specified
				if cmd.Type != "" {
					entry.Hooks[0].Type = cmd.Type
				}
				hooks[hookType] = append(hooks[hookType], entry)
			}
		}
	}
}

// processLegacyHook processes legacy hook.yaml format
func processLegacyHook(hookManifest *hub.HookManifest, profileHooksDir, hookName string, hooks map[config.HookType][]config.SettingsHookEntry) {
	command := buildLegacyCommand(hookManifest, profileHooksDir, hookName)

	// Prepend interpreter if specified
	if hookManifest.Interpreter != "" && hookManifest.Inline == "" {
		command = hookManifest.Interpreter + " " + command
	}

	timeout := hookManifest.Timeout
	if timeout == 0 {
		timeout = config.DefaultHookTimeout()
	}

	entry := config.NewSettingsHookEntry(hookManifest.Matcher, command, timeout)
	hookType := hookManifest.Type
	hooks[hookType] = append(hooks[hookType], entry)
}

// resolvePluginRootPath replaces ${CLAUDE_PLUGIN_ROOT} with the portable hook directory path
func resolvePluginRootPath(command, hookDir string) string {
	if !strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}") {
		return command
	}
	portablePath := config.ToPortablePath(hookDir)
	return strings.ReplaceAll(command, "${CLAUDE_PLUGIN_ROOT}", portablePath)
}

// buildLegacyCommand builds the command string for legacy hook.yaml format
func buildLegacyCommand(hookManifest *hub.HookManifest, profileHooksDir, hookName string) string {
	if hookManifest.Inline != "" {
		return hookManifest.Inline
	}
	if len(hookManifest.Command) > 0 && hookManifest.Command[0] == '/' {
		// Absolute path - use as-is
		return hookManifest.Command
	}
	// Build portable path using profile's hooks directory
	absPath := filepath.Join(profileHooksDir, hookName, hookManifest.Command)
	return config.ToPortablePath(absPath)
}

// RegenerateSettings regenerates settings.json with updated hook paths and settings template
func RegenerateSettings(paths *config.Paths, profileDir string, manifest *Manifest) error {
	settingsPath := filepath.Join(profileDir, "settings.json")

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		return err
	}

	// Write back with pretty printing (no HTML escaping)
	return writeJSONFile(settingsPath, settings)
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
