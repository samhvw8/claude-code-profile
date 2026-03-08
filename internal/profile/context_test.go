package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestContextCreateAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewContextManager(paths)

	ctx := NewContext("test-ctx", "Test context")
	ctx.Hub.Skills = []string{"coding", "debugging"}
	ctx.Hub.Agents = []string{"reviewer"}
	ctx.Hub.Rules = []string{"strict"}
	ctx.Hub.Commands = []string{"deploy"}
	ctx.Hub.Hooks = []string{"prompt-loader"}

	if err := mgr.Create("test-ctx", ctx); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	tomlPath := filepath.Join(paths.ContextDir("test-ctx"), "context.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		t.Fatalf("context.toml not created: %v", err)
	}

	got, err := mgr.Get("test-ctx")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name != "test-ctx" {
		t.Errorf("Name = %q, want %q", got.Name, "test-ctx")
	}
	if len(got.Hub.Skills) != 2 {
		t.Errorf("Skills = %v, want 2 items", got.Hub.Skills)
	}
	if len(got.Hub.Agents) != 1 {
		t.Errorf("Agents = %v, want 1 item", got.Hub.Agents)
	}
	if len(got.Hub.Hooks) != 1 {
		t.Errorf("Hooks = %v, want 1 item", got.Hub.Hooks)
	}
}

func TestContextList(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewContextManager(paths)

	contexts, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(contexts))
	}

	mgr.Create("ctx-a", NewContext("ctx-a", ""))
	mgr.Create("ctx-b", NewContext("ctx-b", ""))

	contexts, err = mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(contexts))
	}
}

func TestContextDelete(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ContextsDir: filepath.Join(tmpDir, "contexts"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	mgr := NewContextManager(paths)
	mgr.Create("to-delete", NewContext("to-delete", ""))

	if !mgr.Exists("to-delete") {
		t.Fatal("context should exist")
	}

	if err := mgr.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if mgr.Exists("to-delete") {
		t.Fatal("context should not exist after delete")
	}
}
