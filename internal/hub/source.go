package hub

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SourceType indicates where the item came from
type SourceType string

const (
	SourceTypeGitHub SourceType = "github" // installed from GitHub
	SourceTypeLocal  SourceType = "local"  // manually added
	SourceTypePlugin SourceType = "plugin" // installed via plugin
)

// GitHubSource contains GitHub repository information
type GitHubSource struct {
	Owner  string `yaml:"owner"`
	Repo   string `yaml:"repo"`
	Ref    string `yaml:"ref,omitempty"`    // branch or tag
	Commit string `yaml:"commit,omitempty"` // specific commit SHA
	Path   string `yaml:"path,omitempty"`   // path within repo (e.g., "skills/my-skill")
}

// RepoURL returns the full GitHub URL
func (g *GitHubSource) RepoURL() string {
	return "https://github.com/" + g.Owner + "/" + g.Repo
}

// PluginSource contains plugin back-reference for components installed via plugin
type PluginSource struct {
	Name    string `yaml:"name"`    // plugin name
	Owner   string `yaml:"owner"`   // original repo owner
	Repo    string `yaml:"repo"`    // original repo name
	Version string `yaml:"version"` // plugin version at install time
}

// SourceManifest tracks the origin of a hub item
type SourceManifest struct {
	Type        SourceType    `yaml:"type"`
	GitHub      *GitHubSource `yaml:"github,omitempty"`
	Plugin      *PluginSource `yaml:"plugin,omitempty"`
	InstalledAt time.Time     `yaml:"installed_at"`
	UpdatedAt   time.Time     `yaml:"updated_at"`
}

// LoadSourceManifest reads source.yaml from item directory
func LoadSourceManifest(itemPath string) (*SourceManifest, error) {
	path := filepath.Join(itemPath, "source.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m SourceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes source.yaml to item directory
func (m *SourceManifest) Save(itemPath string) error {
	m.UpdatedAt = time.Now()
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(itemPath, "source.yaml"), data, 0644)
}

// CanUpdate returns true if this item can be updated from remote
func (m *SourceManifest) CanUpdate() bool {
	return m.Type == SourceTypeGitHub || m.Type == SourceTypePlugin
}

// SourceInfo returns a human-readable source description
func (m *SourceManifest) SourceInfo() string {
	switch m.Type {
	case SourceTypeGitHub:
		if m.GitHub != nil {
			return m.GitHub.Owner + "/" + m.GitHub.Repo
		}
	case SourceTypePlugin:
		if m.Plugin != nil {
			return "plugin:" + m.Plugin.Name
		}
	case SourceTypeLocal:
		return "local"
	}
	return string(m.Type)
}

// NewGitHubSource creates a new SourceManifest for a GitHub-sourced item
func NewGitHubSource(owner, repo, ref, commit, path string) *SourceManifest {
	now := time.Now()
	return &SourceManifest{
		Type: SourceTypeGitHub,
		GitHub: &GitHubSource{
			Owner:  owner,
			Repo:   repo,
			Ref:    ref,
			Commit: commit,
			Path:   path,
		},
		InstalledAt: now,
		UpdatedAt:   now,
	}
}

// NewPluginSource creates a new SourceManifest for a plugin component
func NewPluginSource(pluginName, owner, repo, version string) *SourceManifest {
	now := time.Now()
	return &SourceManifest{
		Type: SourceTypePlugin,
		Plugin: &PluginSource{
			Name:    pluginName,
			Owner:   owner,
			Repo:    repo,
			Version: version,
		},
		InstalledAt: now,
		UpdatedAt:   now,
	}
}
