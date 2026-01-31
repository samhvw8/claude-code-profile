//go:build !windows

package symlink

import (
	"os"
	"path/filepath"
)

// createSymlink creates a symlink on Unix systems using relative paths
func createSymlink(source, target string) error {
	// Ensure parent directory exists
	sourceDir := filepath.Dir(source)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return err
	}

	// Compute relative path from symlink location to target
	relTarget, err := filepath.Rel(sourceDir, target)
	if err != nil {
		// Fall back to absolute path if relative fails
		relTarget = target
	}

	return os.Symlink(relTarget, source)
}

// swapSymlink atomically swaps a symlink on Unix systems
// Uses rename which is atomic on POSIX systems
func swapSymlink(path, newTarget string) error {
	// Create a temporary symlink
	tmpLink := path + ".tmp"

	// Remove any existing temp link
	os.Remove(tmpLink)

	// Compute relative path from symlink location to target
	pathDir := filepath.Dir(path)
	relTarget, err := filepath.Rel(pathDir, newTarget)
	if err != nil {
		// Fall back to absolute path if relative fails
		relTarget = newTarget
	}

	// Create new symlink at temp location
	if err := os.Symlink(relTarget, tmpLink); err != nil {
		return err
	}

	// Atomically rename temp link to final path
	if err := os.Rename(tmpLink, path); err != nil {
		os.Remove(tmpLink)
		return err
	}

	return nil
}
