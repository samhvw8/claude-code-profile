package migration

import (
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

// RegistryMigrator handles migration from registry.toml to ccp.toml
type RegistryMigrator struct {
	paths *config.Paths
}

// NewRegistryMigrator creates a new registry migrator
func NewRegistryMigrator(paths *config.Paths) *RegistryMigrator {
	return &RegistryMigrator{paths: paths}
}

// NeedsMigration checks if registry.toml exists and needs migration
func (m *RegistryMigrator) NeedsMigration() bool {
	legacyPath := filepath.Join(m.paths.CcpDir, "registry.toml")
	_, err := os.Stat(legacyPath)
	return err == nil
}

// Migrate moves sources from registry.toml to ccp.toml
func (m *RegistryMigrator) Migrate() (int, error) {
	if !m.NeedsMigration() {
		return 0, nil
	}

	// LoadRegistry handles the migration internally:
	// - Reads from ccp.toml if sources exist there
	// - Falls back to registry.toml if not
	registry, err := source.LoadRegistry(m.paths.RegistryPath())
	if err != nil {
		return 0, err
	}

	count := len(registry.Sources)
	if count == 0 {
		// No sources to migrate, just remove empty registry.toml
		legacyPath := filepath.Join(m.paths.CcpDir, "registry.toml")
		os.Remove(legacyPath)
		return 0, nil
	}

	// Save triggers migration: writes to ccp.toml and removes registry.toml
	if err := registry.Save(); err != nil {
		return 0, err
	}

	return count, nil
}
