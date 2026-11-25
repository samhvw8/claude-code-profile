package source

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/symlink"
)

// ProjectFile is a project's own ccp.toml: the sources whose items are linked
// into the project's .claude/. Its ccp.lock sits beside it (config.CcpLockPath).
// The clones themselves stay in ~/.ccp/sources, shared with the global hub.
const ProjectFile = "ccp.toml"

// projectMarker is the first line of a project ccp.toml that ccp wrote. A
// ccp.toml without it belongs to something else and is never read or written.
const projectMarker = "# Sources linked into this project's .claude/ by 'ccp project install --link'."

const projectHeader = projectMarker + "\n" +
	"# Machine-local: the links point into ~/.ccp/sources. ccp.lock beside this file holds the commits.\n\n"

type projectSource struct {
	Registry  string   `toml:"registry,omitempty"`
	Provider  string   `toml:"provider"`
	URL       string   `toml:"url"`
	Ref       string   `toml:"ref,omitempty"`
	Installed []string `toml:"installed,omitempty"`
}

type projectConfig struct {
	Sources map[string]projectSource `toml:"sources"`
}

// LoadProjectRegistry reads <root>/ccp.toml and <root>/ccp.lock into a
// Registry whose Save writes them back. A missing file is an empty registry.
// The file is edited by hand, so every entry is checked: a source id must name
// a directory directly under ~/.ccp/sources, a ref or url cannot pass for a git
// option, and an installed entry must be a plain type/name.
func LoadProjectRegistry(root string, paths *config.Paths) (*Registry, error) {
	r := NewRegistry(paths.CcpDir)
	r.projectRoot = root

	path := filepath.Join(root, ProjectFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(string(data), projectMarker) {
		r.projectForeign = true
		return r, nil
	}
	var pc projectConfig
	if err := toml.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	lock, err := config.LoadCcpLock(root)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", config.CcpLockPath(root), err)
	}

	for id, ps := range pc.Sources {
		if !ValidSourceID(paths, id) {
			return nil, fmt.Errorf("%s: invalid source id %q", path, id)
		}
		if strings.HasPrefix(ps.URL, "-") || !validRef(ps.Ref) {
			return nil, fmt.Errorf("%s: source %s: url or ref starts with '-'", path, id)
		}
		for _, item := range ps.Installed {
			if !validItem(item) {
				return nil, fmt.Errorf("%s: source %s: invalid installed item %q", path, id, item)
			}
		}
		locked := lock.Sources[id]
		r.Sources[id] = Source{
			Registry:  ps.Registry,
			Provider:  ps.Provider,
			URL:       ps.URL,
			Path:      paths.SourceDir(id),
			Ref:       ps.Ref,
			Commit:    locked.Commit,
			Checksum:  locked.Checksum,
			Installed: ps.Installed,
		}
	}
	return r, nil
}

// ValidSourceID reports whether id names a clone directly under the sources
// directory (no ".", "..", or empty name after "/" becomes "--").
func ValidSourceID(paths *config.Paths, id string) bool {
	if id == "" || strings.ContainsAny(id, "\\\x00") {
		return false
	}
	dir := paths.SourceDir(id)
	base := filepath.Base(dir)
	return base != "." && base != ".." && filepath.Dir(dir) == paths.SourcesDir()
}

// IsProject reports whether the registry is a project's ccp.toml.
func (r *Registry) IsProject() bool {
	return r.projectRoot != ""
}

// ProjectFileForeign reports that the project root holds a ccp.toml that ccp
// did not write (no marker line). It is loaded as empty and never saved.
func (r *Registry) ProjectFileForeign() bool {
	return r.projectForeign
}

// saveProject writes the project's ccp.toml and ccp.lock. With no sources left
// it removes both.
func (r *Registry) saveProject() error {
	if r.projectForeign {
		return fmt.Errorf("%s was not written by ccp (its first line is not %q): refusing to overwrite it",
			filepath.Join(r.projectRoot, ProjectFile), projectMarker)
	}
	pc := projectConfig{Sources: make(map[string]projectSource, len(r.Sources))}
	lock := &config.CcpLock{Sources: make(map[string]config.LockedSource)}
	for id, src := range r.Sources {
		pc.Sources[id] = projectSource{
			Registry:  src.Registry,
			Provider:  src.Provider,
			URL:       src.URL,
			Ref:       src.Ref,
			Installed: src.Installed,
		}
		if src.Commit != "" || src.Checksum != "" {
			lock.Sources[id] = config.LockedSource{Commit: src.Commit, Checksum: src.Checksum}
		}
	}
	if err := lock.Save(r.projectRoot); err != nil {
		return err
	}
	path := filepath.Join(r.projectRoot, ProjectFile)
	if len(pc.Sources) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := toml.Marshal(&pc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(projectHeader), data...), 0644)
}

// LinkToDir links items of a source into targetDir (a project's .claude/) as
// absolute symlinks into the source clone, so they follow the clone. The link
// is absolute because a project lives outside ~/.ccp and moves on its own.
// It replaces only an earlier ccp link, never a real file or directory.
func (i *Installer) LinkToDir(sourceID string, items []string, targetDir string, allowedTypes map[string]bool) ([]string, error) {
	src, err := i.registry.GetSource(sourceID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(src.Path); err != nil {
		return nil, &SourceError{Op: "link", Source: sourceID, Err: fmt.Errorf("source not downloaded: %s", src.Path)}
	}

	var linked []string
	for _, item := range items {
		srcPath, dstItem, err := i.resolveItemPaths(src.Path, item)
		if err != nil {
			return linked, &SourceError{Op: "link", Source: sourceID, Err: err}
		}
		if !validItem(dstItem) {
			return linked, &SourceError{Op: "link", Source: sourceID, Err: fmt.Errorf("invalid item: %q", dstItem)}
		}
		if _, err := os.Stat(srcPath); err != nil {
			return linked, &SourceError{Op: "link", Source: sourceID, Err: fmt.Errorf("item not found: %s", item)}
		}
		itemType, _, _ := strings.Cut(dstItem, "/")
		if allowedTypes != nil && !allowedTypes[itemType] {
			return linked, &SourceError{Op: "link", Source: sourceID, Err: fmt.Errorf("type not allowed for project install: %s", itemType)}
		}
		if err := i.linkProject(srcPath, filepath.Join(targetDir, filepath.FromSlash(dstItem))); err != nil {
			return linked, &SourceError{Op: "link", Source: sourceID, Err: err}
		}
		linked = append(linked, dstItem)
	}
	return linked, nil
}

// LinkInstalled recreates the link of an installed item (a hub name, as
// ccp.toml lists it) in targetDir.
func (i *Installer) LinkInstalled(sourceID, dstItem, targetDir string) error {
	src, err := i.registry.GetSource(sourceID)
	if err != nil {
		return err
	}
	srcPath, err := i.resolveInstalled(src.Path, dstItem)
	if err != nil {
		return err
	}
	return i.linkProject(srcPath, filepath.Join(targetDir, filepath.FromSlash(dstItem)))
}

func (i *Installer) linkProject(srcPath, dstPath string) error {
	if err := i.checkContained(srcPath); err != nil {
		return err
	}
	if _, err := os.Lstat(dstPath); err == nil {
		if !i.IsShadow(dstPath) {
			return fmt.Errorf("%s exists and is not a ccp link: move or remove it first", dstPath)
		}
		if err := os.Remove(dstPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	return os.Symlink(symlink.RealPath(srcPath), dstPath)
}

// projectLedger lists the project roots that link items on this machine, so
// 'ccp source remove' can tell whether a clone is still linked. It lives in
// ~/.ccp/state, which is machine-local, like the links themselves.
type projectLedger struct {
	Projects []string `toml:"projects"`
}

func projectLedgerPath(paths *config.Paths) string {
	return filepath.Join(paths.CcpDir, "state", "project-links.toml")
}

func loadProjectLedger(paths *config.Paths) (*projectLedger, error) {
	var l projectLedger
	data, err := os.ReadFile(projectLedgerPath(paths))
	if os.IsNotExist(err) {
		return &l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", projectLedgerPath(paths), err)
	}
	return &l, nil
}

// RecordProject adds root to the machine's list of projects with links.
func RecordProject(paths *config.Paths, root string) error {
	l, err := loadProjectLedger(paths)
	if err != nil {
		return err
	}
	if slices.Contains(l.Projects, root) {
		return nil
	}
	l.Projects = append(l.Projects, root)
	slices.Sort(l.Projects)
	data, err := toml.Marshal(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(projectLedgerPath(paths)), 0755); err != nil {
		return err
	}
	return os.WriteFile(projectLedgerPath(paths), data, 0644)
}

// ProjectsUsing lists the recorded projects whose ccp.toml still lists
// sourceID. A project that is gone, or no longer lists it, does not count; one
// whose ccp.toml cannot be read counts, so a clone is never deleted on a guess.
func ProjectsUsing(paths *config.Paths, sourceID string) ([]string, error) {
	l, err := loadProjectLedger(paths)
	if err != nil {
		return nil, err
	}
	var using []string
	for _, root := range l.Projects {
		project, err := LoadProjectRegistry(root, paths)
		if err != nil || project.HasSource(sourceID) {
			using = append(using, root)
		}
	}
	return using, nil
}
