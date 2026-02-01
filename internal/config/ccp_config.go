package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// CcpConfig represents the ccp.toml configuration file
type CcpConfig struct {
	// GitHub registry settings
	GitHub GitHubConfig `toml:"github"`

	// SkillsSh registry settings
	SkillsSh SkillsShConfig `toml:"skillssh"`

	// Default registry for searches
	DefaultRegistry string `toml:"default_registry"`

	// Installed sources (replaces registry.toml)
	Sources map[string]SourceConfig `toml:"sources,omitempty"`
}

// SourceConfig represents an installed source in ccp.toml
type SourceConfig struct {
	Registry  string    `toml:"registry"`
	Provider  string    `toml:"provider"`
	URL       string    `toml:"url"`
	Path      string    `toml:"path"`
	Ref       string    `toml:"ref,omitempty"`
	Commit    string    `toml:"commit,omitempty"`
	Checksum  string    `toml:"checksum,omitempty"`
	Updated   time.Time `toml:"updated"`
	Installed []string  `toml:"installed,omitempty"`
}

// GitHubConfig holds GitHub registry settings
type GitHubConfig struct {
	// Topics to search for skills/plugins
	Topics []string `toml:"topics"`

	// Additional search terms
	Keywords []string `toml:"keywords"`

	// Results per topic
	PerPage int `toml:"per_page"`
}

// SkillsShConfig holds skills.sh registry settings
type SkillsShConfig struct {
	// API base URL (for self-hosted)
	BaseURL string `toml:"base_url"`

	// Default result limit
	Limit int `toml:"limit"`
}

// DefaultCcpConfig returns default configuration
func DefaultCcpConfig() *CcpConfig {
	return &CcpConfig{
		DefaultRegistry: "skills.sh",
		GitHub: GitHubConfig{
			Topics: []string{
				"agent-skills",
				"claude-code",
				"claude-skills",
				"awesome-skills",
				"claude-plugin",
			},
			PerPage: 10,
		},
		SkillsSh: SkillsShConfig{
			BaseURL: "https://skills.sh",
			Limit:   10,
		},
	}
}

// LoadCcpConfig loads ccp.toml from ~/.ccp/ccp.toml
func LoadCcpConfig(ccpDir string) (*CcpConfig, error) {
	configPath := filepath.Join(ccpDir, "ccp.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultCcpConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultCcpConfig()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save writes ccp.toml to disk
func (c *CcpConfig) Save(ccpDir string) error {
	configPath := filepath.Join(ccpDir, "ccp.toml")

	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// Global config instance (lazy loaded)
var globalConfig *CcpConfig

// GetConfig returns the global config, loading it if needed
func GetConfig() *CcpConfig {
	if globalConfig != nil {
		return globalConfig
	}

	paths, err := ResolvePaths()
	if err != nil {
		return DefaultCcpConfig()
	}

	cfg, err := LoadCcpConfig(paths.CcpDir)
	if err != nil {
		return DefaultCcpConfig()
	}

	globalConfig = cfg
	return globalConfig
}
