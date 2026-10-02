package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// SyncLinks makes a profile's hub item symlinks match its manifest: the leaf
// items it lists (rules link by basename) plus the members of its linked
// bundles. Symlinks nothing expects are removed; real files and directories are
// left alone. logf, when non-nil, is told about each change.
func SyncLinks(paths *config.Paths, p *Profile, logf func(format string, args ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	symMgr := p.symMgr
	if symMgr == nil {
		symMgr = NewManager(paths).symMgr
	}

	// Refuse before changing anything, so a refusal never leaves some types
	// reconciled and the rest not.
	if err := checkProfileItemDirs(p.Path); err != nil {
		return err
	}

	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(p.Path, string(itemType))
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", itemType, err)
		}

		expected := BundleMemberLinks(paths, p.Manifest, itemType)
		for _, name := range p.Manifest.GetHubItems(itemType) {
			linkName := name
			if itemType == config.HubRules {
				linkName = filepath.Base(name)
			}
			expected[linkName] = paths.HubItemPath(itemType, name)
		}

		entries, err := os.ReadDir(itemDir)
		if err != nil {
			return fmt.Errorf("failed to read %s directory: %w", itemType, err)
		}
		for _, entry := range entries {
			if _, ok := expected[entry.Name()]; ok {
				continue
			}
			linkPath := filepath.Join(itemDir, entry.Name())
			if !ccpMayRemove(paths, itemType, linkPath) {
				logf("  Keeping %s: %s (not a hub link — link the style through ccp to manage it)\n", itemType, entry.Name())
				continue
			}
			if isLink, _ := symMgr.IsSymlink(linkPath); isLink {
				logf("  Removing unlinked %s: %s\n", itemType, entry.Name())
				os.Remove(linkPath)
			}
		}

		for linkName, target := range expected {
			linkPath := filepath.Join(itemDir, linkName)
			if _, err := os.Stat(target); err != nil {
				logf("  Warning: hub item not found: %s/%s\n", itemType, linkName)
				continue
			}
			if isLink, _ := symMgr.IsSymlink(linkPath); isLink {
				if ok, _ := symMgr.Validate(linkPath, target); ok {
					continue
				}
				os.Remove(linkPath) // wrong target: recreate
			}
			logf("  Linking %s: %s\n", itemType, linkName)
			if err := symMgr.Create(linkPath, target); err != nil {
				logf("  Warning: failed to create symlink for %s/%s: %v\n", itemType, linkName, err)
			}
		}
	}
	return nil
}

// profileItemDir returns a profile's directory for one hub item type. A symlink
// in its place was made by hand: linking into it would write wherever it
// points — the hub itself, for a link to hub/<type> — so it is refused.
func profileItemDir(profilePath string, itemType config.HubItemType) (string, error) {
	dir := filepath.Join(profilePath, string(itemType))
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a symlink, not a directory ccp manages: remove it (the link only, not its target), then run 'ccp profile sync %s'",
			dir, filepath.Base(profilePath))
	}
	return dir, nil
}

// checkProfileItemDirs runs profileItemDir over every hub item type.
func checkProfileItemDirs(profilePath string) error {
	for _, itemType := range config.AllHubItemTypes() {
		if _, err := profileItemDir(profilePath, itemType); err != nil {
			return err
		}
	}
	return nil
}

// ccpMayRemove reports whether an entry the manifest does not list is ccp's to
// remove. SyncLinks and drift both ask here, so 'profile sync' and 'doctor'
// cannot disagree. output-styles/ was not ccp's before it became a hub type,
// so there only links into the hub qualify.
func ccpMayRemove(paths *config.Paths, itemType config.HubItemType, path string) bool {
	return itemType != config.HubOutputStyles || isHubLink(paths, path)
}

// isHubLink reports whether path is a symlink into the hub (bundles included).
func isHubLink(paths *config.Paths, path string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if within(filepath.Clean(target), paths.HubDir) {
		return true
	}
	// Either side may sit behind a symlink (a linked ~/.ccp or hub). Resolve the
	// target's directory, which exists even when the item itself is gone.
	dir, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return false
	}
	hub, err := filepath.EvalSymlinks(paths.HubDir)
	return err == nil && within(filepath.Join(dir, filepath.Base(target)), hub)
}

func within(path, dir string) bool {
	return strings.HasPrefix(path, filepath.Clean(dir)+string(filepath.Separator))
}
