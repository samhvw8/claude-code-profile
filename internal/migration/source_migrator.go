package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

// SourceMigrator handles migration from source.yaml files to ccp.toml
type SourceMigrator struct {
	paths *config.Paths
}

// NewSourceMigrator creates a new source migrator
func NewSourceMigrator(paths *config.Paths) *SourceMigrator {
	return &SourceMigrator{paths: paths}
}

// oldSourceManifest represents the old source.yaml format
type oldSourceManifest struct {
	Type   string `yaml:"type"`
	GitHub struct {
		Owner  string `yaml:"owner"`
		Repo   string `yaml:"repo"`
		Ref    string `yaml:"ref"`
		Commit string `yaml:"commit"`
	} `yaml:"github"`
	Plugin struct {
		Name    string `yaml:"name"`
		Owner   string `yaml:"owner"`
		Repo    string `yaml:"repo"`
		Version string `yaml:"version"`
	} `yaml:"plugin"`
}

// MigrateSourceYAML migrates per-item source.yaml files to ccp.toml
func (m *SourceMigrator) MigrateSourceYAML() (int, error) {
	registry, err := source.LoadRegistry(m.paths.RegistryPath())
	if err != nil {
		return 0, err
	}

	count := 0

	// Scan hub items for source.yaml files
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := m.paths.HubItemDir(itemType)
		entries, err := os.ReadDir(itemDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			itemPath := filepath.Join(itemDir, entry.Name())
			sourceYAML := filepath.Join(itemPath, "source.yaml")

			if _, err := os.Stat(sourceYAML); err != nil {
				continue // No source.yaml
			}

			// Load existing source manifest
			data, err := os.ReadFile(sourceYAML)
			if err != nil {
				continue
			}

			// Parse YAML
			var old oldSourceManifest
			if err := yaml.Unmarshal(data, &old); err != nil {
				continue
			}

			// Convert to registry entry
			sourceEntry, sourceID := m.convertSourceYAML(&old)
			if sourceEntry == nil {
				continue
			}

			// Add to registry if not exists
			if !registry.HasSource(sourceID) {
				registry.AddSource(sourceID, *sourceEntry)
			}

			// Add item to installed list
			itemRef := fmt.Sprintf("%s/%s", itemType, entry.Name())
			registry.AddInstalled(sourceID, itemRef)

			// Backup and remove old source.yaml
			backupPath := sourceYAML + ".bak"
			os.Rename(sourceYAML, backupPath)

			count++
		}
	}

	if count > 0 {
		if err := registry.Save(); err != nil {
			return count, err
		}
	}

	return count, nil
}

// convertSourceYAML converts old source.yaml to registry Source
func (m *SourceMigrator) convertSourceYAML(old *oldSourceManifest) (*source.Source, string) {
	if old.Type == "github" && old.GitHub.Owner != "" {
		sourceID := fmt.Sprintf("%s/%s", old.GitHub.Owner, old.GitHub.Repo)
		return &source.Source{
			Registry: "github",
			Provider: "git",
			URL:      fmt.Sprintf("https://github.com/%s/%s", old.GitHub.Owner, old.GitHub.Repo),
			Path:     m.paths.SourceDir(sourceID),
			Ref:      old.GitHub.Ref,
			Commit:   old.GitHub.Commit,
			Updated:  time.Now(),
		}, sourceID
	}

	if old.Type == "plugin" && old.Plugin.Owner != "" {
		sourceID := fmt.Sprintf("%s/%s", old.Plugin.Owner, old.Plugin.Repo)
		return &source.Source{
			Registry: "manual",
			Provider: "git",
			URL:      fmt.Sprintf("https://github.com/%s/%s", old.Plugin.Owner, old.Plugin.Repo),
			Path:     m.paths.SourceDir(sourceID),
			Ref:      old.Plugin.Version,
			Updated:  time.Now(),
		}, sourceID
	}

	return nil, ""
}

// NeedsMigration checks if any source.yaml migration is needed
func (m *SourceMigrator) NeedsMigration() bool {
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := m.paths.HubItemDir(itemType)
		entries, err := os.ReadDir(itemDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			sourceYAML := filepath.Join(itemDir, entry.Name(), "source.yaml")
			if _, err := os.Stat(sourceYAML); err == nil {
				return true
			}
		}
	}
	return false
}
