package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

// TemplateMigrator converts setting-fragments to settings templates
type TemplateMigrator struct {
	paths *config.Paths
}

// NewTemplateMigrator creates a new template migrator
func NewTemplateMigrator(paths *config.Paths) *TemplateMigrator {
	return &TemplateMigrator{paths: paths}
}

// NeedsMigration checks if any profiles or engines still use setting-fragments
func (m *TemplateMigrator) NeedsMigration() bool {
	// Check profiles
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		if len(manifest.Hub.SettingFragments) > 0 && manifest.SettingsTemplate == "" {
			return true
		}
	}

	// Check engines
	enginesEntries, err := os.ReadDir(m.paths.EnginesDir)
	if err != nil {
		return false
	}
	for _, entry := range enginesEntries {
		if !entry.IsDir() {
			continue
		}
		engine, err := profile.LoadEngine(m.paths.EngineDir(entry.Name()))
		if err != nil {
			continue
		}
		if len(engine.Hub.SettingFragments) > 0 && engine.SettingsTemplate == "" {
			return true
		}
	}

	return false
}

// Migrate converts all setting-fragment references to settings templates.
// For each profile/engine with fragments:
//  1. Merges fragments into a single settings map
//  2. Saves as a settings template
//  3. Updates manifest/engine to reference the template
//  4. Clears the setting-fragments list
//
// Returns the number of profiles + engines migrated.
func (m *TemplateMigrator) Migrate() (int, error) {
	tmplMgr := hub.NewTemplateManager(m.paths.HubDir)
	migrated := 0

	// Migrate profiles
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		if len(manifest.Hub.SettingFragments) == 0 || manifest.SettingsTemplate != "" {
			continue
		}

		// Merge fragments into settings map
		settings, err := MergeSettingFragments(m.paths.HubDir, manifest.Hub.SettingFragments)
		if err != nil {
			return migrated, fmt.Errorf("failed to merge fragments for profile %s: %w", entry.Name(), err)
		}

		// Choose a unique template name
		tmplName := m.uniqueTemplateName(tmplMgr, entry.Name())

		// Save as template
		tmpl := &hub.Template{Name: tmplName, Settings: settings}
		if err := tmplMgr.Save(tmpl); err != nil {
			return migrated, fmt.Errorf("failed to save template for profile %s: %w", entry.Name(), err)
		}

		// Update manifest
		manifest.SettingsTemplate = tmplName
		manifest.Hub.SettingFragments = nil

		if err := manifest.Save(manifestPath); err != nil {
			return migrated, fmt.Errorf("failed to update manifest for profile %s: %w", entry.Name(), err)
		}

		migrated++
	}

	// Migrate engines
	enginesEntries, err := os.ReadDir(m.paths.EnginesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return migrated, nil
		}
		return migrated, err
	}

	for _, entry := range enginesEntries {
		if !entry.IsDir() {
			continue
		}

		engineDir := m.paths.EngineDir(entry.Name())
		engine, err := profile.LoadEngine(engineDir)
		if err != nil {
			continue
		}

		if len(engine.Hub.SettingFragments) == 0 || engine.SettingsTemplate != "" {
			continue
		}

		// Merge fragments into settings map
		settings, err := MergeSettingFragments(m.paths.HubDir, engine.Hub.SettingFragments)
		if err != nil {
			return migrated, fmt.Errorf("failed to merge fragments for engine %s: %w", entry.Name(), err)
		}

		// Choose a unique template name
		tmplName := m.uniqueTemplateName(tmplMgr, "engine-"+entry.Name())

		// Save as template
		tmpl := &hub.Template{Name: tmplName, Settings: settings}
		if err := tmplMgr.Save(tmpl); err != nil {
			return migrated, fmt.Errorf("failed to save template for engine %s: %w", entry.Name(), err)
		}

		// Update engine
		engine.SettingsTemplate = tmplName
		engine.Hub.SettingFragments = nil

		if err := engine.Save(engineDir); err != nil {
			return migrated, fmt.Errorf("failed to update engine %s: %w", entry.Name(), err)
		}

		migrated++
	}

	return migrated, nil
}

// Summary returns a human-readable summary of what needs migration
func (m *TemplateMigrator) Summary() string {
	var profiles, engines []string

	entries, _ := os.ReadDir(m.paths.ProfilesDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		if len(manifest.Hub.SettingFragments) > 0 && manifest.SettingsTemplate == "" {
			fragments := sortedCopy(manifest.Hub.SettingFragments)
			profiles = append(profiles, fmt.Sprintf("%s (%s)", entry.Name(), strings.Join(fragments, ", ")))
		}
	}

	enginesEntries, _ := os.ReadDir(m.paths.EnginesDir)
	for _, entry := range enginesEntries {
		if !entry.IsDir() {
			continue
		}
		engine, err := profile.LoadEngine(filepath.Join(m.paths.EnginesDir, entry.Name()))
		if err != nil {
			continue
		}
		if len(engine.Hub.SettingFragments) > 0 && engine.SettingsTemplate == "" {
			fragments := sortedCopy(engine.Hub.SettingFragments)
			engines = append(engines, fmt.Sprintf("%s (%s)", entry.Name(), strings.Join(fragments, ", ")))
		}
	}

	var parts []string
	if len(profiles) > 0 {
		parts = append(parts, fmt.Sprintf("profiles: %s", strings.Join(profiles, ", ")))
	}
	if len(engines) > 0 {
		parts = append(parts, fmt.Sprintf("engines: %s", strings.Join(engines, ", ")))
	}
	return strings.Join(parts, "; ")
}

// uniqueTemplateName generates a unique template name, appending a suffix if needed
func (m *TemplateMigrator) uniqueTemplateName(tmplMgr *hub.TemplateManager, base string) string {
	if !tmplMgr.Exists(base) {
		return base
	}
	for i := 2; i < 100; i++ {
		name := fmt.Sprintf("%s-%d", base, i)
		if !tmplMgr.Exists(name) {
			return name
		}
	}
	return base // shouldn't happen
}

func sortedCopy(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	sort.Strings(c)
	return c
}
