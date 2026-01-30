package hub

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// PluginManifest tracks installed plugin and its components
type PluginManifest struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Version     string        `yaml:"version"`
	GitHub      GitHubSource  `yaml:"github"`
	InstalledAt time.Time     `yaml:"installed_at"`
	UpdatedAt   time.Time     `yaml:"updated_at"`
	Components  ComponentList `yaml:"components"`
}

// ComponentList tracks what was installed from this plugin
type ComponentList struct {
	Skills   []string `yaml:"skills,omitempty"`
	Agents   []string `yaml:"agents,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Rules    []string `yaml:"rules,omitempty"`
	Hooks    []string `yaml:"hooks,omitempty"` // hook folder names (usually just "hooks")
}

// ComponentRef represents a type/name pair for a component
type ComponentRef struct {
	Type string
	Name string
}

// AllComponents returns all component names as type/name pairs
func (c *ComponentList) AllComponents() []ComponentRef {
	var result []ComponentRef
	for _, s := range c.Skills {
		result = append(result, ComponentRef{Type: "skills", Name: s})
	}
	for _, a := range c.Agents {
		result = append(result, ComponentRef{Type: "agents", Name: a})
	}
	for _, cmd := range c.Commands {
		result = append(result, ComponentRef{Type: "commands", Name: cmd})
	}
	for _, r := range c.Rules {
		result = append(result, ComponentRef{Type: "rules", Name: r})
	}
	for _, h := range c.Hooks {
		result = append(result, ComponentRef{Type: "hooks", Name: h})
	}
	return result
}

// Count returns total number of components
func (c *ComponentList) Count() int {
	return len(c.Skills) + len(c.Agents) + len(c.Commands) + len(c.Rules) + len(c.Hooks)
}

// LoadPluginManifest reads plugin.yaml from plugins directory
func LoadPluginManifest(pluginsDir, name string) (*PluginManifest, error) {
	path := filepath.Join(pluginsDir, name, "plugin.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m PluginManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes plugin.yaml to plugins directory
func (m *PluginManifest) Save(pluginsDir string) error {
	m.UpdatedAt = time.Now()
	dir := filepath.Join(pluginsDir, m.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plugin.yaml"), data, 0644)
}

// Delete removes the plugin manifest directory
func (m *PluginManifest) Delete(pluginsDir string) error {
	return os.RemoveAll(filepath.Join(pluginsDir, m.Name))
}

// ListPlugins returns all installed plugins
func ListPlugins(pluginsDir string) ([]*PluginManifest, error) {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []*PluginManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pm, err := LoadPluginManifest(pluginsDir, e.Name())
		if err != nil {
			continue // skip invalid entries
		}
		plugins = append(plugins, pm)
	}
	return plugins, nil
}

// NewPluginManifest creates a new plugin manifest
func NewPluginManifest(name, description, version string, github GitHubSource, components ComponentList) *PluginManifest {
	now := time.Now()
	return &PluginManifest{
		Name:        name,
		Description: description,
		Version:     version,
		GitHub:      github,
		InstalledAt: now,
		UpdatedAt:   now,
		Components:  components,
	}
}
