package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/claudemd"
	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

// LinkedDirMigrator scans profiles for CLAUDE.md @imports, moves referenced
// directories to hub/rules for reusability, and creates root-level symlinks.
type LinkedDirMigrator struct {
	paths  *config.Paths
	symMgr *symlink.Manager
}

// NewLinkedDirMigrator creates a new linked dir migrator
func NewLinkedDirMigrator(paths *config.Paths) *LinkedDirMigrator {
	return &LinkedDirMigrator{
		paths:  paths,
		symMgr: symlink.New(),
	}
}

// NeedsMigration checks if any profiles have untracked CLAUDE.md linked dirs
func (m *LinkedDirMigrator) NeedsMigration() bool {
	entries, _ := os.ReadDir(m.paths.ProfilesDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := filepath.Join(m.paths.ProfilesDir, entry.Name())
		claudeMD := filepath.Join(profileDir, "CLAUDE.md")

		dirs, err := claudemd.LinkedDirs(claudeMD)
		if err != nil || len(dirs) == 0 {
			continue
		}

		manifest, err := profile.LoadManifest(profile.ManifestPath(profileDir))
		if err != nil {
			continue
		}

		if len(manifest.LinkedDirs) == 0 {
			return true
		}
	}
	return false
}

// Migrate moves CLAUDE.md-referenced directories to hub/rules and creates symlinks
func (m *LinkedDirMigrator) Migrate() (int, error) {
	count := 0

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

		profileDir := filepath.Join(m.paths.ProfilesDir, entry.Name())
		claudeMD := filepath.Join(profileDir, "CLAUDE.md")

		dirs, err := claudemd.LinkedDirs(claudeMD)
		if err != nil || len(dirs) == 0 {
			continue
		}

		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		if len(manifest.LinkedDirs) > 0 {
			continue // Already tracked
		}

		var migratedDirs []string
		for _, dir := range dirs {
			dirPath := filepath.Join(profileDir, dir)
			info, err := os.Lstat(dirPath)
			if err != nil {
				continue
			}

			hubDst := m.paths.HubItemPath(config.HubRules, dir)

			// If it's already a symlink, just track it
			if info.Mode()&os.ModeSymlink != 0 {
				migratedDirs = append(migratedDirs, dir)
				manifest.AddHubItem(config.HubRules, dir)
				continue
			}

			if !info.IsDir() {
				continue
			}

			// Move real directory to hub/rules/ if not already there
			if _, err := os.Stat(hubDst); os.IsNotExist(err) {
				if err := moveItem(dirPath, hubDst); err != nil {
					return count, fmt.Errorf("failed to move %s to hub: %w", dir, err)
				}
			} else {
				// Hub already has it, just remove the profile copy
				os.RemoveAll(dirPath)
			}

			// Create root-level symlink
			if err := m.symMgr.Create(dirPath, hubDst); err != nil {
				return count, fmt.Errorf("failed to create root symlink for %s: %w", dir, err)
			}

			// Create rules/ symlink if not already present
			rulesLink := filepath.Join(profileDir, string(config.HubRules), dir)
			if _, err := os.Lstat(rulesLink); os.IsNotExist(err) {
				if err := m.symMgr.Create(rulesLink, hubDst); err != nil {
					return count, fmt.Errorf("failed to create rules symlink for %s: %w", dir, err)
				}
			}

			manifest.AddHubItem(config.HubRules, dir)
			migratedDirs = append(migratedDirs, dir)
		}

		if len(migratedDirs) == 0 {
			continue
		}

		manifest.LinkedDirs = migratedDirs
		if err := manifest.Save(manifestPath); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}
