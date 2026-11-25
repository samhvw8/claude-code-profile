package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

func projectPaths(t *testing.T) *config.Paths {
	t.Helper()
	dir := t.TempDir()
	return &config.Paths{CcpDir: dir, HubDir: filepath.Join(dir, "hub")}
}

func TestProjectRegistry_RoundTrip(t *testing.T) {
	paths := projectPaths(t)
	root := t.TempDir()
	r, err := LoadProjectRegistry(root, paths)
	if err != nil || r.SourceCount() != 0 || !r.IsProject() {
		t.Fatalf("empty load: %v, %d sources", err, r.SourceCount())
	}
	commit := strings.Repeat("a", 40)
	r.AddSource("owner/repo", Source{Provider: "git", URL: "https://github.com/owner/repo.git", Ref: "main",
		Commit: commit, Installed: []string{"skills/x"}})
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(root, ProjectFile))
	if strings.Contains(string(data), "path") || strings.Contains(string(data), commit) {
		t.Errorf("ccp.toml holds a path or commit:\n%s", data)
	}
	got, err := LoadProjectRegistry(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := got.GetSource("owner/repo")
	if src.Commit != commit || src.Path != paths.SourceDir("owner/repo") || src.Ref != "main" {
		t.Errorf("reloaded = %+v", src)
	}

	got.RemoveSource("owner/repo")
	if err := got.Save(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{ProjectFile, "ccp.lock"} {
		if _, err := os.Stat(filepath.Join(root, f)); !os.IsNotExist(err) {
			t.Errorf("%s left after the last source went", f)
		}
	}
}

func TestProjectRegistry_RejectsUnsafeEntries(t *testing.T) {
	cases := map[string]string{
		"dot-dot id":   "[sources.'..']\nprovider = 'git'\nurl = 'u'\n",
		"item escapes": "[sources.'o/r']\nprovider = 'git'\nurl = 'u'\ninstalled = ['skills/../../x']\n",
		"option ref":   "[sources.'o/r']\nprovider = 'git'\nurl = 'u'\nref = '--upload-pack=x'\n",
		"option url":   "[sources.'o/r']\nprovider = 'git'\nurl = '-x'\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.WriteFile(filepath.Join(root, ProjectFile), []byte(projectMarker+"\n"+body), 0644)
			if _, err := LoadProjectRegistry(root, projectPaths(t)); err == nil {
				t.Error("loaded an unsafe entry")
			}
		})
	}
}
