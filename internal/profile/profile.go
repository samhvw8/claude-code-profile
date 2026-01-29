package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/symlink"
)

// Profile represents a Claude Code profile
type Profile struct {
	Name     string
	Path     string
	Manifest *Manifest
	symMgr   *symlink.Manager
}

// Manager handles profile operations
type Manager struct {
	paths  *config.Paths
	symMgr *symlink.Manager
}

// NewManager creates a new profile manager
func NewManager(paths *config.Paths) *Manager {
	return &Manager{
		paths:  paths,
		symMgr: symlink.New(),
	}
}

// Get retrieves a profile by name
func (m *Manager) Get(name string) (*Profile, error) {
	profileDir := m.paths.ProfileDir(name)

	info, err := os.Stat(profileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	manifestPath := filepath.Join(profileDir, "profile.yaml")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Profile exists but no manifest - create a basic one
			manifest = NewManifest(name, "")
		} else {
			return nil, err
		}
	}

	return &Profile{
		Name:     name,
		Path:     profileDir,
		Manifest: manifest,
		symMgr:   m.symMgr,
	}, nil
}

// List returns all profiles
func (m *Manager) List() ([]*Profile, error) {
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var profiles []*Profile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip special directories
		if entry.Name() == "shared" {
			continue
		}

		profile, err := m.Get(entry.Name())
		if err != nil {
			continue
		}
		if profile != nil {
			profiles = append(profiles, profile)
		}
	}

	return profiles, nil
}

// Create creates a new profile
func (m *Manager) Create(name string, manifest *Manifest) (*Profile, error) {
	profileDir := m.paths.ProfileDir(name)

	// Check if already exists
	if _, err := os.Stat(profileDir); err == nil {
		return nil, os.ErrExist
	}

	// Get default permissions from ~/.claude if it exists (resolved through symlink)
	defaultPerm := os.FileMode(0755)
	if info, err := os.Stat(m.paths.ClaudeDir); err == nil {
		defaultPerm = info.Mode().Perm()
	}

	// Create profile directory
	if err := os.MkdirAll(profileDir, defaultPerm); err != nil {
		return nil, err
	}

	// Create hub item directories
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(profileDir, string(itemType))

		// Preserve subdir permissions from ~/.claude if they exist
		itemPerm := defaultPerm
		srcItemDir := filepath.Join(m.paths.ClaudeDir, string(itemType))
		if info, err := os.Stat(srcItemDir); err == nil {
			itemPerm = info.Mode().Perm()
		}

		if err := os.MkdirAll(itemDir, itemPerm); err != nil {
			return nil, err
		}
	}

	// Create data directories based on config
	for _, dataType := range config.AllDataItemTypes() {
		mode := manifest.GetDataShareMode(dataType)
		dataDir := filepath.Join(profileDir, string(dataType))

		// Preserve data dir permissions from ~/.claude if they exist
		dataPerm := defaultPerm
		srcDataDir := filepath.Join(m.paths.ClaudeDir, string(dataType))
		if info, err := os.Stat(srcDataDir); err == nil {
			dataPerm = info.Mode().Perm()
		}

		if mode == config.ShareModeShared {
			// Create symlink to shared directory
			sharedDir := m.paths.SharedDataDir(dataType)
			if err := os.MkdirAll(sharedDir, dataPerm); err != nil {
				return nil, err
			}
			if err := m.symMgr.Create(dataDir, sharedDir); err != nil {
				return nil, err
			}
		} else {
			// Create local directory
			if err := os.MkdirAll(dataDir, dataPerm); err != nil {
				return nil, err
			}
		}
	}

	// Create symlinks for hub items
	for _, itemType := range config.AllHubItemTypes() {
		for _, itemName := range manifest.GetHubItems(itemType) {
			hubItemPath := m.paths.HubItemPath(itemType, itemName)
			profileItemPath := filepath.Join(profileDir, string(itemType), itemName)
			if err := m.symMgr.Create(profileItemPath, hubItemPath); err != nil {
				return nil, err
			}
		}
	}

	// Save manifest
	manifestPath := filepath.Join(profileDir, "profile.yaml")
	manifest.Name = name
	if err := manifest.Save(manifestPath); err != nil {
		return nil, err
	}

	// Generate settings.json with hooks and setting fragments from hub
	if len(manifest.Hub.Hooks) > 0 || len(manifest.Hub.SettingFragments) > 0 {
		if err := RegenerateSettings(m.paths, profileDir, manifest); err != nil {
			// Non-fatal - log and continue
			fmt.Fprintf(os.Stderr, "Warning: failed to generate settings.json: %v\n", err)
		}
	} else if len(manifest.Hooks) > 0 {
		// Legacy: Sync hooks from old-style manifest.Hooks
		settingsMgr := NewSettingsManager(m.paths)
		if err := settingsMgr.SyncHooksFromManifest(profileDir, manifest); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to sync hooks to settings.json: %v\n", err)
		}
	}

	return &Profile{
		Name:     name,
		Path:     profileDir,
		Manifest: manifest,
		symMgr:   m.symMgr,
	}, nil
}

// Delete removes a profile
func (m *Manager) Delete(name string) error {
	// Don't allow deleting default if it's the only profile
	profileDir := m.paths.ProfileDir(name)
	return os.RemoveAll(profileDir)
}

// Exists checks if a profile exists
func (m *Manager) Exists(name string) bool {
	profileDir := m.paths.ProfileDir(name)
	info, err := os.Stat(profileDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// LinkHubItem adds a hub item to a profile
func (m *Manager) LinkHubItem(profileName string, itemType config.HubItemType, itemName string) error {
	profile, err := m.Get(profileName)
	if err != nil {
		return err
	}
	if profile == nil {
		return os.ErrNotExist
	}

	// Create symlink
	hubItemPath := m.paths.HubItemPath(itemType, itemName)
	profileItemPath := filepath.Join(profile.Path, string(itemType), itemName)

	// Check hub item exists
	if _, err := os.Stat(hubItemPath); err != nil {
		return err
	}

	// Create symlink
	if err := m.symMgr.Create(profileItemPath, hubItemPath); err != nil {
		return err
	}

	// Update manifest
	profile.Manifest.AddHubItem(itemType, itemName)
	manifestPath := filepath.Join(profile.Path, "profile.yaml")
	return profile.Manifest.Save(manifestPath)
}

// UnlinkHubItem removes a hub item from a profile
func (m *Manager) UnlinkHubItem(profileName string, itemType config.HubItemType, itemName string) error {
	profile, err := m.Get(profileName)
	if err != nil {
		return err
	}
	if profile == nil {
		return os.ErrNotExist
	}

	// Remove symlink
	profileItemPath := filepath.Join(profile.Path, string(itemType), itemName)
	if err := os.Remove(profileItemPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Update manifest
	profile.Manifest.RemoveHubItem(itemType, itemName)
	manifestPath := filepath.Join(profile.Path, "profile.yaml")
	return profile.Manifest.Save(manifestPath)
}

// GetActive returns the currently active profile (via symlink)
func (m *Manager) GetActive() (*Profile, error) {
	// Check if ClaudeDir is a symlink
	isSymlink, err := m.symMgr.IsSymlink(m.paths.ClaudeDir)
	if err != nil {
		return nil, err
	}
	if !isSymlink {
		return nil, nil
	}

	// Read the symlink target
	target, err := m.symMgr.ReadLink(m.paths.ClaudeDir)
	if err != nil {
		return nil, err
	}

	// Extract profile name from path
	profileName := filepath.Base(target)
	return m.Get(profileName)
}

// SetActive sets the active profile by updating the symlink
func (m *Manager) SetActive(name string) error {
	profileDir := m.paths.ProfileDir(name)

	// Verify profile exists
	if _, err := os.Stat(profileDir); err != nil {
		return err
	}

	// Swap symlink
	return m.symMgr.Swap(m.paths.ClaudeDir, profileDir)
}
