package source

import (
	"os"
	"path/filepath"
)

// Behind is an installed item that a source update does not reach.
type Behind struct {
	Item  string
	State ItemState // never ItemShadow
	Gone  bool      // the clone no longer has the item: its shadow is broken
}

// FollowUpdate runs after a source update. A shadow whose item moved upstream
// (skills/x to .claude/skills/x) is broken; it is pointed at the item's new
// path. Every other installed item that does not follow the clone — a copy,
// the user's own link, an item missing from the hub or gone upstream — is
// returned untouched, so the update can say which items stayed behind.
func (i *Installer) FollowUpdate(sourceID string) (relinked []string, behind []Behind, err error) {
	src, err := i.registry.GetSource(sourceID)
	if err != nil {
		return nil, nil, err
	}
	sourceDir := i.paths.SourceDir(sourceID)
	for _, item := range src.Installed {
		state := i.State(sourceID, item)
		switch state {
		case ItemShadow:
			continue
		case ItemMissing:
			dstPath := filepath.Join(i.paths.HubDir, filepath.FromSlash(item))
			if _, lerr := os.Lstat(dstPath); lerr != nil || !i.IsShadow(dstPath) {
				break // not in the hub: 'ccp install' recreates it
			}
			srcPath, rerr := i.resolveInstalled(sourceDir, item)
			if rerr != nil {
				behind = append(behind, Behind{Item: item, State: state, Gone: true})
				continue
			}
			if err := os.Remove(dstPath); err != nil {
				return relinked, behind, err
			}
			if err := i.linkShadow(srcPath, dstPath); err != nil {
				return relinked, behind, &SourceError{Op: "relink", Source: sourceID, Err: err}
			}
			relinked = append(relinked, item)
			continue
		}
		behind = append(behind, Behind{Item: item, State: state})
	}
	return relinked, behind, nil
}
