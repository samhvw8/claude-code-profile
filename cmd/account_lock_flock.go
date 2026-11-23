//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package cmd

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// lockAccountDir takes an exclusive flock on the account's relink lock and
// returns the func that releases it (safe to call more than once). Go opens
// files close-on-exec, so a started child never inherits the lock and it is
// never held for a session's lifetime.
func lockAccountDir(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, relinkLockFile), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return sync.OnceFunc(func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}), nil
}
