package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

// MigrationPlan describes what will be migrated
type MigrationPlan struct {
	HubItems    map[config.HubItemType][]string
	FilesToCopy []string // CLAUDE.md, settings.json, etc.
	DataDirs    []string // tasks, todos, etc.
}

// Migrator handles the init migration process
type Migrator struct {
	paths    *config.Paths
	symMgr   *symlink.Manager
	rollback *Rollback
}

// NewMigrator creates a new migrator
func NewMigrator(paths *config.Paths) *Migrator {
	return &Migrator{
		paths:    paths,
		symMgr:   symlink.New(),
		rollback: NewRollback(),
	}
}

// Plan creates a migration plan by scanning the existing claude directory
func (m *Migrator) Plan() (*MigrationPlan, error) {
	plan := &MigrationPlan{
		HubItems: make(map[config.HubItemType][]string),
	}

	// Scan for hub-eligible items
	scanner := hub.NewScanner()
	h, err := scanner.ScanSource(m.paths.ClaudeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan source: %w", err)
	}

	for itemType, items := range h.Items {
		for _, item := range items {
			plan.HubItems[itemType] = append(plan.HubItems[itemType], item.Name)
		}
	}

	// Check for files to copy
	filesToCheck := []string{"CLAUDE.md", "settings.json", "settings.local.json"}
	for _, f := range filesToCheck {
		if _, err := os.Stat(filepath.Join(m.paths.ClaudeDir, f)); err == nil {
			plan.FilesToCopy = append(plan.FilesToCopy, f)
		}
	}

	// Check for data directories
	for _, dataType := range config.AllDataItemTypes() {
		dataDir := filepath.Join(m.paths.ClaudeDir, string(dataType))
		if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
			plan.DataDirs = append(plan.DataDirs, string(dataType))
		}
	}

	return plan, nil
}

// Execute performs the migration
func (m *Migrator) Execute(plan *MigrationPlan, dryRun bool) error {
	if dryRun {
		return nil
	}

	// Step 1: Create ccp directory structure
	if err := os.MkdirAll(m.paths.CcpDir, 0755); err != nil {
		return m.rollbackAndReturn(fmt.Errorf("failed to create ccp dir: %w", err))
	}
	m.rollback.AddDir(m.paths.CcpDir)

	// Step 2: Create hub directory structure
	if err := m.createHubStructure(); err != nil {
		return m.rollbackAndReturn(err)
	}

	// Step 3: Move hub items to hub directory
	if err := m.moveHubItems(plan); err != nil {
		return m.rollbackAndReturn(err)
	}

	// Step 4: Create profiles directory
	if err := os.MkdirAll(m.paths.ProfilesDir, 0755); err != nil {
		return m.rollbackAndReturn(fmt.Errorf("failed to create profiles dir: %w", err))
	}
	m.rollback.AddDir(m.paths.ProfilesDir)

	// Step 5: Create shared directory
	if err := os.MkdirAll(m.paths.SharedDir, 0755); err != nil {
		return m.rollbackAndReturn(fmt.Errorf("failed to create shared dir: %w", err))
	}
	m.rollback.AddDir(m.paths.SharedDir)

	// Step 6: Create default profile
	if err := m.createDefaultProfile(plan); err != nil {
		return m.rollbackAndReturn(err)
	}

	// Step 7: Replace ~/.claude with symlink to default profile
	if err := m.activateDefaultProfile(); err != nil {
		return m.rollbackAndReturn(err)
	}

	return nil
}

// createHubStructure creates the hub directory and subdirectories
func (m *Migrator) createHubStructure() error {
	if err := os.MkdirAll(m.paths.HubDir, 0755); err != nil {
		return fmt.Errorf("failed to create hub dir: %w", err)
	}
	m.rollback.AddDir(m.paths.HubDir)

	for _, itemType := range config.AllHubItemTypes() {
		dir := m.paths.HubItemDir(itemType)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create hub/%s: %w", itemType, err)
		}
	}

	return nil
}

// moveHubItems moves items from source to hub
func (m *Migrator) moveHubItems(plan *MigrationPlan) error {
	for itemType, items := range plan.HubItems {
		for _, itemName := range items {
			src := filepath.Join(m.paths.ClaudeDir, string(itemType), itemName)
			dst := m.paths.HubItemPath(itemType, itemName)

			if err := m.moveItem(src, dst); err != nil {
				return fmt.Errorf("failed to move %s/%s: %w", itemType, itemName, err)
			}

			m.rollback.AddMove(dst, src)
		}
	}

	return nil
}

// moveItem moves a file or directory
func (m *Migrator) moveItem(src, dst string) error {
	// Try rename first (fastest, works on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + remove
	if err := copyRecursive(src, dst); err != nil {
		return err
	}

	return os.RemoveAll(src)
}

// createDefaultProfile creates the default profile from existing config
func (m *Migrator) createDefaultProfile(plan *MigrationPlan) error {
	defaultDir := m.paths.ProfileDir("default")

	// Preserve original ~/.claude permissions if it exists, otherwise use 0755
	profilePerm := os.FileMode(0755)
	if info, err := os.Stat(m.paths.ClaudeDir); err == nil {
		profilePerm = info.Mode().Perm()
	}

	if err := os.MkdirAll(defaultDir, profilePerm); err != nil {
		return fmt.Errorf("failed to create default profile dir: %w", err)
	}
	m.rollback.AddDir(defaultDir)

	// Create manifest
	manifest := profile.NewManifest("default", "Migrated from original ~/.claude")

	// Add all hub items to manifest
	for itemType, items := range plan.HubItems {
		manifest.SetHubItems(itemType, items)
	}

	// Create hub item directories and symlinks
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(defaultDir, string(itemType))

		// Preserve original subdir permissions if it exists
		srcItemDir := filepath.Join(m.paths.ClaudeDir, string(itemType))
		itemPerm := os.FileMode(0755)
		if info, err := os.Stat(srcItemDir); err == nil {
			itemPerm = info.Mode().Perm()
		}

		if err := os.MkdirAll(itemDir, itemPerm); err != nil {
			return err
		}

		for _, itemName := range manifest.GetHubItems(itemType) {
			hubPath := m.paths.HubItemPath(itemType, itemName)
			linkPath := filepath.Join(itemDir, itemName)
			if err := m.symMgr.Create(linkPath, hubPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", linkPath, err)
			}
		}
	}

	// Copy config files
	for _, f := range plan.FilesToCopy {
		src := filepath.Join(m.paths.ClaudeDir, f)
		dst := filepath.Join(defaultDir, f)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy %s: %w", f, err)
		}
	}

	// Handle data directories
	for _, dataType := range config.AllDataItemTypes() {
		srcDir := filepath.Join(m.paths.ClaudeDir, string(dataType))
		dstDir := filepath.Join(defaultDir, string(dataType))

		// Get source permissions if it exists
		var srcInfo os.FileInfo
		srcExists := false
		if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
			srcExists = true
			srcInfo = info
		}

		mode := manifest.GetDataShareMode(dataType)

		if mode == config.ShareModeShared {
			// Create shared directory with source permissions or default
			sharedDir := m.paths.SharedDataDir(dataType)
			sharedPerm := os.FileMode(0755)
			if srcExists {
				sharedPerm = srcInfo.Mode().Perm()
			}
			if err := os.MkdirAll(sharedDir, sharedPerm); err != nil {
				return err
			}

			// Move existing data to shared if exists
			if srcExists {
				// Copy contents to shared
				entries, err := os.ReadDir(srcDir)
				if err != nil {
					return err
				}
				for _, entry := range entries {
					src := filepath.Join(srcDir, entry.Name())
					dst := filepath.Join(sharedDir, entry.Name())
					if err := m.moveItem(src, dst); err != nil {
						return err
					}
				}
			}

			// Create symlink in profile
			if err := m.symMgr.Create(dstDir, sharedDir); err != nil {
				return err
			}
		} else {
			// Isolated: move or create directory
			if srcExists {
				if err := m.moveItem(srcDir, dstDir); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(dstDir, 0755); err != nil {
					return err
				}
			}
		}
	}

	// Save manifest
	manifestPath := filepath.Join(defaultDir, "profile.yaml")
	if err := manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	return nil
}

// activateDefaultProfile replaces ~/.claude with a symlink to the default profile
func (m *Migrator) activateDefaultProfile() error {
	defaultProfileDir := m.paths.ProfileDir("default")

	// Remove the original ~/.claude directory (now empty of migrated items)
	// First, clean up any remaining empty directories
	if err := m.cleanupSourceDir(); err != nil {
		return fmt.Errorf("failed to cleanup source dir: %w", err)
	}

	// Move any remaining unknown directories/files to the new profile
	// This preserves user data that isn't in our known types
	if err := m.moveRemainingItems(defaultProfileDir); err != nil {
		return fmt.Errorf("failed to move remaining items: %w", err)
	}

	// Remove the source directory (should be empty now)
	if err := os.RemoveAll(m.paths.ClaudeDir); err != nil {
		return fmt.Errorf("failed to remove source dir: %w", err)
	}

	// Create symlink ~/.claude -> ~/.ccp/profiles/default
	if err := m.symMgr.Create(m.paths.ClaudeDir, defaultProfileDir); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// moveRemainingItems moves any remaining files/dirs from source to profile
// This handles unknown directories (like debug/) that aren't in our known types
func (m *Migrator) moveRemainingItems(profileDir string) error {
	entries, err := os.ReadDir(m.paths.ClaudeDir)
	if err != nil {
		return err
	}

	// Build set of known names to skip (already handled)
	knownNames := make(map[string]bool)
	for _, t := range config.AllHubItemTypes() {
		knownNames[string(t)] = true
	}
	for _, t := range config.AllDataItemTypes() {
		knownNames[string(t)] = true
	}
	// Also skip files we already copied
	knownNames["CLAUDE.md"] = true
	knownNames["settings.json"] = true
	knownNames["settings.local.json"] = true
	knownNames["profile.yaml"] = true

	for _, entry := range entries {
		if knownNames[entry.Name()] {
			continue
		}

		src := filepath.Join(m.paths.ClaudeDir, entry.Name())
		dst := filepath.Join(profileDir, entry.Name())

		// Move unknown item to profile
		if err := m.moveItem(src, dst); err != nil {
			return fmt.Errorf("failed to move %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// cleanupSourceDir removes empty directories from the source claude dir
func (m *Migrator) cleanupSourceDir() error {
	// Remove empty hub item directories
	for _, itemType := range config.AllHubItemTypes() {
		dir := filepath.Join(m.paths.ClaudeDir, string(itemType))
		// Try to remove - will fail if not empty, which is fine
		os.Remove(dir)
	}

	// Remove empty data directories
	for _, dataType := range config.AllDataItemTypes() {
		dir := filepath.Join(m.paths.ClaudeDir, string(dataType))
		os.Remove(dir)
	}

	return nil
}

func (m *Migrator) rollbackAndReturn(err error) error {
	if rbErr := m.rollback.Execute(); rbErr != nil {
		return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
	}
	return err
}

// copyFile copies a single file preserving permissions
func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

// copyRecursive copies a file or directory recursively
func copyRecursive(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

// copyDir copies a directory recursively preserving permissions
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
