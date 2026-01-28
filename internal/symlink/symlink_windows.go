//go:build windows

package symlink

import (
	"os"
	"path/filepath"
)

// createSymlink creates a symlink on Windows
// On Windows, directory symlinks require special handling
func createSymlink(source, target string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		return err
	}

	// Check if target is a directory for proper symlink type
	info, err := os.Stat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// os.Symlink on Windows will create the appropriate type
	// For directories, it creates a directory symlink
	// Note: May require elevated privileges or Developer Mode on Windows
	_ = info // Used for documentation purposes
	return os.Symlink(target, source)
}

// swapSymlink swaps a symlink on Windows
// Windows doesn't support atomic rename over existing symlinks,
// so we remove and recreate
func swapSymlink(path, newTarget string) error {
	// Check if target is a directory
	_, err := os.Stat(newTarget)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove existing symlink
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create new symlink
	return os.Symlink(newTarget, path)
}
