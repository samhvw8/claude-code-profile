package hub

import (
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
	Name     string
	Type     config.HubItemType
	Path     string
	IsDir    bool
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

// HookManifest represents the hook.yaml file in hub
type HookManifest struct {
	Name        string          `yaml:"name"`
	Type        config.HookType `yaml:"type"`
	Timeout     int             `yaml:"timeout,omitempty"`
	Command     string          `yaml:"command,omitempty"`     // Relative to hook folder or absolute for external
	Interpreter string          `yaml:"interpreter,omitempty"` // e.g., "bash", "node"
	Matcher     string          `yaml:"matcher,omitempty"`
	Inline      string          `yaml:"inline,omitempty"` // For inline hooks
}

// GetHookManifest reads the hook.yaml manifest from a hook folder
func GetHookManifest(hubDir, hookName string) (*HookManifest, error) {
	manifestPath := filepath.Join(hubDir, string(config.HubHooks), hookName, "hook.yaml")
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
