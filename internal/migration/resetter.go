package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhoang/ccp/internal/config"
)

// Resetter handles the reset/uninstall process
type Resetter struct {
	paths *config.Paths
}

// NewResetter creates a new resetter
func NewResetter(paths *config.Paths) *Resetter {
	return &Resetter{paths: paths}
}

// Execute performs the reset operation
func (r *Resetter) Execute() error {
	// Step 1: Get the active profile path (symlink target)
	activeProfile, err := os.Readlink(r.paths.ClaudeDir)
	if err != nil {
		return fmt.Errorf("failed to read symlink: %w", err)
	}

	// Get the profile directory permissions to restore later
	profileInfo, err := os.Stat(activeProfile)
	if err != nil {
		return fmt.Errorf("failed to stat profile: %w", err)
	}
	profilePerm := profileInfo.Mode().Perm()

	// Step 2: Create a temporary directory to hold restored content
	tempDir, err := os.MkdirTemp(filepath.Dir(r.paths.ClaudeDir), ".claude-restore-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // Clean up on failure

	// Step 3: Copy active profile contents to temp dir, resolving symlinks
	if err := r.copyProfileContents(activeProfile, tempDir); err != nil {
		return fmt.Errorf("failed to copy profile contents: %w", err)
	}

	// Step 4: Remove the symlink
	if err := os.Remove(r.paths.ClaudeDir); err != nil {
		return fmt.Errorf("failed to remove symlink: %w", err)
	}

	// Step 5: Rename temp dir to ~/.claude
	if err := os.Rename(tempDir, r.paths.ClaudeDir); err != nil {
		return fmt.Errorf("failed to rename temp dir: %w", err)
	}

	// Step 6: Restore the correct permissions (MkdirTemp creates with 0700)
	if err := os.Chmod(r.paths.ClaudeDir, profilePerm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Step 7: Remove ~/.ccp entirely
	if err := os.RemoveAll(r.paths.CcpDir); err != nil {
		return fmt.Errorf("failed to remove ccp dir: %w", err)
	}

	return nil
}

// copyProfileContents copies profile contents, resolving symlinks to actual files
func (r *Resetter) copyProfileContents(profileDir, destDir string) error {
	return r.copyDirResolvingSymlinks(profileDir, destDir, true)
}

// copyDirResolvingSymlinks copies a directory, resolving symlinks to actual content
func (r *Resetter) copyDirResolvingSymlinks(srcDir, dstDir string, isRoot bool) error {
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dstDir, srcInfo.Mode()); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		// Skip profile.yaml at root - it's ccp-specific
		if isRoot && entry.Name() == "profile.yaml" {
			continue
		}

		// Check if it's a symlink
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - resolve and copy the actual content
			target, err := filepath.EvalSymlinks(srcPath)
			if err != nil {
				// Target doesn't exist, skip
				continue
			}

			targetInfo, err := os.Stat(target)
			if err != nil {
				// Target doesn't exist, skip
				continue
			}

			if targetInfo.IsDir() {
				if err := copyDir(target, dstPath); err != nil {
					return err
				}
			} else {
				if err := copyFile(target, dstPath); err != nil {
					return err
				}
			}
		} else if info.IsDir() {
			// Regular directory - recurse, resolving any symlinks inside
			if err := r.copyDirResolvingSymlinks(srcPath, dstPath, false); err != nil {
				return err
			}
		} else {
			// Regular file - copy
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
