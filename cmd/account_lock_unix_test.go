//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// tryLock takes and drops the account's relink lock without waiting; it
// fails while another open file holds it.
func tryLock(t *testing.T, path string) error {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// A pooled pick holds the account's lock from prepare until the process has
// started, and never after: a second locker gets it at once while the child
// still runs.
func TestPrepareRunCmd_PoolLockReleasedAfterStart(t *testing.T) {
	paths, _ := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 10, "c2": 50}, nil)
	ra, err := parseRunArgs([]string{"a", "--account", "auto:p", "--", "sleep", "1"})
	if err != nil {
		t.Fatal(err)
	}
	var execCmd *exec.Cmd
	var release func()
	if _, err := stderrOf(t, func() error {
		var err error
		execCmd, release, err = prepareRunCmd(paths, ra)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(paths.AccountDir("c1"), relinkLockFile)
	if err := tryLock(t, lock); err == nil {
		t.Error("the lock was not held between prepare and start")
	}
	if err := execCmd.Start(); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	release() // safe twice
	if err := tryLock(t, lock); err != nil {
		t.Errorf("lock still held after start: %v", err)
	}
	if err := execCmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("child is not running while the lock is free: %v", err)
	}
	if err := execCmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

// Through the real command: the started child itself can take the lock, so
// ccp released it and the child did not inherit it.
func TestRun_PoolChildDoesNotHoldLock(t *testing.T) {
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("perl not found")
	}
	paths, _ := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 10, "c2": 50}, nil)
	lock := filepath.Join(paths.AccountDir("c1"), relinkLockFile)
	script := `use Fcntl ":flock"; open(my $f, "<", $ARGV[0]) or exit 2; flock($f, LOCK_EX|LOCK_NB) or exit 3; exit 0`
	if _, err := stderrOf(t, func() error { return runCcp(t, "run", "a", "--account", "auto:p", "--", perl, "-e", script, lock) }); err != nil {
		t.Errorf("child could not take the lock: %v", err)
	}
}

// Every Claude LinkAccount takes the account's lock: a named launcher waits
// for a pooled relink in progress, then sees its new binding (and refuses,
// since the account is now linked to another profile).
func TestActivationDir_WaitsForRelinkLock(t *testing.T) {
	paths, _ := poolEnv(t)
	release, err := lockAccountDir(paths.AccountDir("c1"))
	if err != nil {
		t.Fatal(err)
	}
	// The first launcher relinks c1 to b under the lock.
	if _, _, err := profile.LinkAccount(paths, "c1", "b", true); err != nil {
		t.Fatal(err)
	}
	type result struct {
		dir string
		err error
	}
	done := make(chan result, 1)
	go func() {
		dir, rel, err := lockedActivationDir(paths, "a", "c1", true, false)
		rel()
		done <- result{dir, err}
	}()
	select {
	case r := <-done:
		t.Fatalf("second launcher did not wait for the lock: %+v", r)
	case <-time.After(200 * time.Millisecond):
	}
	release()
	select {
	case r := <-done:
		if r.err == nil || !strings.Contains(r.err.Error(), `linked to profile "b"`) {
			t.Errorf("after the lock: %+v, want the new binding's refusal", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second launcher still blocked after release")
	}
}

// 'ccp account pool' rewrites the binding under the same lock.
func TestAccountPool_TakesLock(t *testing.T) {
	paths, _ := poolEnv(t)
	release, err := lockAccountDir(paths.AccountDir("c3"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- setPoolLocked(paths, "c3", "q") }()
	select {
	case err := <-done:
		t.Fatalf("pool set did not wait for the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if a := profile.ReadAccount(paths, "c3"); a.Pool != "q" {
		t.Errorf("pool = %q", a.Pool)
	}
}
