//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package cmd

// lockAccountDir has no flock here (Windows, Solaris, illumos, AIX): a pooled
// Claude pick counts the account as lost instead of relinking unlocked, and
// other callers go on without the lock, as before account pools.
func lockAccountDir(string) (func(), error) {
	return nil, errNoFileLock
}
