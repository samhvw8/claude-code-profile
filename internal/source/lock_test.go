package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

// ccp.toml holds what the user chose; the resolved commit goes to ccp.lock,
// and nothing machine-specific (no timestamp) is written to either file.
func TestRegistrySave_SplitsLock(t *testing.T) {
	ccpDir := t.TempDir()
	r := NewRegistry(ccpDir)
	if err := r.AddSource("o/r", Source{Provider: "git", URL: "https://x/o/r.git", Path: filepath.Join(ccpDir, "sources", "o--r"), Ref: "main", Commit: testSHA}); err != nil {
		t.Fatal(err)
	}
	if err := r.AddInstalled("o/r", "skills/x"); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}

	cfg := readFile(t, filepath.Join(ccpDir, "ccp.toml"))
	for _, unwanted := range []string{"commit", "updated", "checksum"} {
		if strings.Contains(cfg, unwanted) {
			t.Errorf("ccp.toml contains %q:\n%s", unwanted, cfg)
		}
	}
	if !strings.Contains(cfg, "skills/x") {
		t.Errorf("ccp.toml lost installed items:\n%s", cfg)
	}
	lock := readFile(t, filepath.Join(ccpDir, "ccp.lock"))
	if !strings.Contains(lock, testSHA) || strings.Contains(lock, "updated") {
		t.Errorf("ccp.lock = \n%s", lock)
	}

	// Saving again without a change writes the same bytes: two machines agree.
	r2, err := LoadRegistry(filepath.Join(ccpDir, "registry.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := r2.GetSource("o/r"); src.Commit != testSHA {
		t.Errorf("commit = %q after reload, want %q", src.Commit, testSHA)
	}
	if err := r2.AddInstalled("o/r", "skills/x"); err != nil {
		t.Fatal(err)
	}
	if err := r2.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(ccpDir, "ccp.toml")); got != cfg {
		t.Errorf("ccp.toml changed on a no-op save:\n%s\nvs\n%s", cfg, got)
	}
}

// A ccp.toml written before ccp.lock still carries commit: it is read, and
// the next save moves it into ccp.lock.
func TestLoadRegistry_MigratesLegacyCommit(t *testing.T) {
	ccpDir := t.TempDir()
	legacy := "[sources.'o/r']\nprovider = 'git'\nurl = 'u'\npath = 'sources/o--r'\ncommit = '" + testSHA + "'\nupdated = 2026-10-21T07:46:31+07:00\ninstalled = ['skills/x']\n"
	if err := os.WriteFile(filepath.Join(ccpDir, "ccp.toml"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRegistry(filepath.Join(ccpDir, "registry.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := r.GetSource("o/r"); src.Commit != testSHA {
		t.Fatalf("commit = %q, want legacy %q", src.Commit, testSHA)
	}
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	if cfg := readFile(t, filepath.Join(ccpDir, "ccp.toml")); strings.Contains(cfg, "commit") || strings.Contains(cfg, "updated") {
		t.Errorf("ccp.toml still has lock data:\n%s", cfg)
	}
	if !strings.Contains(readFile(t, filepath.Join(ccpDir, "ccp.lock")), testSHA) {
		t.Error("ccp.lock missing the migrated commit")
	}
}

// ccp.lock wins over a stale commit left in ccp.toml.
func TestLoadRegistry_LockWins(t *testing.T) {
	ccpDir := t.TempDir()
	other := strings.Repeat("f", 40)
	os.WriteFile(filepath.Join(ccpDir, "ccp.toml"), []byte("[sources.'o/r']\nprovider = 'git'\nurl = 'u'\npath = 'p'\ncommit = '"+other+"'\n"), 0644)
	os.WriteFile(filepath.Join(ccpDir, "ccp.lock"), []byte("version = 1\n[sources.'o/r']\ncommit = '"+testSHA+"'\n"), 0644)
	r, err := LoadRegistry(filepath.Join(ccpDir, "registry.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := r.GetSource("o/r"); src.Commit != testSHA {
		t.Errorf("commit = %q, want the lock's %q", src.Commit, testSHA)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ccp.toml is often a symlink into a dotfiles repo: ccp.lock goes beside the
// real file, so the pair is synced together.
func TestRegistrySave_LockBesideLinkedConfig(t *testing.T) {
	root := t.TempDir()
	ccpDir, dotfiles := filepath.Join(root, "ccp"), filepath.Join(root, "dotfiles")
	os.MkdirAll(ccpDir, 0755)
	os.MkdirAll(dotfiles, 0755)
	os.WriteFile(filepath.Join(dotfiles, "ccp.toml"), []byte("default_registry = 'skills.sh'\n"), 0644)
	if err := os.Symlink(filepath.Join(dotfiles, "ccp.toml"), filepath.Join(ccpDir, "ccp.toml")); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(ccpDir)
	r.AddSource("o/r", Source{Provider: "git", URL: "u", Path: filepath.Join(ccpDir, "sources", "o--r"), Commit: testSHA})
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(ccpDir, "ccp.lock")); !os.IsNotExist(err) {
		t.Error("ccp.lock written into ~/.ccp, outside the dotfiles repo")
	}
	if !strings.Contains(readFile(t, filepath.Join(dotfiles, "ccp.lock")), testSHA) {
		t.Error("ccp.lock not beside the real ccp.toml")
	}
	r2, err := LoadRegistry(filepath.Join(ccpDir, "registry.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := r2.GetSource("o/r"); src.Commit != testSHA {
		t.Errorf("commit = %q after reload", src.Commit)
	}
}
