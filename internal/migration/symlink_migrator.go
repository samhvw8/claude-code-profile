package migration

import (
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// SymlinkMigrator handles migration of absolute symlinks to relative
type SymlinkMigrator struct {
	paths *config.Paths
}

// NewSymlinkMigrator creates a new symlink migrator
func NewSymlinkMigrator(paths *config.Paths) *SymlinkMigrator {
	return &SymlinkMigrator{paths: paths}
}

// MigrateSymlinks converts absolute symlinks to relative in all profiles
func (m *SymlinkMigrator) MigrateSymlinks() (int, error) {
	count := 0

	// Migrate ~/.claude symlink
	if migrated, err := m.migrateSymlink(m.paths.ClaudeDir); err != nil {
		return count, err
	} else if migrated {
		count++
	}

	// Migrate symlinks in each profile
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return count, nil
		}
		return count, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := filepath.Join(m.paths.ProfilesDir, entry.Name())
		profileCount, err := m.migrateProfileSymlinks(profileDir)
		if err != nil {
			return count, err
		}
		count += profileCount
	}

	return count, nil
}

// migrateProfileSymlinks migrates all symlinks in a profile directory
func (m *SymlinkMigrator) migrateProfileSymlinks(profileDir string) (int, error) {
	count := 0

	// Item type directories that may contain symlinks to hub
	itemTypes := []string{"skills", "agents", "hooks", "rules", "commands", "setting-fragments"}

	for _, itemType := range itemTypes {
		typeDir := filepath.Join(profileDir, itemType)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			linkPath := filepath.Join(typeDir, entry.Name())
			if migrated, err := m.migrateSymlink(linkPath); err != nil {
				return count, err
			} else if migrated {
				count++
			}
		}
	}

	// Data directories that may be symlinks to shared
	dataDirs := []string{"tasks", "todos", "paste-cache", "history", "file-history", "session-env", "projects", "plans"}

	for _, dataDir := range dataDirs {
		linkPath := filepath.Join(profileDir, dataDir)
		if migrated, err := m.migrateSymlink(linkPath); err != nil {
			return count, err
		} else if migrated {
			count++
		}
	}

	return count, nil
}

// migrateSymlink converts an absolute symlink to relative
func (m *SymlinkMigrator) migrateSymlink(path string) (bool, error) {
	// Check if it's a symlink
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}

	// Read current target
	target, err := os.Readlink(path)
	if err != nil {
		return false, err
	}

	// Skip if already relative
	if !filepath.IsAbs(target) {
		return false, nil
	}

	// Compute relative path
	linkDir := filepath.Dir(path)
	relTarget, err := filepath.Rel(linkDir, target)
	if err != nil {
		return false, nil // Can't compute relative, skip
	}

	// Remove old symlink and create new one with relative path
	if err := os.Remove(path); err != nil {
		return false, err
	}

	if err := os.Symlink(relTarget, path); err != nil {
		// Try to restore absolute symlink on failure
		os.Symlink(target, path)
		return false, err
	}

	return true, nil
}

// NeedsMigration checks if any absolute symlinks need migration
func (m *SymlinkMigrator) NeedsMigration() bool {
	// Check ~/.claude
	if m.isAbsoluteSymlink(m.paths.ClaudeDir) {
		return true
	}

	// Check profile directories
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := filepath.Join(m.paths.ProfilesDir, entry.Name())
		if m.profileHasAbsoluteSymlinks(profileDir) {
			return true
		}
	}

	return false
}

// profileHasAbsoluteSymlinks checks if a profile has any absolute symlinks
func (m *SymlinkMigrator) profileHasAbsoluteSymlinks(profileDir string) bool {
	itemTypes := []string{"skills", "agents", "hooks", "rules", "commands", "setting-fragments"}

	for _, itemType := range itemTypes {
		typeDir := filepath.Join(profileDir, itemType)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if m.isAbsoluteSymlink(filepath.Join(typeDir, entry.Name())) {
				return true
			}
		}
	}

	dataDirs := []string{"tasks", "todos", "paste-cache", "history", "file-history", "session-env", "projects", "plans"}
	for _, dataDir := range dataDirs {
		if m.isAbsoluteSymlink(filepath.Join(profileDir, dataDir)) {
			return true
		}
	}

	return false
}

// isAbsoluteSymlink checks if a path is a symlink with an absolute target
func (m *SymlinkMigrator) isAbsoluteSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}

	target, err := os.Readlink(path)
	if err != nil {
		return false
	}

	return filepath.IsAbs(target)
}
