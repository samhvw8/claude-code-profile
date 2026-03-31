package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewGitHubSource(t *testing.T) {
	sm := NewGitHubSource("owner", "repo", "main", "abc123", "skills/my-skill")

	if sm.Type != SourceTypeGitHub {
		t.Errorf("Type = %q, want %q", sm.Type, SourceTypeGitHub)
	}
	if sm.GitHub == nil {
		t.Fatal("GitHub should not be nil")
	}
	if sm.GitHub.Owner != "owner" {
		t.Errorf("Owner = %q, want %q", sm.GitHub.Owner, "owner")
	}
	if sm.GitHub.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", sm.GitHub.Repo, "repo")
	}
	if sm.GitHub.Ref != "main" {
		t.Errorf("Ref = %q, want %q", sm.GitHub.Ref, "main")
	}
	if sm.GitHub.Commit != "abc123" {
		t.Errorf("Commit = %q, want %q", sm.GitHub.Commit, "abc123")
	}
	if sm.GitHub.Path != "skills/my-skill" {
		t.Errorf("Path = %q, want %q", sm.GitHub.Path, "skills/my-skill")
	}
	if sm.InstalledAt.IsZero() {
		t.Error("InstalledAt should not be zero")
	}
}

func TestNewPluginSource(t *testing.T) {
	sm := NewPluginSource("my-plugin", "owner", "repo", "1.0.0")

	if sm.Type != SourceTypePlugin {
		t.Errorf("Type = %q, want %q", sm.Type, SourceTypePlugin)
	}
	if sm.Plugin == nil {
		t.Fatal("Plugin should not be nil")
	}
	if sm.Plugin.Name != "my-plugin" {
		t.Errorf("Name = %q, want %q", sm.Plugin.Name, "my-plugin")
	}
	if sm.Plugin.Owner != "owner" {
		t.Errorf("Owner = %q, want %q", sm.Plugin.Owner, "owner")
	}
	if sm.Plugin.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", sm.Plugin.Version, "1.0.0")
	}
}

func TestGitHubSource_RepoURL(t *testing.T) {
	g := &GitHubSource{Owner: "samhoang", Repo: "ccp"}
	want := "https://github.com/samhoang/ccp"
	if got := g.RepoURL(); got != want {
		t.Errorf("RepoURL() = %q, want %q", got, want)
	}
}

func TestSourceManifest_CanUpdate(t *testing.T) {
	tests := []struct {
		name     string
		manifest *SourceManifest
		want     bool
	}{
		{"github", &SourceManifest{Type: SourceTypeGitHub}, true},
		{"plugin", &SourceManifest{Type: SourceTypePlugin}, true},
		{"local", &SourceManifest{Type: SourceTypeLocal}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.manifest.CanUpdate(); got != tt.want {
				t.Errorf("CanUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceManifest_SourceInfo(t *testing.T) {
	tests := []struct {
		name     string
		manifest *SourceManifest
		want     string
	}{
		{
			"github with details",
			&SourceManifest{Type: SourceTypeGitHub, GitHub: &GitHubSource{Owner: "alice", Repo: "tools"}},
			"alice/tools",
		},
		{
			"github without details",
			&SourceManifest{Type: SourceTypeGitHub},
			"github",
		},
		{
			"plugin with details",
			&SourceManifest{Type: SourceTypePlugin, Plugin: &PluginSource{Name: "my-plugin"}},
			"plugin:my-plugin",
		},
		{
			"plugin without details",
			&SourceManifest{Type: SourceTypePlugin},
			"plugin",
		},
		{
			"local",
			&SourceManifest{Type: SourceTypeLocal},
			"local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.manifest.SourceInfo(); got != tt.want {
				t.Errorf("SourceInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSourceManifest_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	sm := NewGitHubSource("owner", "repo", "main", "abc123", "skills/test")
	if err := sm.Save(tmpDir); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(filepath.Join(tmpDir, "source.yaml")); err != nil {
		t.Fatalf("source.yaml not created: %v", err)
	}

	loaded, err := LoadSourceManifest(tmpDir)
	if err != nil {
		t.Fatalf("LoadSourceManifest() error: %v", err)
	}

	if loaded.Type != SourceTypeGitHub {
		t.Errorf("Type = %q, want %q", loaded.Type, SourceTypeGitHub)
	}
	if loaded.GitHub.Owner != "owner" {
		t.Errorf("Owner = %q, want %q", loaded.GitHub.Owner, "owner")
	}
}

func TestLoadSourceManifest_NotFound(t *testing.T) {
	_, err := LoadSourceManifest(t.TempDir())
	if err == nil {
		t.Error("expected error for missing source.yaml")
	}
}
