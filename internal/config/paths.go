package config

import (
	"os"
	"path/filepath"
)

// Paths holds all resolved paths for ccp operations
type Paths struct {
	ClaudeDir   string // ~/.claude (or CLAUDE_CONFIG_DIR)
	HubDir      string // ~/.claude/hub
	ProfilesDir string // ~/.claude/profiles
	SharedDir   string // ~/.claude/profiles/shared
}

// HubItemType represents the type of item in the hub
type HubItemType string

const (
	HubSkills      HubItemType = "skills"
	HubAgents      HubItemType = "agents"
	HubHooks       HubItemType = "hooks"
	HubRules       HubItemType = "rules"
	HubCommands    HubItemType = "commands"
	HubMdFragments HubItemType = "md-fragments"
)

// AllHubItemTypes returns all hub item types in order
func AllHubItemTypes() []HubItemType {
	return []HubItemType{HubSkills, HubAgents, HubHooks, HubRules, HubCommands, HubMdFragments}
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
	claudeDir := os.Getenv("CCP_CLAUDE_DIR")
	if claudeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		claudeDir = filepath.Join(home, ".claude")
	}

	return &Paths{
		ClaudeDir:   claudeDir,
		HubDir:      filepath.Join(claudeDir, "hub"),
		ProfilesDir: filepath.Join(claudeDir, "profiles"),
		SharedDir:   filepath.Join(claudeDir, "profiles", "shared"),
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
	return filepath.Join(p.HubDir, string(itemType), name)
}

// SharedDataDir returns the shared data directory for a specific data type
func (p *Paths) SharedDataDir(dataType DataItemType) string {
	return filepath.Join(p.SharedDir, string(dataType))
}

// IsInitialized checks if ccp has been initialized (hub directory exists)
func (p *Paths) IsInitialized() bool {
	info, err := os.Stat(p.HubDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ClaudeDirExists checks if the claude directory exists
func (p *Paths) ClaudeDirExists() bool {
	info, err := os.Stat(p.ClaudeDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}
