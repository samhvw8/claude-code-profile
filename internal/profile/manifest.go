package profile

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
)

// Manifest represents the profile.yaml file
type Manifest struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description,omitempty"`
	Created     time.Time  `yaml:"created"`
	Updated     time.Time  `yaml:"updated"`
	Hub         HubLinks   `yaml:"hub"`
	Data        DataConfig `yaml:"data"`
}

// HubLinks defines which hub items are linked to this profile
type HubLinks struct {
	Skills      []string `yaml:"skills,omitempty"`
	Agents      []string `yaml:"agents,omitempty"`
	Hooks       []string `yaml:"hooks,omitempty"`
	Rules       []string `yaml:"rules,omitempty"`
	Commands    []string `yaml:"commands,omitempty"`
	MdFragments []string `yaml:"md-fragments,omitempty"`
}

// DataConfig defines sharing mode for data directories
type DataConfig struct {
	Tasks       config.ShareMode `yaml:"tasks"`
	Todos       config.ShareMode `yaml:"todos"`
	PasteCache  config.ShareMode `yaml:"paste-cache"`
	History     config.ShareMode `yaml:"history"`
	FileHistory config.ShareMode `yaml:"file-history"`
	SessionEnv  config.ShareMode `yaml:"session-env"`
	Projects    config.ShareMode `yaml:"projects"`
	Plans       config.ShareMode `yaml:"plans"`
}

// NewManifest creates a new manifest with defaults
func NewManifest(name, description string) *Manifest {
	now := time.Now()
	defaults := config.DefaultDataConfig()

	return &Manifest{
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

// LoadManifest reads a manifest from file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// Save writes the manifest to file
func (m *Manifest) Save(path string) error {
	m.Updated = time.Now()

	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
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
	case config.HubMdFragments:
		return m.Hub.MdFragments
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
	case config.HubMdFragments:
		m.Hub.MdFragments = items
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
