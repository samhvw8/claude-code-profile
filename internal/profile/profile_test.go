package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samhoang/ccp/internal/config"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest("test-profile", "Test description")

	if m.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", m.Name, "test-profile")
	}

	if m.Description != "Test description" {
		t.Errorf("Description = %q, want %q", m.Description, "Test description")
	}

	if time.Since(m.Created) > time.Second {
		t.Error("Created timestamp is too old")
	}

	// Check default data config
	if m.Data.Tasks != config.ShareModeShared {
		t.Error("Data.Tasks should default to shared")
	}

	if m.Data.History != config.ShareModeIsolated {
		t.Error("Data.History should default to isolated")
	}
}

func TestManifestSaveLoad(t *testing.T) {
	testDir := t.TempDir()
	manifestPath := filepath.Join(testDir, "profile.yaml")

	// Create and save
	m := NewManifest("save-test", "Save test description")
	m.Hub.Skills = []string{"skill1", "skill2"}
	m.Hub.Hooks = []string{"hook1"}

	if err := m.Save(manifestPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load
	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "save-test")
	}

	if len(loaded.Hub.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(loaded.Hub.Skills))
	}

	if len(loaded.Hub.Hooks) != 1 {
		t.Errorf("len(Hooks) = %d, want 1", len(loaded.Hub.Hooks))
	}
}

func TestManifestAddRemoveHubItem(t *testing.T) {
	m := NewManifest("test", "")

	// Add item
	m.AddHubItem(config.HubSkills, "new-skill")
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(m.Hub.Skills))
	}

	// Add duplicate (should not add)
	m.AddHubItem(config.HubSkills, "new-skill")
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) after duplicate = %d, want 1", len(m.Hub.Skills))
	}

	// Add another
	m.AddHubItem(config.HubSkills, "another-skill")
	if len(m.Hub.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(m.Hub.Skills))
	}

	// Remove
	removed := m.RemoveHubItem(config.HubSkills, "new-skill")
	if !removed {
		t.Error("RemoveHubItem() = false, want true")
	}
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) after remove = %d, want 1", len(m.Hub.Skills))
	}

	// Remove non-existent
	removed = m.RemoveHubItem(config.HubSkills, "nonexistent")
	if removed {
		t.Error("RemoveHubItem(nonexistent) = true, want false")
	}
}

func TestManifestGetSetHubItems(t *testing.T) {
	m := NewManifest("test", "")

	items := []string{"a", "b", "c"}
	m.SetHubItems(config.HubRules, items)

	got := m.GetHubItems(config.HubRules)
	if len(got) != 3 {
		t.Errorf("len(Rules) = %d, want 3", len(got))
	}
}

func TestManifestAllHubItemsFlat(t *testing.T) {
	m := NewManifest("test", "")
	m.Hub.Skills = []string{"s1", "s2"}
	m.Hub.Hooks = []string{"h1"}
	m.Hub.Rules = []string{"r1", "r2", "r3"}

	flat := m.AllHubItemsFlat()
	if len(flat) != 6 {
		t.Errorf("len(AllHubItemsFlat) = %d, want 6", len(flat))
	}
}

func TestProfileManager(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		ClaudeDir:   testDir,
		HubDir:      filepath.Join(testDir, "hub"),
		ProfilesDir: filepath.Join(testDir, "profiles"),
		SharedDir:   filepath.Join(testDir, "profiles", "shared"),
	}

	// Create hub structure
	for _, itemType := range config.AllHubItemTypes() {
		if err := os.MkdirAll(paths.HubItemDir(itemType), 0755); err != nil {
			t.Fatal(err)
		}
	}

	mgr := NewManager(paths)

	// Test create
	manifest := NewManifest("test-profile", "Test")
	p, err := mgr.Create("test-profile", manifest)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if p.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", p.Name, "test-profile")
	}

	// Test exists
	if !mgr.Exists("test-profile") {
		t.Error("Exists() = false, want true")
	}

	// Test get
	got, err := mgr.Get("test-profile")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", got.Name, "test-profile")
	}

	// Test list
	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List) = %d, want 1", len(list))
	}

	// Test delete
	if err := mgr.Delete("test-profile"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if mgr.Exists("test-profile") {
		t.Error("profile still exists after delete")
	}
}
