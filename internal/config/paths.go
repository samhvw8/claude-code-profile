package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths holds all resolved paths for ccp operations
type Paths struct {
	CcpDir      string // ~/.ccp (ccp data directory)
	ClaudeDir   string // ~/.claude (symlink to active profile)
	HubDir      string // ~/.ccp/hub
	ProfilesDir string // ~/.ccp/profiles
	SharedDir   string // ~/.ccp/profiles/shared
}

// HubItemType represents the type of item in the hub
type HubItemType string

const (
	HubSkills           HubItemType = "skills"
	HubAgents           HubItemType = "agents"
	HubHooks            HubItemType = "hooks"
	HubRules            HubItemType = "rules"
	HubCommands         HubItemType = "commands"
	HubSettingFragments HubItemType = "setting-fragments"
)

// AllHubItemTypes returns all hub item types in order
func AllHubItemTypes() []HubItemType {
	return []HubItemType{HubSkills, HubAgents, HubHooks, HubRules, HubCommands, HubSettingFragments}
}

// DataItemType represents data directories that can be shared or isolated
type DataItemType string

const (
	DataTasks       DataItemType = "tasks"
	DataTodos       DataItemType = "todos"
	DataPasteCache  DataItemType = "paste-cache"
	DataHistory     DataItemType = "history"
	DataFileHistory DataItemType = "file-history"
	DataSessionEnv  DataItemType = "session-env"
	DataProjects    DataItemType = "projects"
	DataPlans       DataItemType = "plans"
)

// AllDataItemTypes returns all data item types
func AllDataItemTypes() []DataItemType {
	return []DataItemType{
		DataTasks, DataTodos, DataPasteCache, DataHistory,
		DataFileHistory, DataSessionEnv, DataProjects, DataPlans,
	}
}

// DefaultDataConfig returns the default sharing configuration
func DefaultDataConfig() map[DataItemType]ShareMode {
	return map[DataItemType]ShareMode{
		DataTasks:       ShareModeShared,
		DataTodos:       ShareModeShared,
		DataPasteCache:  ShareModeShared,
		DataHistory:     ShareModeIsolated,
		DataFileHistory: ShareModeIsolated,
		DataSessionEnv:  ShareModeIsolated,
		DataProjects:    ShareModeShared,
		DataPlans:       ShareModeIsolated,
	}
}

// ShareMode indicates whether data is shared or isolated
type ShareMode string

const (
	ShareModeShared   ShareMode = "shared"
	ShareModeIsolated ShareMode = "isolated"
)

// ResolvePaths resolves all paths based on environment and defaults
func ResolvePaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// CCP data directory (can be overridden)
	ccpDir := os.Getenv("CCP_DIR")
	if ccpDir == "" {
		ccpDir = filepath.Join(home, ".ccp")
	}

	// Claude config directory (symlink target)
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}

	return &Paths{
		CcpDir:      ccpDir,
		ClaudeDir:   claudeDir,
		HubDir:      filepath.Join(ccpDir, "hub"),
		ProfilesDir: filepath.Join(ccpDir, "profiles"),
		SharedDir:   filepath.Join(ccpDir, "profiles", "shared"),
	}, nil
}

// ProfileDir returns the directory for a specific profile
func (p *Paths) ProfileDir(name string) string {
	return filepath.Join(p.ProfilesDir, name)
}

// HubItemDir returns the directory for a specific hub item type
func (p *Paths) HubItemDir(itemType HubItemType) string {
	return filepath.Join(p.HubDir, string(itemType))
}

// HubItemPath returns the full path to a specific hub item
func (p *Paths) HubItemPath(itemType HubItemType, name string) string {
	// Setting fragments are stored as .yaml files
	if itemType == HubSettingFragments {
		return filepath.Join(p.HubDir, string(itemType), name+".yaml")
	}
	return filepath.Join(p.HubDir, string(itemType), name)
}

// SharedDataDir returns the shared data directory for a specific data type
func (p *Paths) SharedDataDir(dataType DataItemType) string {
	return filepath.Join(p.SharedDir, string(dataType))
}

// PluginsDir returns the plugins tracking directory
func (p *Paths) PluginsDir() string {
	return filepath.Join(p.HubDir, "plugins")
}

// IsInitialized checks if ccp has been initialized (hub directory exists)
func (p *Paths) IsInitialized() bool {
	info, err := os.Stat(p.HubDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CcpDirExists checks if the ccp directory exists
func (p *Paths) CcpDirExists() bool {
	info, err := os.Stat(p.CcpDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ClaudeDirExistsAsDir checks if ~/.claude exists as a real directory (not symlink)
// Used to detect existing Claude config that can be migrated
func (p *Paths) ClaudeDirExistsAsDir() bool {
	info, err := os.Lstat(p.ClaudeDir)
	if err != nil {
		return false
	}
	// Must be a directory, not a symlink
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

// ClaudeDirIsSymlink checks if ~/.claude is a symlink
func (p *Paths) ClaudeDirIsSymlink() bool {
	info, err := os.Lstat(p.ClaudeDir)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// SourcesDir returns the directory for cloned sources
func (p *Paths) SourcesDir() string {
	return filepath.Join(p.CcpDir, "sources")
}

// RegistryPath returns the path to registry.toml
func (p *Paths) RegistryPath() string {
	return filepath.Join(p.CcpDir, "registry.toml")
}

// SourceDir returns the directory for a specific source
func (p *Paths) SourceDir(sourceID string) string {
	safeName := strings.ReplaceAll(sourceID, "/", "--")
	return filepath.Join(p.SourcesDir(), safeName)
}
