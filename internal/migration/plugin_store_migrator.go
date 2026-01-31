package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/symlink"
)

// PluginStoreMigrator handles migration of plugin cache to shared store
type PluginStoreMigrator struct {
	paths  *config.Paths
	symMgr *symlink.Manager
}

// NewPluginStoreMigrator creates a new plugin store migrator
func NewPluginStoreMigrator(paths *config.Paths) *PluginStoreMigrator {
	return &PluginStoreMigrator{
		paths:  paths,
		symMgr: symlink.New(),
	}
}

// NeedsMigration checks if any profile has plugin items that should be in store
func (m *PluginStoreMigrator) NeedsMigration() bool {
	profiles, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		return false
	}

	for _, profile := range profiles {
		if !profile.IsDir() || profile.Name() == "shared" {
			continue
		}

		profileDir := m.paths.ProfileDir(profile.Name())
		pluginsDir := filepath.Join(profileDir, "plugins")

		for _, item := range config.SharedPluginStoreItems() {
			itemPath := filepath.Join(pluginsDir, string(item))
			info, err := os.Lstat(itemPath)
			if err != nil {
				continue
			}
			// If it exists and is NOT a symlink, needs migration
			if info.Mode()&os.ModeSymlink == 0 {
				return true
			}
		}
	}

	return false
}

// Migrate moves plugin items to shared store and replaces with symlinks
func (m *PluginStoreMigrator) Migrate() (int, error) {
	// Create store plugins directory
	storePluginsDir := m.paths.StorePluginsDir()
	if err := os.MkdirAll(storePluginsDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create store plugins dir: %w", err)
	}

	profiles, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		return 0, nil // No profiles yet
	}

	count := 0

	for _, profile := range profiles {
		if !profile.IsDir() || profile.Name() == "shared" {
			continue
		}

		profileDir := m.paths.ProfileDir(profile.Name())
		pluginsDir := filepath.Join(profileDir, "plugins")

		for _, item := range config.SharedPluginStoreItems() {
			itemPath := filepath.Join(pluginsDir, string(item))
			storeItemPath := m.paths.StorePluginItemPath(item)

			info, err := os.Lstat(itemPath)
			if err != nil {
				continue // Item doesn't exist
			}

			// Skip if already a symlink
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}

			// Check if store already has this item
			if _, err := os.Stat(storeItemPath); err == nil {
				// Store already has it, just replace with symlink
				if err := os.RemoveAll(itemPath); err != nil {
					return count, fmt.Errorf("failed to remove %s: %w", itemPath, err)
				}
			} else {
				// Move to store
				if err := os.Rename(itemPath, storeItemPath); err != nil {
					// Try copy + remove if rename fails (cross-device)
					if err := copyRecursive(itemPath, storeItemPath); err != nil {
						return count, fmt.Errorf("failed to copy %s to store: %w", itemPath, err)
					}
					if err := os.RemoveAll(itemPath); err != nil {
						return count, fmt.Errorf("failed to remove %s after copy: %w", itemPath, err)
					}
				}
			}

			// Create symlink
			if err := m.symMgr.Create(itemPath, storeItemPath); err != nil {
				return count, fmt.Errorf("failed to create symlink for %s: %w", item, err)
			}

			count++
		}
	}

	return count, nil
}

// CreateStoreStructure creates the store directory structure
func (m *PluginStoreMigrator) CreateStoreStructure() error {
	storePluginsDir := m.paths.StorePluginsDir()
	if err := os.MkdirAll(storePluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create store plugins dir: %w", err)
	}
	return nil
}
