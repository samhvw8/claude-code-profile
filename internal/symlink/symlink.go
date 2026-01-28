package symlink

import (
	"os"
	"path/filepath"
)

// Manager handles symlink operations
type Manager struct{}

// New creates a new symlink manager
func New() *Manager {
	return &Manager{}
}

// Info contains information about a symlink
type Info struct {
	Path       string
	Target     string
	Exists     bool
	IsSymlink  bool
	IsBroken   bool
	TargetInfo os.FileInfo
}

// Create creates a symlink from source to target
// source: the path where the symlink will be created
// target: the path the symlink points to
func (m *Manager) Create(source, target string) error {
	return createSymlink(source, target)
}

// Remove removes a symlink (not its target)
func (m *Manager) Remove(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrInvalid}
	}
	return os.Remove(path)
}

// Info returns information about a path, resolving symlinks
func (m *Manager) Info(path string) (*Info, error) {
	info := &Info{Path: path}

	// Check if path exists (as symlink or regular file)
	linfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		info.Exists = false
		return info, nil
	}
	if err != nil {
		return nil, err
	}

	info.Exists = true
	info.IsSymlink = linfo.Mode()&os.ModeSymlink != 0

	if info.IsSymlink {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}

		// Resolve relative symlinks
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		info.Target = target

		// Check if target exists
		targetInfo, err := os.Stat(path)
		if os.IsNotExist(err) {
			info.IsBroken = true
		} else if err == nil {
			info.TargetInfo = targetInfo
		}
	}

	return info, nil
}

// Validate checks if a symlink points to the expected target
func (m *Manager) Validate(path, expectedTarget string) (bool, error) {
	info, err := m.Info(path)
	if err != nil {
		return false, err
	}

	if !info.Exists || !info.IsSymlink {
		return false, nil
	}

	// Resolve both paths to absolute for comparison
	absTarget, err := filepath.Abs(info.Target)
	if err != nil {
		return false, err
	}
	absExpected, err := filepath.Abs(expectedTarget)
	if err != nil {
		return false, err
	}

	return absTarget == absExpected, nil
}

// Swap atomically replaces a symlink with a new target
func (m *Manager) Swap(path, newTarget string) error {
	return swapSymlink(path, newTarget)
}

// IsSymlink checks if a path is a symlink
func (m *Manager) IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}

// ReadLink returns the target of a symlink
func (m *Manager) ReadLink(path string) (string, error) {
	return os.Readlink(path)
}

// EnsureDir creates a directory if it doesn't exist
func (m *Manager) EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
