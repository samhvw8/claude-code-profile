package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

// legacyFragment represents an old setting-fragment YAML file
type legacyFragment struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Key         string      `yaml:"key"`
	Value       interface{} `yaml:"value"`
}

// FragmentMigrator converts legacy setting-fragments to settings templates
type FragmentMigrator struct {
	paths *config.Paths
}

// NewFragmentMigrator creates a new fragment migrator
func NewFragmentMigrator(paths *config.Paths) *FragmentMigrator {
	return &FragmentMigrator{paths: paths}
}

// NeedsMigration checks if hub/setting-fragments/ exists and has YAML files
func (m *FragmentMigrator) NeedsMigration() bool {
	fragDir := filepath.Join(m.paths.HubDir, "setting-fragments")
	entries, err := os.ReadDir(fragDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			return true
		}
	}
	return false
}

// Migrate reads all fragment YAML files, merges into a settings template,
// sets the template on profiles that don't have one, and removes the fragments dir.
// Returns the number of fragments merged.
func (m *FragmentMigrator) Migrate() (int, error) {
	fragDir := filepath.Join(m.paths.HubDir, "setting-fragments")

	// Read all fragment files
	entries, err := os.ReadDir(fragDir)
	if err != nil {
		return 0, nil // dir doesn't exist, nothing to do
	}

	settings := make(map[string]interface{})
	count := 0

	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(fragDir, e.Name()))
		if err != nil {
			continue
		}

		var frag legacyFragment
		if err := yaml.Unmarshal(data, &frag); err != nil {
			continue
		}

		if frag.Key != "" {
			settings[frag.Key] = frag.Value
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	// Save as a settings template named "migrated-fragments"
	tmplMgr := hub.NewTemplateManager(m.paths.HubDir)
	tmplName := "migrated-fragments"
	if tmplMgr.Exists(tmplName) {
		// Find unique name
		for i := 2; i < 100; i++ {
			candidate := fmt.Sprintf("%s-%d", tmplName, i)
			if !tmplMgr.Exists(candidate) {
				tmplName = candidate
				break
			}
		}
	}

	tmpl := &hub.Template{Name: tmplName, Settings: settings}
	if err := tmplMgr.Save(tmpl); err != nil {
		return 0, fmt.Errorf("failed to save template: %w", err)
	}

	// Set template on profiles that don't already have one
	profileEntries, err := os.ReadDir(m.paths.ProfilesDir)
	if err == nil {
		for _, pe := range profileEntries {
			if !pe.IsDir() || pe.Name() == "shared" {
				continue
			}
			profileDir := m.paths.ProfileDir(pe.Name())
			manifestPath := profile.ManifestPath(profileDir)
			manifest, err := profile.LoadManifest(manifestPath)
			if err != nil {
				continue
			}
			if manifest.SettingsTemplate == "" {
				manifest.SettingsTemplate = tmplName
				manifest.Save(manifestPath)
			}
		}
	}

	// Remove the fragments directory
	os.RemoveAll(fragDir)

	return count, nil
}
