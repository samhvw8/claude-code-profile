package hub

import (
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
