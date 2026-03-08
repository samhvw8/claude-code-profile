package migration

import (
	"os"

	"github.com/samhoang/ccp/internal/config"
)

// StructureMigrator ensures required directory structure exists
type StructureMigrator struct {
	paths *config.Paths
}

// NewStructureMigrator creates a new structure migrator
func NewStructureMigrator(paths *config.Paths) *StructureMigrator {
	return &StructureMigrator{paths: paths}
}

// NeedsMigration checks if engines or contexts directories are missing
func (m *StructureMigrator) NeedsMigration() bool {
	for _, dir := range m.requiredDirs() {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// Migrate creates missing directories
func (m *StructureMigrator) Migrate() (int, error) {
	count := 0
	for _, dir := range m.requiredDirs() {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func (m *StructureMigrator) requiredDirs() []string {
	return []string{m.paths.EnginesDir, m.paths.ContextsDir}
}
