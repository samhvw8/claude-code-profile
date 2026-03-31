package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPluginManifest(t *testing.T) {
	github := GitHubSource{Owner: "alice", Repo: "tools", Ref: "main"}
	components := ComponentList{
		Skills: []string{"code-review"},
		Hooks:  []string{"pre-commit"},
	}

	pm := NewPluginManifest("my-plugin", "A test plugin", "1.0.0", github, components)

	if pm.Name != "my-plugin" {
		t.Errorf("Name = %q, want %q", pm.Name, "my-plugin")
	}
	if pm.Description != "A test plugin" {
		t.Errorf("Description = %q, want %q", pm.Description, "A test plugin")
	}
	if pm.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", pm.Version, "1.0.0")
	}
	if pm.GitHub.Owner != "alice" {
		t.Errorf("GitHub.Owner = %q, want %q", pm.GitHub.Owner, "alice")
	}
	if pm.InstalledAt.IsZero() {
		t.Error("InstalledAt should not be zero")
	}
}

func TestComponentList_AllComponents(t *testing.T) {
	cl := ComponentList{
		Skills:   []string{"s1", "s2"},
		Agents:   []string{"a1"},
		Commands: []string{"c1"},
		Rules:    []string{"r1", "r2"},
		Hooks:    []string{"h1"},
	}

	all := cl.AllComponents()
	if len(all) != 7 {
		t.Errorf("AllComponents() returned %d items, want 7", len(all))
	}

	// Verify ordering: skills, agents, commands, rules, hooks
	expected := []ComponentRef{
		{Type: "skills", Name: "s1"},
		{Type: "skills", Name: "s2"},
		{Type: "agents", Name: "a1"},
		{Type: "commands", Name: "c1"},
		{Type: "rules", Name: "r1"},
		{Type: "rules", Name: "r2"},
		{Type: "hooks", Name: "h1"},
	}

	for i, want := range expected {
		if i >= len(all) {
			t.Fatalf("missing component at index %d", i)
		}
		if all[i].Type != want.Type || all[i].Name != want.Name {
			t.Errorf("component[%d] = %v, want %v", i, all[i], want)
		}
	}
}

func TestComponentList_Count(t *testing.T) {
	tests := []struct {
		name string
		cl   ComponentList
		want int
	}{
		{"empty", ComponentList{}, 0},
		{"skills only", ComponentList{Skills: []string{"s1"}}, 1},
		{"mixed", ComponentList{Skills: []string{"s1"}, Agents: []string{"a1"}, Hooks: []string{"h1"}}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cl.Count(); got != tt.want {
				t.Errorf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPluginManifest_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	github := GitHubSource{Owner: "bob", Repo: "kit"}
	components := ComponentList{Skills: []string{"tool-use"}}
	pm := NewPluginManifest("save-test", "test", "2.0.0", github, components)

	if err := pm.Save(tmpDir); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify directory and file were created
	pluginDir := filepath.Join(tmpDir, "save-test")
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.yaml")); err != nil {
		t.Fatalf("plugin.yaml not created: %v", err)
	}

	loaded, err := LoadPluginManifest(tmpDir, "save-test")
	if err != nil {
		t.Fatalf("LoadPluginManifest() error: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "save-test")
	}
	if loaded.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", loaded.Version, "2.0.0")
	}
	if len(loaded.Components.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(loaded.Components.Skills))
	}
}

func TestPluginManifest_Delete(t *testing.T) {
	tmpDir := t.TempDir()

	github := GitHubSource{Owner: "owner", Repo: "repo"}
	pm := NewPluginManifest("del-test", "", "1.0.0", github, ComponentList{})
	pm.Save(tmpDir)

	if err := pm.Delete(tmpDir); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "del-test")); !os.IsNotExist(err) {
		t.Error("expected plugin directory to be removed")
	}
}

func TestLoadPluginManifest_NotFound(t *testing.T) {
	_, err := LoadPluginManifest(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestListPlugins(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some plugins
	github := GitHubSource{Owner: "o", Repo: "r"}
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pm := NewPluginManifest(name, "", "1.0.0", github, ComponentList{})
		pm.Save(tmpDir)
	}

	plugins, err := ListPlugins(tmpDir)
	if err != nil {
		t.Fatalf("ListPlugins() error: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestListPlugins_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	plugins, err := ListPlugins(tmpDir)
	if err != nil {
		t.Fatalf("ListPlugins() error: %v", err)
	}
	if plugins != nil {
		t.Errorf("expected nil, got %v", plugins)
	}
}

func TestListPlugins_NonexistentDir(t *testing.T) {
	plugins, err := ListPlugins("/nonexistent/plugins")
	if err != nil {
		t.Fatalf("ListPlugins() error: %v", err)
	}
	if plugins != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", plugins)
	}
}

func TestListPlugins_SkipsInvalid(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid plugin
	github := GitHubSource{Owner: "o", Repo: "r"}
	pm := NewPluginManifest("valid", "", "1.0.0", github, ComponentList{})
	pm.Save(tmpDir)

	// Create an invalid entry (directory without plugin.yaml)
	os.MkdirAll(filepath.Join(tmpDir, "invalid"), 0755)

	// Create a non-directory entry
	os.WriteFile(filepath.Join(tmpDir, "not-a-dir"), []byte("file"), 0644)

	plugins, err := ListPlugins(tmpDir)
	if err != nil {
		t.Fatalf("ListPlugins() error: %v", err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 valid plugin, got %d", len(plugins))
	}
}
