package profile

import (
	"fmt"
	"os"
	"path/filepath"

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
