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

func TestExtractFromSettings_NoHooks(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	content := `{"model": "sonnet", "temperature": 0.3}`
	os.WriteFile(settingsPath, []byte(content), 0644)

	settings, err := ExtractFromSettings(settingsPath)
	if err != nil {
		t.Fatalf("ExtractFromSettings() error = %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("expected 2 keys, got %d", len(settings))
	}
}

func TestExtractFromSettings_MissingFile(t *testing.T) {
	_, err := ExtractFromSettings("/nonexistent/settings.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExtractFromSettings_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(settingsPath, []byte("not valid json{{{"), 0644)

	_, err := ExtractFromSettings(settingsPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTemplateManager_List_MultipleTemplates(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	// Save three templates
	for _, name := range []string{"alpha", "beta", "gamma"} {
		mgr.Save(&Template{
			Name: name,
			Settings: map[string]interface{}{
				"model": name + "-model",
			},
		})
	}

	templates, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(templates) != 3 {
		t.Errorf("expected 3 templates, got %d", len(templates))
	}

	// Verify each loaded correctly
	names := map[string]bool{}
	for _, tmpl := range templates {
		names[tmpl.Name] = true
		if tmpl.Settings["model"] != tmpl.Name+"-model" {
			t.Errorf("template %s model = %v, want %s-model", tmpl.Name, tmpl.Settings["model"], tmpl.Name)
		}
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !names[expected] {
			t.Errorf("template %q not found in list", expected)
		}
	}
}

func TestTemplateManager_List_SkipsInvalid(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	// Save a valid template
	mgr.Save(&Template{Name: "valid", Settings: map[string]interface{}{"k": "v"}})

	// Create an invalid template (directory without valid settings.json)
	invalidDir := filepath.Join(hubDir, "settings-templates", "invalid")
	os.MkdirAll(invalidDir, 0755)
	os.WriteFile(filepath.Join(invalidDir, "settings.json"), []byte("not json"), 0644)

	// Create a non-directory entry (should be skipped)
	os.WriteFile(filepath.Join(hubDir, "settings-templates", "not-a-dir"), []byte("file"), 0644)

	templates, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	// Only "valid" should be returned; "invalid" has bad JSON, "not-a-dir" is a file
	if len(templates) != 1 {
		t.Errorf("expected 1 valid template, got %d", len(templates))
	}
	if templates[0].Name != "valid" {
		t.Errorf("expected 'valid', got %q", templates[0].Name)
	}
}

func TestTemplateManager_Load_InvalidJSON(t *testing.T) {
	hubDir := t.TempDir()
	tmplDir := filepath.Join(hubDir, "settings-templates", "bad")
	os.MkdirAll(tmplDir, 0755)
	os.WriteFile(filepath.Join(tmplDir, "settings.json"), []byte("{{invalid json"), 0644)

	mgr := NewTemplateManager(hubDir)
	_, err := mgr.Load("bad")
	if err == nil {
		t.Error("expected error loading invalid JSON template")
	}
}

func TestTemplateManager_Save_ComplexSettings(t *testing.T) {
	hubDir := t.TempDir()
	mgr := NewTemplateManager(hubDir)

	tmpl := &Template{
		Name: "complex",
		Settings: map[string]interface{}{
			"model":       "opus",
			"temperature": 0.9,
			"maxTokens":   4096.0,
			"nested": map[string]interface{}{
				"key1": "value1",
				"key2": true,
			},
			"list": []interface{}{"a", "b", "c"},
		},
	}

	if err := mgr.Save(tmpl); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := mgr.Load("complex")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Settings["model"] != "opus" {
		t.Errorf("model = %v, want 'opus'", loaded.Settings["model"])
	}
	nested, ok := loaded.Settings["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested should be a map")
	}
	if nested["key1"] != "value1" {
		t.Errorf("nested.key1 = %v, want 'value1'", nested["key1"])
	}
}
