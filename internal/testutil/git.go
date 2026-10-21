// Package testutil holds helpers shared by tests in several packages.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs git in dir with a fixed identity and no global config, failing the
// test on error, and returns the trimmed output.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// CommitAll stages everything in dir, commits it, and returns the new HEAD.
func CommitAll(t *testing.T, dir, msg string) string {
	t.Helper()
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-q", "-m", msg)
	return Git(t, dir, "rev-parse", "HEAD")
}

// WriteFile writes content to path, creating parent directories.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// IsSymlink reports whether path is a symlink.
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
