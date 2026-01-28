//go:build !windows

package symlink

import (
	"os"
	"path/filepath"
)

// createSymlink creates a symlink on Unix systems
func createSymlink(source, target string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		return err
	}
	return os.Symlink(target, source)
}

// swapSymlink atomically swaps a symlink on Unix systems
// Uses rename which is atomic on POSIX systems
func swapSymlink(path, newTarget string) error {
	// Create a temporary symlink
	tmpLink := path + ".tmp"

	// Remove any existing temp link
	os.Remove(tmpLink)

	// Create new symlink at temp location
	if err := os.Symlink(newTarget, tmpLink); err != nil {
		return err
	}

	// Atomically rename temp link to final path
	if err := os.Rename(tmpLink, path); err != nil {
		os.Remove(tmpLink)
		return err
	}

	return nil
}
