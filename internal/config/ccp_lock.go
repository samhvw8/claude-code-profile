package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// CcpLockVersion is the current ccp.lock format version.
const CcpLockVersion = 1

const ccpLockHeader = "# Written by ccp: the version each source in ccp.toml is locked to.\n# Do not edit by hand; 'ccp source update' moves it.\n\n"

// CcpLock is ccp.lock: the resolved version of each source in ccp.toml. It
// holds no timestamps, so the same sources give the same bytes on every machine.
type CcpLock struct {
	Version int                     `toml:"version"`
	Sources map[string]LockedSource `toml:"sources,omitempty"`
}

// LockedSource is one source's resolved version.
type LockedSource struct {
	Commit   string `toml:"commit,omitempty"`   // git commit SHA
	Checksum string `toml:"checksum,omitempty"` // for http downloads
}

// CcpLockPath returns where ccp.lock lives: beside the real ccp.toml. When
// ccp.toml is a symlink into a dotfiles repo, the lock goes there too, so the
// pair is synced together.
func CcpLockPath(ccpDir string) string {
	dir := ccpDir
	if real, err := filepath.EvalSymlinks(filepath.Join(ccpDir, "ccp.toml")); err == nil {
		dir = filepath.Dir(real)
	}
	return filepath.Join(dir, "ccp.lock")
}

// LoadCcpLock reads ccp.lock (see CcpLockPath); a missing file is an empty lock.
func LoadCcpLock(ccpDir string) (*CcpLock, error) {
	lock := &CcpLock{Version: CcpLockVersion, Sources: map[string]LockedSource{}}
	data, err := os.ReadFile(CcpLockPath(ccpDir))
	if err != nil {
		if os.IsNotExist(err) {
			return lock, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, lock); err != nil {
		return nil, err
	}
	if lock.Sources == nil {
		lock.Sources = map[string]LockedSource{}
	}
	return lock, nil
}

// Save writes ccp.lock, or removes it when no source is locked.
func (l *CcpLock) Save(ccpDir string) error {
	path := CcpLockPath(ccpDir)
	if len(l.Sources) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	l.Version = CcpLockVersion
	data, err := toml.Marshal(l)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(ccpLockHeader), data...), 0644)
}
