package hub

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
)

// Hub represents the central hub of reusable components
type Hub struct {
	Path  string
	Items map[config.HubItemType][]Item
}

// Item represents a single hub item (skill, hook, rule, etc.)
type Item struct {
	Name   string
	Type   config.HubItemType
	Path   string
	IsDir  bool
	Source *SourceManifest // nil if no source.yaml
}

// New creates a new Hub instance
func New(path string) *Hub {
	return &Hub{
		Path:  path,
		Items: make(map[config.HubItemType][]Item),
	}
}

// GetItems returns items of a specific type
func (h *Hub) GetItems(itemType config.HubItemType) []Item {
	return h.Items[itemType]
}

// GetItem returns a specific item by type and name
func (h *Hub) GetItem(itemType config.HubItemType, name string) *Item {
	for _, item := range h.Items[itemType] {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

// HasItem checks if an item exists in the hub
func (h *Hub) HasItem(itemType config.HubItemType, name string) bool {
	return h.GetItem(itemType, name) != nil
}

// AllItems returns all items as a flat slice
func (h *Hub) AllItems() []Item {
	var all []Item
	for _, itemType := range config.AllHubItemTypes() {
		all = append(all, h.Items[itemType]...)
	}
	return all
}

// ItemCount returns the total number of items
func (h *Hub) ItemCount() int {
	count := 0
	for _, items := range h.Items {
		count += len(items)
	}
	return count
}

// ItemCountByType returns items count per type
func (h *Hub) ItemCountByType() map[config.HubItemType]int {
	counts := make(map[config.HubItemType]int)
	for itemType, items := range h.Items {
		counts[itemType] = len(items)
	}
	return counts
}

// HookManifest represents the hook.yaml file in hub (legacy format)
type HookManifest struct {
	Name        string          `yaml:"name"`
	Type        config.HookType `yaml:"type"`
	Timeout     int             `yaml:"timeout,omitempty"`
	Command     string          `yaml:"command,omitempty"`     // Relative to hook folder or absolute for external
	Interpreter string          `yaml:"interpreter,omitempty"` // e.g., "bash", "node"
	Matcher     string          `yaml:"matcher,omitempty"`
	Inline      string          `yaml:"inline,omitempty"` // For inline hooks
}

// GetHookManifest reads the hook manifest from a hook folder
// Tries hooks.json (official format) first, falls back to hook.yaml (legacy)
func GetHookManifest(hubDir, hookName string) (*HookManifest, error) {
	hookDir := filepath.Join(hubDir, string(config.HubHooks), hookName)

	// Try hooks.json first (official format)
	hooksJSON, err := GetHooksJSON(hookDir)
	if err == nil && hooksJSON != nil {
		// Convert HooksJSON to HookManifest for compatibility
		return hooksJSONToManifest(hooksJSON, hookName)
	}

	// Fall back to hook.yaml (legacy format)
	manifestPath := filepath.Join(hookDir, "hook.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest HookManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// GetHooksJSON reads the hooks.json file from a hook folder (official Claude Code format)
func GetHooksJSON(hookDir string) (*config.HooksJSON, error) {
	hooksPath := filepath.Join(hookDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		return nil, err
	}

	var hooksJSON config.HooksJSON
	if err := json.Unmarshal(data, &hooksJSON); err != nil {
		return nil, err
	}

	return &hooksJSON, nil
}

// SaveHooksJSON writes a hooks.json file to the specified hook folder
func SaveHooksJSON(hookDir string, hooksJSON *config.HooksJSON) error {
	hooksPath := filepath.Join(hookDir, "hooks.json")
	data, err := json.MarshalIndent(hooksJSON, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(hooksPath, data, 0644)
}

// hooksJSONToManifest converts the first hook entry from HooksJSON to HookManifest
// This provides backward compatibility when reading hooks.json as HookManifest
func hooksJSONToManifest(hooksJSON *config.HooksJSON, hookName string) (*HookManifest, error) {
	// Find the first hook entry
	for hookType, entries := range hooksJSON.Hooks {
		if len(entries) > 0 && len(entries[0].Hooks) > 0 {
			cmd := entries[0].Hooks[0]
			return &HookManifest{
				Name:    hookName,
				Type:    hookType,
				Timeout: cmd.Timeout,
				Command: cmd.Command,
				Matcher: entries[0].Matcher,
			}, nil
		}
	}

	return &HookManifest{Name: hookName}, nil
}

// GetHookCommand returns the command to execute for a hook
func (m *HookManifest) GetHookCommand(hookDir string) string {
	if m.Inline != "" {
		return m.Inline
	}

	// If command is absolute path, use as-is
	if len(m.Command) > 0 && m.Command[0] == '/' {
		return m.Command
	}

	// Build path relative to the hook folder
	return filepath.Join(hookDir, m.Command)
}
