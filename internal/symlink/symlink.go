package symlink

import (
	"os"
	"path/filepath"
	"strings"
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

// RealPath resolves symlinks in a path, falling back to the cleaned path when
// part of it does not exist yet.
func RealPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// CreateRelative makes a relative symlink at linkPath pointing at target. Both
// the link's parent and the target are resolved first, so the link still
// resolves when a parent directory is itself a symlink (a dotfiles-managed
// ~/.ccp or ~/.omp). The link therefore points at target's real location.
func CreateRelative(linkPath, target string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	rel, err := filepath.Rel(RealPath(filepath.Dir(linkPath)), RealPath(target))
	if err != nil {
		return err
	}
	return os.Symlink(rel, linkPath)
}

// Within reports whether path lies strictly inside dir (textually).
func Within(path, dir string) bool {
	return strings.HasPrefix(path, filepath.Clean(dir)+string(filepath.Separator))
}

// PointsInto reports whether path is a symlink whose target lies inside dir,
// whether or not the target exists. A relative target is read against the
// link's textual parent and against its resolved parent (CreateRelative writes
// the latter), and either side may sit behind a symlink.
func PointsInto(path, dir string) bool {
	raw, err := os.Readlink(path)
	if err != nil {
		return false
	}
	candidates := []string{raw}
	if !filepath.IsAbs(raw) {
		candidates = []string{
			filepath.Join(filepath.Dir(path), raw),
			filepath.Join(RealPath(filepath.Dir(path)), raw),
		}
	}
	realDir := RealPath(dir)
	for _, target := range candidates {
		target = filepath.Clean(target)
		if Within(target, dir) {
			return true
		}
		// Resolve the target's directory, which exists even when the item is gone.
		parent, err := filepath.EvalSymlinks(filepath.Dir(target))
		if err == nil && Within(filepath.Join(parent, filepath.Base(target)), realDir) {
			return true
		}
	}
	return false
}
