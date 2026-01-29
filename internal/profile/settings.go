package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// SettingsManager handles settings.json synchronization
type SettingsManager struct {
	paths *config.Paths
}

// NewSettingsManager creates a new settings manager
func NewSettingsManager(paths *config.Paths) *SettingsManager {
	return &SettingsManager{paths: paths}
}

// Settings represents the settings.json structure
type Settings struct {
	Hooks           map[string][]HookEntry `json:"hooks,omitempty"`
	OtherSettings   map[string]interface{} `json:"-"` // Preserve other settings
	rawData         map[string]interface{}
}

// HookEntry represents a hook configuration in settings.json
type HookEntry struct {
	Hooks   []HookCommand `json:"hooks"`
	Matcher string        `json:"matcher,omitempty"`
}

// HookCommand represents the actual command in a hook
type HookCommand struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
	Type    string `json:"type"`
}

// LoadSettings loads settings.json from a profile directory
func (sm *SettingsManager) LoadSettings(profileDir string) (*Settings, error) {
	settingsPath := filepath.Join(profileDir, "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Settings{
				Hooks:   make(map[string][]HookEntry),
				rawData: make(map[string]interface{}),
			}, nil
		}
		return nil, err
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, err
	}

	settings := &Settings{
		Hooks:   make(map[string][]HookEntry),
		rawData: rawData,
	}

	// Parse hooks if present
	if hooksData, ok := rawData["hooks"].(map[string]interface{}); ok {
		for hookType, entries := range hooksData {
			if entriesArr, ok := entries.([]interface{}); ok {
				for _, entry := range entriesArr {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						hookEntry := HookEntry{}
						if matcher, ok := entryMap["matcher"].(string); ok {
							hookEntry.Matcher = matcher
						}
						if hooksArr, ok := entryMap["hooks"].([]interface{}); ok {
							for _, h := range hooksArr {
								if hMap, ok := h.(map[string]interface{}); ok {
									cmd := HookCommand{Type: "command"}
									if c, ok := hMap["command"].(string); ok {
										cmd.Command = c
									}
									if t, ok := hMap["timeout"].(float64); ok {
										cmd.Timeout = int(t)
									}
									if tp, ok := hMap["type"].(string); ok {
										cmd.Type = tp
									}
									hookEntry.Hooks = append(hookEntry.Hooks, cmd)
								}
							}
						}
						settings.Hooks[hookType] = append(settings.Hooks[hookType], hookEntry)
					}
				}
			}
		}
	}

	return settings, nil
}

// SaveSettings saves settings.json to a profile directory
func (sm *SettingsManager) SaveSettings(profileDir string, settings *Settings) error {
	settingsPath := filepath.Join(profileDir, "settings.json")

	// Start with raw data to preserve unknown fields
	output := settings.rawData
	if output == nil {
		output = make(map[string]interface{})
	}

	// Convert hooks to JSON structure
	if len(settings.Hooks) > 0 {
		hooksData := make(map[string]interface{})
		for hookType, entries := range settings.Hooks {
			var entriesData []interface{}
			for _, entry := range entries {
				entryData := make(map[string]interface{})
				if entry.Matcher != "" {
					entryData["matcher"] = entry.Matcher
				}
				var hooksArr []interface{}
				for _, h := range entry.Hooks {
					hookData := map[string]interface{}{
						"command": h.Command,
						"type":    h.Type,
					}
					if h.Timeout > 0 {
						hookData["timeout"] = h.Timeout
					}
					hooksArr = append(hooksArr, hookData)
				}
				entryData["hooks"] = hooksArr
				entriesData = append(entriesData, entryData)
			}
			hooksData[hookType] = entriesData
		}
		output["hooks"] = hooksData
	}

	return writeJSONFile(settingsPath, output)
}

// SyncHooksFromManifest updates settings.json hooks based on manifest
func (sm *SettingsManager) SyncHooksFromManifest(profileDir string, manifest *Manifest) error {
	settings, err := sm.LoadSettings(profileDir)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	// Clear existing hooks that were managed by ccp
	// We'll rebuild from manifest
	settings.Hooks = make(map[string][]HookEntry)

	// Add hooks from manifest
	for _, hookCfg := range manifest.Hooks {
		hookType := string(hookCfg.Type)

		// Determine command
		command := hookCfg.Command
		if command == "" {
			// Default to running the hook file
			hookPath := filepath.Join(profileDir, "hooks", hookCfg.Name)
			command = fmt.Sprintf("bash %s", hookPath)
		}

		// Determine timeout
		timeout := hookCfg.Timeout
		if timeout == 0 {
			timeout = config.DefaultHookTimeout()
		}

		entry := HookEntry{
			Hooks: []HookCommand{
				{
					Command: command,
					Timeout: timeout,
					Type:    "command",
				},
			},
			Matcher: hookCfg.Matcher,
		}

		settings.Hooks[hookType] = append(settings.Hooks[hookType], entry)
	}

	return sm.SaveSettings(profileDir, settings)
}

// GetHookConfig returns the hook configuration for a given hook name from manifest
func (m *Manifest) GetHookConfig(name string) *config.HookConfig {
	for i := range m.Hooks {
		if m.Hooks[i].Name == name {
			return &m.Hooks[i]
		}
	}
	return nil
}

// SetHookConfig adds or updates a hook configuration
func (m *Manifest) SetHookConfig(cfg config.HookConfig) {
	for i := range m.Hooks {
		if m.Hooks[i].Name == cfg.Name {
			m.Hooks[i] = cfg
			return
		}
	}
	m.Hooks = append(m.Hooks, cfg)
}

// RemoveHookConfig removes a hook configuration
func (m *Manifest) RemoveHookConfig(name string) bool {
	for i := range m.Hooks {
		if m.Hooks[i].Name == name {
			m.Hooks = append(m.Hooks[:i], m.Hooks[i+1:]...)
			return true
		}
	}
	return false
}
