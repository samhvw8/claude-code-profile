//go:build unix

package accountusage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/profile"
)

func TestDefaultAgyRunner_TimeoutKillsProcessGroup(t *testing.T) {
	binDir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	a, r, _ := setupAgy(t, "agy1", []byte("tok"), nil)
	r.AgyTimeout = 80 * time.Millisecond
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	// Spawn a child sleep process inheriting stdout so only process-group kill terminates it.
	script := "#!/bin/sh\nsleep 30 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || s.Live || !strings.Contains(s.Note, "timed out") {
		t.Fatalf("expected timeout note, got %+v", s)
	}

	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("child pid not written: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("invalid child pid %q: %v", pidBytes, err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return // child process is gone
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("orphan child process %d survived agy timeout", pid)
}
