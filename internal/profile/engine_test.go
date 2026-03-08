package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestEngineCreateAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewEngineManager(paths)

	engine := NewEngine("test-engine", "Test engine")
	engine.Hub.SettingFragments = []string{"model-opus", "permissions-full"}
	engine.Hub.Hooks = []string{"session-manager"}
	engine.Data.History = config.ShareModeIsolated

	if err := mgr.Create("test-engine", engine); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	tomlPath := filepath.Join(paths.EngineDir("test-engine"), "engine.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		t.Fatalf("engine.toml not created: %v", err)
	}

	// Get it back
	got, err := mgr.Get("test-engine")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name != "test-engine" {
		t.Errorf("Name = %q, want %q", got.Name, "test-engine")
	}
	if got.Description != "Test engine" {
		t.Errorf("Description = %q, want %q", got.Description, "Test engine")
	}
	if len(got.Hub.SettingFragments) != 2 {
		t.Errorf("SettingFragments = %v, want 2 items", got.Hub.SettingFragments)
	}
	if len(got.Hub.Hooks) != 1 {
		t.Errorf("Hooks = %v, want 1 item", got.Hub.Hooks)
	}
}

func TestEngineList(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewEngineManager(paths)

	// Empty list
	engines, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(engines) != 0 {
		t.Errorf("expected 0 engines, got %d", len(engines))
	}

	// Create two engines
	mgr.Create("engine-a", NewEngine("engine-a", ""))
	mgr.Create("engine-b", NewEngine("engine-b", ""))

	engines, err = mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(engines) != 2 {
		t.Errorf("expected 2 engines, got %d", len(engines))
	}
}

func TestEngineDelete(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewEngineManager(paths)
	mgr.Create("to-delete", NewEngine("to-delete", ""))

	if !mgr.Exists("to-delete") {
		t.Fatal("engine should exist")
	}

	if err := mgr.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if mgr.Exists("to-delete") {
		t.Fatal("engine should not exist after delete")
	}
}

func TestEngineDuplicateCreate(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewEngineManager(paths)
	mgr.Create("dup", NewEngine("dup", ""))

	err := mgr.Create("dup", NewEngine("dup", ""))
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
}
