package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateManager_SaveAndLoad(t *testing.T) {
	hubDir := t.TempDir()

	mgr := NewTemplateManager(hubDir)

	tmpl := &Template{
		Name: "test",
		Settings: map[string]interface{}{
			"model":       "claude-sonnet-4-20250514",
			"temperature": 0.7,
		},
	}

	if err := mgr.Save(tmpl); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := mgr.Load("test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Name != "test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test")
	}
	if loaded.Settings["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("model = %v, want 'claude-sonnet-4-20250514'", loaded.Settings["model"])
	}
}

func TestTemplateManager_List(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	// Empty list
	templates, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(templates))
	}

	// Save two templates
	mgr.Save(&Template{Name: "a", Settings: map[string]interface{}{"k": "v"}})
	mgr.Save(&Template{Name: "b", Settings: map[string]interface{}{"k": "v"}})

	templates, err = mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(templates) != 2 {
		t.Errorf("expected 2 templates, got %d", len(templates))
	}
}

func TestTemplateManager_Delete(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	mgr.Save(&Template{Name: "del", Settings: map[string]interface{}{"k": "v"}})

	if !mgr.Exists("del") {
		t.Fatal("expected template to exist")
	}

	if err := mgr.Delete("del"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if mgr.Exists("del") {
		t.Fatal("expected template to not exist after delete")
	}
}

func TestTemplateManager_Exists(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	if mgr.Exists("nonexistent") {
		t.Error("expected nonexistent template to not exist")
	}
}

func TestTemplateManager_LoadNotFound(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	_, err := mgr.Load("missing")
	if err == nil {
		t.Error("expected error loading missing template")
	}
}

func TestExtractFromSettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	content := `{
  "model": "opus",
  "temperature": 0.5,
  "hooks": {
    "SessionStart": [{"hooks": [{"command": "/bin/true", "type": "command"}]}]
  }
}`
	os.WriteFile(settingsPath, []byte(content), 0644)

	settings, err := ExtractFromSettings(settingsPath)
	if err != nil {
		t.Fatalf("ExtractFromSettings() error = %v", err)
	}

	if settings["model"] != "opus" {
		t.Errorf("model = %v, want 'opus'", settings["model"])
	}

	// Hooks should be removed
	if _, ok := settings["hooks"]; ok {
		t.Error("hooks should be excluded from template")
	}

	if len(settings) != 2 {
		t.Errorf("expected 2 keys, got %d", len(settings))
	}
}
