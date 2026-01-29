package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		return fmt.Errorf("failed to read symlink %s: %w (is ccp initialized?)", r.paths.ClaudeDir, err)
	}

	// Get the profile directory permissions to restore later
	profileInfo, err := os.Stat(activeProfile)
	if err != nil {
		return fmt.Errorf("failed to stat profile %s: %w (profile may be corrupted)", activeProfile, err)
	}
	profilePerm := profileInfo.Mode().Perm()

	// Step 2: Create a temporary directory to hold restored content
	tempDir, err := os.MkdirTemp(filepath.Dir(r.paths.ClaudeDir), ".claude-restore-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir in %s: %w (check disk space and permissions)", filepath.Dir(r.paths.ClaudeDir), err)
	}
	defer os.RemoveAll(tempDir) // Clean up on failure

	// Step 3: Copy active profile contents to temp dir, resolving symlinks
	if err := r.copyProfileContents(activeProfile, tempDir); err != nil {
		return fmt.Errorf("failed to copy profile contents from %s: %w", activeProfile, err)
	}

	// Step 4: Remove the symlink
	if err := os.Remove(r.paths.ClaudeDir); err != nil {
		return fmt.Errorf("failed to remove symlink %s: %w (check permissions)", r.paths.ClaudeDir, err)
	}

	// Step 5: Rename temp dir to ~/.claude
	if err := os.Rename(tempDir, r.paths.ClaudeDir); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tempDir, r.paths.ClaudeDir, err)
	}

	// Step 6: Restore the correct permissions (MkdirTemp creates with 0700)
	if err := os.Chmod(r.paths.ClaudeDir, profilePerm); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", r.paths.ClaudeDir, err)
	}

	// Step 6.5: Rewrite settings.json to update hook paths
	// Replace $HOME/.ccp/profiles/<name>/hooks/ with $HOME/.claude/hooks/
	if err := r.rewriteSettingsHooks(activeProfile); err != nil {
		// Warn but don't fail - the reset succeeded
		fmt.Printf("Warning: failed to rewrite settings.json hook paths: %v\n", err)
	}

	// Step 7: Remove ~/.ccp entirely
	if err := os.RemoveAll(r.paths.CcpDir); err != nil {
		return fmt.Errorf("failed to remove ccp dir %s: %w (manual cleanup may be needed)", r.paths.CcpDir, err)
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

// rewriteSettingsHooks rewrites settings.json to update hook paths from profile paths to ~/.claude paths
func (r *Resetter) rewriteSettingsHooks(oldProfileDir string) error {
	settingsPath := filepath.Join(r.paths.ClaudeDir, "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No settings.json, nothing to rewrite
		}
		return err
	}

	content := string(data)

	// Get the profile hooks path pattern to replace
	// e.g., $HOME/.ccp/profiles/default/hooks/ -> $HOME/.claude/hooks/
	home, _ := os.UserHomeDir()

	// Build the old path pattern (with $HOME)
	oldHooksPath := oldProfileDir + "/hooks/"
	if home != "" && strings.HasPrefix(oldHooksPath, home) {
		oldHooksPath = "$HOME" + oldHooksPath[len(home):]
	}

	// Build the new path pattern
	newHooksPath := "$HOME/.claude/hooks/"

	// Replace all occurrences
	content = strings.ReplaceAll(content, oldHooksPath, newHooksPath)

	return os.WriteFile(settingsPath, []byte(content), 0644)
}
