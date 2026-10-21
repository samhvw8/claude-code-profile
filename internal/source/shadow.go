package source

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/symlink"
)

// A hub item installed from a source is a shadow: a symlink into the source's
// clone, not a copy. ccp.toml already holds everything needed to rebuild it, so
// the hub stores real content only for items that have no source — local items.
// A shadow follows `ccp source update` with no extra step.
//
// The guarantees that protect edits made through a shadow (LocalEdits, the
// commit lock) need git; a source from another provider has neither.

// InstallMode says how Install places an item in the hub.
type InstallMode int

const (
	// Shadow links the item into the source clone and tracks it in installed.
	Shadow InstallMode = iota
	// Clone copies the item into the hub as an untracked local item. An
	// existing shadow is converted; an existing tracked copy is untracked.
	Clone
)

// ItemState says how a hub item listed in a source's `installed` is stored.
type ItemState int

const (
	ItemMissing     ItemState = iota // nothing in the hub, or a broken shadow
	ItemShadow                       // symlink into the source clone
	ItemCopySame                     // real copy, identical to the source
	ItemCopyDiffers                  // real copy that differs: a local edit or an older version
	ItemUnresolved                   // real copy whose source path cannot be found
	ItemForeignLink                  // symlink to somewhere outside sources/: the user's own
)

// Leftovers of an interrupted Reshadow or clone. The leading dot keeps the
// scanner from listing them as hub items; doctor sweeps them.
const (
	reshadowSuffix = ".ccp-reshadow"
	cloneSuffix    = ".ccp-clone"
)

func leftoverPath(dstPath, suffix string) string {
	return filepath.Join(filepath.Dir(dstPath), "."+filepath.Base(dstPath)+suffix)
}

// validItem reports whether item is a plain "type/name" path that stays inside
// the hub. ccp.toml is synced between machines and edited by hand, so an
// installed entry is not trusted to be one.
func validItem(item string) bool {
	itemType, name, ok := strings.Cut(item, "/")
	if !ok || itemType == "" || name == "" || filepath.IsAbs(item) {
		return false
	}
	for part := range strings.SplitSeq(item, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// linkShadow makes dstPath a relative symlink to srcPath. It refuses an item
// that leaves the sources directory — the item itself, or a symlink inside it —
// since an upstream repo could otherwise point a hub item at ~/.ssh.
func (i *Installer) linkShadow(srcPath, dstPath string) error {
	if err := i.checkContained(srcPath); err != nil {
		return err
	}
	return symlink.CreateRelative(dstPath, srcPath)
}

// checkContained fails when srcPath, or any symlink under it, resolves outside
// the sources directory.
func (i *Installer) checkContained(srcPath string) error {
	sources := symlink.RealPath(i.paths.SourcesDir())
	root := symlink.RealPath(srcPath)
	if !symlink.Within(root, sources) {
		return fmt.Errorf("%s resolves outside the sources directory (to %s): refusing to install it", srcPath, root)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if target := symlink.RealPath(path); !symlink.Within(target, sources) {
				return fmt.Errorf("%s links outside the sources directory (to %s): refusing to install it", path, target)
			}
		}
		return nil
	})
}

// IsShadow reports whether path is a symlink into the sources directory.
func (i *Installer) IsShadow(path string) bool {
	return symlink.PointsInto(path, i.paths.SourcesDir())
}

// resolveInstalled finds the source path of an installed item. installed holds
// hub names, which match the source layout only for plain items: a plugin item
// (skills/<plugin>-<name>) or a URL install (any directory holding SKILL.md) is
// found by matching the hub name each candidate would install as.
func (i *Installer) resolveInstalled(sourceDir, dstItem string) (string, error) {
	if !validItem(dstItem) {
		return "", fmt.Errorf("invalid item: %q", dstItem)
	}
	if src, dst, err := i.resolveItemPaths(sourceDir, dstItem); err == nil && dst == dstItem {
		return src, nil
	}
	for _, spec := range i.DiscoverItems(sourceDir) {
		if src, dst, err := i.resolveItemPaths(sourceDir, spec); err == nil && dst == dstItem {
			return src, nil
		}
	}
	if itemType, name, _ := strings.Cut(dstItem, "/"); itemType == "skills" {
		var found string
		filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
			if found != "" {
				return filepath.SkipAll
			}
			if err != nil {
				return nil // unreadable: skip it
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.IsDir() && rootSkillName(path) == name {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("item not found in source: %s", dstItem)
}

// State reports how an installed item of sourceID is stored in the hub.
func (i *Installer) State(sourceID, item string) ItemState {
	if !validItem(item) {
		return ItemUnresolved
	}
	dstPath := filepath.Join(i.paths.HubDir, filepath.FromSlash(item))
	info, err := os.Lstat(dstPath)
	if err != nil {
		return ItemMissing
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if !i.IsShadow(dstPath) {
			return ItemForeignLink
		}
		if _, err := os.Stat(dstPath); err != nil {
			return ItemMissing
		}
		return ItemShadow
	}
	sourceDir := i.paths.SourceDir(sourceID)
	srcPath, err := i.resolveInstalled(sourceDir, item)
	if err != nil {
		return ItemUnresolved
	}
	same, err := sameTree(dstPath, srcPath, gitIgnored(sourceDir, srcPath))
	if err != nil {
		return ItemUnresolved
	}
	if same {
		return ItemCopySame
	}
	return ItemCopyDiffers
}

// Reshadow replaces a hub copy of an installed item with a shadow link. It acts
// only when the copy is identical to the source: a copy that differs holds a
// local edit, and that must never be lost silently.
func (i *Installer) Reshadow(sourceID, item string) error {
	if state := i.State(sourceID, item); state != ItemCopySame {
		return fmt.Errorf("%s is not an unchanged copy of %s", item, sourceID)
	}
	srcPath, err := i.resolveInstalled(i.paths.SourceDir(sourceID), item)
	if err != nil {
		return err
	}
	if err := i.checkContained(srcPath); err != nil {
		return err
	}
	dstPath := filepath.Join(i.paths.HubDir, filepath.FromSlash(item))

	// Move the copy aside first, so a failed link leaves it restorable.
	aside := leftoverPath(dstPath, reshadowSuffix)
	if err := os.Rename(dstPath, aside); err != nil {
		return err
	}
	if err := i.linkShadow(srcPath, dstPath); err != nil {
		os.Rename(aside, dstPath)
		return err
	}
	return os.RemoveAll(aside)
}

// cloneShadow turns the shadow at dstPath into a local item: a real copy of its
// target that is no longer tracked in installed. Edits made through the shadow
// live in the clone; they move into the copy, and then the item's path in the
// clone is reset so `source update` is not blocked by them. The reset comes
// last: if it fails, the edits are already safe in the copy.
func (i *Installer) cloneShadow(sourceID, sourceDir, dstItem, dstPath string) error {
	target := symlink.RealPath(dstPath)
	rel, err := filepath.Rel(symlink.RealPath(sourceDir), target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is a shadow of another source, not %s", dstItem, sourceID)
	}

	tmp := leftoverPath(dstPath, cloneSuffix)
	os.RemoveAll(tmp)
	if err := CopyTree(target, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Remove(dstPath); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		return fmt.Errorf("the copy is at %s, move it to %s: %w", tmp, dstPath, err)
	}
	i.registry.RemoveInstalled(sourceID, dstItem)

	if err := discardGitChanges(sourceDir, rel); err != nil {
		return fmt.Errorf("%s is now a local item, but its path in the clone was not reset (source update stays blocked until it is): %w", dstItem, err)
	}
	return nil
}

// treeEntry is one file or directory in a tree being compared.
type treeEntry struct {
	path string
	mode fs.FileMode
}

// sameTree reports whether two files or directory trees hold the same content:
// the same files and directories (empty ones included), file modes and bytes.
// Symlinks are followed (CopyDir resolves them), and .git is skipped on both
// sides (CopyDir never copies it). Entries only in a that ignored reports
// (nil: none) do not count.
func sameTree(a, b string, ignored func(extra map[string]treeEntry) map[string]bool) (bool, error) {
	ta, err := treeEntries(a)
	if err != nil {
		return false, err
	}
	tb, err := treeEntries(b)
	if err != nil {
		return false, err
	}
	if ignored != nil {
		extra := map[string]treeEntry{}
		for rel, e := range ta {
			if _, ok := tb[rel]; !ok {
				extra[rel] = e
			}
		}
		for rel := range ignored(extra) {
			delete(ta, rel)
		}
	}
	if len(ta) != len(tb) {
		return false, nil
	}
	for rel, ea := range ta {
		eb, ok := tb[rel]
		if !ok || ea.mode != eb.mode {
			return false, nil
		}
		if ea.mode.IsDir() {
			continue
		}
		da, err := os.ReadFile(ea.path)
		if err != nil {
			return false, err
		}
		db, err := os.ReadFile(eb.path)
		if err != nil {
			return false, err
		}
		if !bytes.Equal(da, db) {
			return false, nil
		}
	}
	return true, nil
}

// gitIgnored returns a sameTree filter that reports the entries the git clone
// at sourceDir ignores, read as paths under srcPath: files a shadow would hold
// as well (a __pycache__ written when a skill ran), so not local edits. Not a
// git clone, or git fails: it reports nothing, and every extra file counts.
func gitIgnored(sourceDir, srcPath string) func(map[string]treeEntry) map[string]bool {
	return func(extra map[string]treeEntry) map[string]bool {
		if len(extra) == 0 || !isGitRepo(sourceDir) {
			return nil
		}
		base, err := filepath.Rel(symlink.RealPath(sourceDir), symlink.RealPath(srcPath))
		if err != nil {
			return nil
		}
		byPath := map[string]string{}
		var input strings.Builder
		for rel, e := range extra {
			path := filepath.ToSlash(filepath.Join(base, rel))
			if e.mode.IsDir() {
				path += "/" // a pattern like "__pycache__/" matches directories only
			}
			byPath[path] = rel
			input.WriteString(path + "\x00")
		}
		cmd := exec.Command("git", "-C", sourceDir, "check-ignore", "--stdin", "-z")
		cmd.Stdin = strings.NewReader(input.String())
		out, _ := cmd.Output() // exit 1 means nothing is ignored
		ignored := map[string]bool{}
		for path := range strings.SplitSeq(string(out), "\x00") {
			if rel, ok := byPath[path]; ok {
				ignored[rel] = true
			}
		}
		return ignored
	}
}

// treeEntries maps every regular file and directory under root (root itself
// when it is a file) to its path and mode, keyed by the path relative to root.
// A directory symlink that loops back into the tree is an error, not a hang.
func treeEntries(root string) (map[string]treeEntry, error) {
	entries := map[string]treeEntry{}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		entries["."] = treeEntry{root, info.Mode()}
		return entries, nil
	}
	visiting := map[string]bool{}
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		real := symlink.RealPath(dir)
		if visiting[real] {
			return fmt.Errorf("symlink loop at %s", dir)
		}
		visiting[real] = true
		defer delete(visiting, real)

		list, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range list {
			if entry.Name() == ".git" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			key := filepath.Join(rel, entry.Name())
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			switch {
			case info.IsDir():
				entries[key] = treeEntry{path, fs.ModeDir}
				if err := walk(path, key); err != nil {
					return err
				}
			case info.Mode().IsRegular():
				entries[key] = treeEntry{path, info.Mode().Perm()}
			}
		}
		return nil
	}
	return entries, walk(root, "")
}

// isGitRepo reports whether dir is the top of a git work tree.
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// LocalEdits lists the changes in a source clone that a reset would discard or
// that sit inside an installed shadow: every change to a tracked file, and any
// untracked file under an installed item's path. Untracked files elsewhere (a
// stray .DS_Store) survive a reset and do not count. Only git sources have
// local edits to report.
func (i *Installer) LocalEdits(sourceID string) ([]string, error) {
	src, err := i.registry.GetSource(sourceID)
	if err != nil {
		return nil, err
	}
	if src.Path == "" {
		return nil, fmt.Errorf("source %s has no path in ccp.toml", sourceID)
	}
	if !isGitRepo(src.Path) {
		return nil, nil
	}
	out, err := exec.Command("git", "-C", src.Path, "status", "--porcelain", "-z").Output()
	if err != nil {
		return nil, &SourceError{Op: "git status", Source: src.Path, Err: err}
	}

	var scopes []string
	sourceReal := symlink.RealPath(src.Path)
	for _, item := range src.Installed {
		if srcPath, err := i.resolveInstalled(src.Path, item); err == nil {
			if rel, err := filepath.Rel(sourceReal, symlink.RealPath(srcPath)); err == nil {
				scopes = append(scopes, filepath.ToSlash(rel))
			}
		}
	}
	inScope := func(path string) bool {
		for _, scope := range scopes {
			if scope == "." || path == scope || strings.HasPrefix(path, scope+"/") {
				return true
			}
		}
		return false
	}

	var edits []string
	fields := strings.Split(string(out), "\x00")
	for n := 0; n < len(fields); n++ {
		entry := fields[n]
		if len(entry) < 4 {
			continue
		}
		status, path := entry[:2], entry[3:]
		if status[0] == 'R' || status[0] == 'C' {
			n++ // the original path follows as its own field
		}
		if status != "??" || inScope(path) {
			edits = append(edits, path)
		}
	}
	return edits, nil
}

// discardGitChanges resets one path of a git source clone to HEAD and removes
// untracked files under it. It does nothing outside a git clone.
func discardGitChanges(sourceDir, rel string) error {
	if !isGitRepo(sourceDir) {
		return nil
	}
	run := func(args ...string) error {
		if out, err := exec.Command("git", append([]string{"-C", sourceDir}, args...)...).CombinedOutput(); err != nil {
			return &SourceError{Op: "git " + args[0], Source: sourceDir, Err: fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))}
		}
		return nil
	}
	// checkout fails on a path HEAD does not have; such a path is all untracked.
	if out, _ := exec.Command("git", "-C", sourceDir, "ls-tree", "--name-only", "HEAD", "--", rel).Output(); len(bytes.TrimSpace(out)) > 0 {
		if err := run("checkout", "HEAD", "--", rel); err != nil {
			return err
		}
	}
	return run("clean", "-fdq", "--", rel)
}

// shaPattern matches a full SHA-1 or SHA-256 commit id.
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// ValidCommit reports whether commit is a full commit id. ccp.toml is synced
// and hand-edited, so its commit is checked before it reaches git, where a
// value like --upload-pack=… would be read as an option.
func ValidCommit(commit string) bool {
	return shaPattern.MatchString(commit)
}

// validRef rejects a git ref that git would read as an option.
func validRef(ref string) bool {
	return !strings.HasPrefix(ref, "-")
}

// CheckoutCommit moves a git source clone to commit, the one ccp.toml records.
// Clones are shallow, so the commit is fetched by SHA first (GitHub allows it).
func (p *GitProvider) CheckoutCommit(ctx context.Context, sourceDir, commit string) error {
	if !ValidCommit(commit) {
		return &SourceError{Op: "git checkout", Source: sourceDir, Err: fmt.Errorf("not a full commit id: %q", commit)}
	}
	if p.getCommit(sourceDir) == commit {
		return nil
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", sourceDir, "fetch", "--depth", "1", "origin", commit).CombinedOutput(); err != nil {
		return &SourceError{Op: "git fetch", Source: sourceDir, Err: fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))}
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", sourceDir, "reset", "--hard", commit, "--").CombinedOutput(); err != nil {
		return &SourceError{Op: "git reset", Source: sourceDir, Err: fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))}
	}
	return nil
}

// SweepLeftovers finds what an interrupted Reshadow or clone left in the hub.
// With fix, a leftover whose item is present is redundant and removed; one
// whose item is missing holds the only copy and is moved back. It returns the
// leftovers found (fixed or not) and the first error.
func SweepLeftovers(hubDir string, fix bool) ([]string, error) {
	var found []string
	typeDirs, err := os.ReadDir(hubDir)
	if err != nil {
		return nil, err
	}
	for _, typeDir := range typeDirs {
		dir := filepath.Join(hubDir, typeDir.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			var item string
			for _, suffix := range []string{reshadowSuffix, cloneSuffix} {
				if strings.HasPrefix(name, ".") && strings.HasSuffix(name, suffix) {
					item = strings.TrimSuffix(strings.TrimPrefix(name, "."), suffix)
				}
			}
			if item == "" {
				continue
			}
			leftover := filepath.Join(dir, name)
			found = append(found, leftover)
			if !fix {
				continue
			}
			itemPath := filepath.Join(dir, item)
			if _, err := os.Lstat(itemPath); err == nil {
				err = os.RemoveAll(leftover)
			} else {
				err = os.Rename(leftover, itemPath)
			}
			if err != nil {
				return found, err
			}
		}
	}
	return found, nil
}
