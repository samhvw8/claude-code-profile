package hub

import (
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// Scanner scans directories for hub items
type Scanner struct{}

// NewScanner creates a new Scanner
func NewScanner() *Scanner {
	return &Scanner{}
}

// Scan scans the hub directory and returns a populated Hub
func (s *Scanner) Scan(hubPath string) (*Hub, error) {
	hub := New(hubPath)

	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(hubPath, string(itemType))

		items, err := s.scanItemDir(itemDir, itemType)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist, skip
				continue
			}
			return nil, err
		}

		hub.Items[itemType] = items
	}

	return hub, nil
}

// ScanSource scans an existing claude directory for hub-eligible items
// Used during init to find items to migrate
func (s *Scanner) ScanSource(claudeDir string) (*Hub, error) {
	hub := New(claudeDir)

	// Map source directories to hub types
	dirMap := map[string]config.HubItemType{
		"skills":   config.HubSkills,
		"agents":   config.HubAgents,
		"hooks":    config.HubHooks,
		"rules":    config.HubRules,
		"commands": config.HubCommands,
	}

	for dirName, itemType := range dirMap {
		itemDir := filepath.Join(claudeDir, dirName)

		items, err := s.scanItemDir(itemDir, itemType)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		hub.Items[itemType] = items
	}

	return hub, nil
}

// scanItemDir scans a single item directory
func (s *Scanner) scanItemDir(dir string, itemType config.HubItemType) ([]Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var items []Item
	for _, entry := range entries {
		// Skip hidden files
		if entry.Name()[0] == '.' {
			continue
		}

		name := entry.Name()

		// For setting-fragments, strip the .yaml extension from the name
		if itemType == config.HubSettingFragments && !entry.IsDir() {
			if ext := filepath.Ext(name); ext == ".yaml" || ext == ".yml" {
				name = name[:len(name)-len(ext)]
			}
		}

		item := Item{
			Name:  name,
			Type:  itemType,
			Path:  filepath.Join(dir, entry.Name()),
			IsDir: entry.IsDir(),
		}

		items = append(items, item)
	}

	return items, nil
}
