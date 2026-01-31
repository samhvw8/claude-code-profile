//go:build windows

package symlink

import (
	"os"
	"path/filepath"
)

// createSymlink creates a symlink on Windows using relative paths
// On Windows, directory symlinks require special handling
func createSymlink(source, target string) error {
	// Ensure parent directory exists
	sourceDir := filepath.Dir(source)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return err
	}

	// Check if target is a directory for proper symlink type
	info, err := os.Stat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Compute relative path from symlink location to target
	relTarget, err := filepath.Rel(sourceDir, target)
	if err != nil {
		// Fall back to absolute path if relative fails
		relTarget = target
	}

	// os.Symlink on Windows will create the appropriate type
	// For directories, it creates a directory symlink
	// Note: May require elevated privileges or Developer Mode on Windows
	_ = info // Used for documentation purposes
	return os.Symlink(relTarget, source)
}

// swapSymlink swaps a symlink on Windows using relative paths
// Windows doesn't support atomic rename over existing symlinks,
// so we remove and recreate
func swapSymlink(path, newTarget string) error {
	// Check if target is a directory
	_, err := os.Stat(newTarget)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Compute relative path from symlink location to target
	pathDir := filepath.Dir(path)
	relTarget, err := filepath.Rel(pathDir, newTarget)
	if err != nil {
		// Fall back to absolute path if relative fails
		relTarget = newTarget
	}

	// Remove existing symlink
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create new symlink
	return os.Symlink(relTarget, path)
}
