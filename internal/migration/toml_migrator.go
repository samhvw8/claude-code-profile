package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

// TOMLMigrator handles migration from YAML to TOML
type TOMLMigrator struct {
	paths *config.Paths
}

// NewTOMLMigrator creates a new TOML migrator
func NewTOMLMigrator(paths *config.Paths) *TOMLMigrator {
	return &TOMLMigrator{paths: paths}
}

// MigrateProfiles migrates all profile.yaml files to profile.toml
func (m *TOMLMigrator) MigrateProfiles() ([]string, error) {
	var migrated []string

	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := filepath.Join(m.paths.ProfilesDir, entry.Name())
		yamlPath := filepath.Join(profileDir, "profile.yaml")
		tomlPath := filepath.Join(profileDir, "profile.toml")

		// Skip if already migrated (profile.toml exists)
		if _, err := os.Stat(tomlPath); err == nil {
			continue
		}

		// Check if profile.yaml exists
		if _, err := os.Stat(yamlPath); err != nil {
			continue
		}

		// Load YAML manifest
		manifest, err := profile.LoadManifest(yamlPath)
		if err != nil {
			return migrated, fmt.Errorf("load %s: %w", yamlPath, err)
		}

		// Save as TOML
		if err := manifest.SaveTOML(profileDir); err != nil {
			return migrated, fmt.Errorf("save %s: %w", tomlPath, err)
		}

		// Backup and remove YAML
		backupPath := yamlPath + ".bak"
		if err := os.Rename(yamlPath, backupPath); err != nil {
			// Non-fatal: TOML was saved successfully
			fmt.Printf("Warning: could not backup %s: %v\n", yamlPath, err)
		}

		migrated = append(migrated, entry.Name())
	}

	return migrated, nil
}

// NeedsMigration checks if any profile migration is needed
func (m *TOMLMigrator) NeedsMigration() bool {
	entries, _ := os.ReadDir(m.paths.ProfilesDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		yamlPath := filepath.Join(m.paths.ProfilesDir, entry.Name(), "profile.yaml")
		tomlPath := filepath.Join(m.paths.ProfilesDir, entry.Name(), "profile.toml")
		if _, err := os.Stat(yamlPath); err == nil {
			if _, err := os.Stat(tomlPath); err != nil {
				return true // YAML exists, TOML doesn't
			}
		}
	}
	return false
}

// MigrateProfile migrates a single profile from YAML to TOML
func (m *TOMLMigrator) MigrateProfile(name string) error {
	profileDir := filepath.Join(m.paths.ProfilesDir, name)
	yamlPath := filepath.Join(profileDir, "profile.yaml")
	tomlPath := filepath.Join(profileDir, "profile.toml")

	// Skip if already migrated
	if _, err := os.Stat(tomlPath); err == nil {
		return nil
	}

	// Check if profile.yaml exists
	if _, err := os.Stat(yamlPath); err != nil {
		return nil // No YAML to migrate
	}

	// Load YAML manifest
	manifest, err := profile.LoadManifest(yamlPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Save as TOML
	if err := manifest.SaveTOML(profileDir); err != nil {
		return fmt.Errorf("save TOML: %w", err)
	}

	// Backup YAML
	backupPath := yamlPath + ".bak"
	if err := os.Rename(yamlPath, backupPath); err != nil {
		// Non-fatal
		return nil
	}

	return nil
}

// UpgradeV2ToV3 upgrades all v2 TOML manifests to v3.
// This is a no-op structurally (v2 manifests already work as v3),
// but re-saves to bump the version number.
func (m *TOMLMigrator) UpgradeV2ToV3() ([]string, error) {
	var upgraded []string

	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		tomlPath := filepath.Join(m.paths.ProfilesDir, entry.Name(), "profile.toml")

		manifest, err := profile.LoadManifest(tomlPath)
		if err != nil {
			continue
		}

		if manifest.Version >= profile.ManifestVersion {
			continue // Already v3+
		}

		// Re-save stamps ManifestVersion (3)
		if err := manifest.SaveTOML(filepath.Join(m.paths.ProfilesDir, entry.Name())); err != nil {
			return upgraded, fmt.Errorf("upgrade %s: %w", entry.Name(), err)
		}

		upgraded = append(upgraded, entry.Name())
	}

	return upgraded, nil
}

// NeedsV2ToV3Upgrade checks if any profiles need version upgrade
func (m *TOMLMigrator) NeedsV2ToV3Upgrade() bool {
	entries, _ := os.ReadDir(m.paths.ProfilesDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		tomlPath := filepath.Join(m.paths.ProfilesDir, entry.Name(), "profile.toml")
		manifest, err := profile.LoadManifest(tomlPath)
		if err != nil {
			continue
		}
		if manifest.Version < profile.ManifestVersion {
			return true
		}
	}
	return false
}
