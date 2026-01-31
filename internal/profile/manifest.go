package profile

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
)

// ManifestVersion is the current manifest format version
const ManifestVersion = 2

// Manifest represents the profile.toml file
type Manifest struct {
	Version     int               `toml:"version" yaml:"-"`
	Name        string            `toml:"name" yaml:"name"`
	Description string            `toml:"description,omitempty" yaml:"description,omitempty"`
	Created     time.Time         `toml:"created" yaml:"created"`
	Updated     time.Time         `toml:"updated" yaml:"updated"`
	Hub         HubLinks          `toml:"hub" yaml:"hub"`
	Data        DataConfig        `toml:"data" yaml:"data"`
	Hooks       []config.HookConfig `toml:"hooks,omitempty" yaml:"hooks,omitempty"`
}

// HubLinks defines which hub items are linked to this profile
type HubLinks struct {
	Skills           []string `toml:"skills,omitempty" yaml:"skills,omitempty"`
	Agents           []string `toml:"agents,omitempty" yaml:"agents,omitempty"`
	Hooks            []string `toml:"hooks,omitempty" yaml:"hooks,omitempty"`
	Rules            []string `toml:"rules,omitempty" yaml:"rules,omitempty"`
	Commands         []string `toml:"commands,omitempty" yaml:"commands,omitempty"`
	SettingFragments []string `toml:"setting-fragments,omitempty" yaml:"setting-fragments,omitempty"`
}

// DataConfig defines sharing mode for data directories
type DataConfig struct {
	Tasks       config.ShareMode `toml:"tasks" yaml:"tasks"`
	Todos       config.ShareMode `toml:"todos" yaml:"todos"`
	PasteCache  config.ShareMode `toml:"paste-cache" yaml:"paste-cache"`
	History     config.ShareMode `toml:"history" yaml:"history"`
	FileHistory config.ShareMode `toml:"file-history" yaml:"file-history"`
	SessionEnv  config.ShareMode `toml:"session-env" yaml:"session-env"`
	Projects    config.ShareMode `toml:"projects" yaml:"projects"`
	Plans       config.ShareMode `toml:"plans" yaml:"plans"`
}

// NewManifest creates a new manifest with defaults
func NewManifest(name, description string) *Manifest {
	now := time.Now()
	defaults := config.DefaultDataConfig()

	return &Manifest{
		Version:     ManifestVersion,
		Name:        name,
		Description: description,
		Created:     now,
		Updated:     now,
		Hub:         HubLinks{},
		Data: DataConfig{
			Tasks:       defaults[config.DataTasks],
			Todos:       defaults[config.DataTodos],
			PasteCache:  defaults[config.DataPasteCache],
			History:     defaults[config.DataHistory],
			FileHistory: defaults[config.DataFileHistory],
			SessionEnv:  defaults[config.DataSessionEnv],
			Projects:    defaults[config.DataProjects],
			Plans:       defaults[config.DataPlans],
		},
	}
}

// LoadManifest reads a manifest from file (TOML or YAML)
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest

	// Try TOML first (new format)
	if err := toml.Unmarshal(data, &m); err == nil && m.Version >= 2 {
		return &m, nil
	}

	// Fall back to YAML (old format)
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	// Mark as needing migration (version 0 or 1 = YAML)
	if m.Version == 0 {
		m.Version = 1
	}

	return &m, nil
}

// Save writes the manifest to file (always TOML)
func (m *Manifest) Save(path string) error {
	m.Updated = time.Now()
	m.Version = ManifestVersion

	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// SaveTOML saves as profile.toml specifically
func (m *Manifest) SaveTOML(dir string) error {
	path := filepath.Join(dir, "profile.toml")
	return m.Save(path)
}

// NeedsMigration returns true if manifest is old YAML format
func (m *Manifest) NeedsMigration() bool {
	return m.Version < ManifestVersion
}

// ManifestPath returns the path to the manifest file
// Checks for .toml first, falls back to .yaml
func ManifestPath(profileDir string) string {
	tomlPath := filepath.Join(profileDir, "profile.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}
	return filepath.Join(profileDir, "profile.yaml")
}

// GetHubItems returns all hub item names for a given type
func (m *Manifest) GetHubItems(itemType config.HubItemType) []string {
	switch itemType {
	case config.HubSkills:
		return m.Hub.Skills
	case config.HubAgents:
		return m.Hub.Agents
	case config.HubHooks:
		return m.Hub.Hooks
	case config.HubRules:
		return m.Hub.Rules
	case config.HubCommands:
		return m.Hub.Commands
	case config.HubSettingFragments:
		return m.Hub.SettingFragments
	default:
		return nil
	}
}

// SetHubItems sets hub item names for a given type
func (m *Manifest) SetHubItems(itemType config.HubItemType, items []string) {
	switch itemType {
	case config.HubSkills:
		m.Hub.Skills = items
	case config.HubAgents:
		m.Hub.Agents = items
	case config.HubHooks:
		m.Hub.Hooks = items
	case config.HubRules:
		m.Hub.Rules = items
	case config.HubCommands:
		m.Hub.Commands = items
	case config.HubSettingFragments:
		m.Hub.SettingFragments = items
	}
}

// AddHubItem adds a hub item to the manifest
func (m *Manifest) AddHubItem(itemType config.HubItemType, name string) {
	items := m.GetHubItems(itemType)
	for _, existing := range items {
		if existing == name {
			return // Already exists
		}
	}
	m.SetHubItems(itemType, append(items, name))
}

// RemoveHubItem removes a hub item from the manifest
func (m *Manifest) RemoveHubItem(itemType config.HubItemType, name string) bool {
	items := m.GetHubItems(itemType)
	for i, existing := range items {
		if existing == name {
			m.SetHubItems(itemType, append(items[:i], items[i+1:]...))
			return true
		}
	}
	return false
}

// GetDataShareMode returns the share mode for a data type
func (m *Manifest) GetDataShareMode(dataType config.DataItemType) config.ShareMode {
	switch dataType {
	case config.DataTasks:
		return m.Data.Tasks
	case config.DataTodos:
		return m.Data.Todos
	case config.DataPasteCache:
		return m.Data.PasteCache
	case config.DataHistory:
		return m.Data.History
	case config.DataFileHistory:
		return m.Data.FileHistory
	case config.DataSessionEnv:
		return m.Data.SessionEnv
	case config.DataProjects:
		return m.Data.Projects
	case config.DataPlans:
		return m.Data.Plans
	default:
		return config.ShareModeIsolated
	}
}

// SetDataShareMode sets the share mode for a data type
func (m *Manifest) SetDataShareMode(dataType config.DataItemType, mode config.ShareMode) {
	switch dataType {
	case config.DataTasks:
		m.Data.Tasks = mode
	case config.DataTodos:
		m.Data.Todos = mode
	case config.DataPasteCache:
		m.Data.PasteCache = mode
	case config.DataHistory:
		m.Data.History = mode
	case config.DataFileHistory:
		m.Data.FileHistory = mode
	case config.DataSessionEnv:
		m.Data.SessionEnv = mode
	case config.DataProjects:
		m.Data.Projects = mode
	case config.DataPlans:
		m.Data.Plans = mode
	}
}

// AllHubItemsFlat returns all hub items as type/name pairs
func (m *Manifest) AllHubItemsFlat() []struct {
	Type config.HubItemType
	Name string
} {
	var result []struct {
		Type config.HubItemType
		Name string
	}

	for _, itemType := range config.AllHubItemTypes() {
		for _, name := range m.GetHubItems(itemType) {
			result = append(result, struct {
				Type config.HubItemType
				Name string
			}{Type: itemType, Name: name})
		}
	}

	return result
}
