package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestGenerateSettings_WithTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	// Create template
	tmplDir := filepath.Join(hubDir, "settings-templates", "test-tmpl")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmplSettings := map[string]interface{}{
		"model":       "claude-sonnet-4-20250514",
		"temperature": 0.7,
	}
	data, _ := json.Marshal(tmplSettings)
	if err := os.WriteFile(filepath.Join(tmplDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create hooks dir (empty)
	if err := os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{
		CcpDir: tmpDir,
		HubDir: hubDir,
	}
	manifest := &Manifest{
		SettingsTemplate: "test-tmpl",
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}

	if settings["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model = 'claude-sonnet-4-20250514', got %v", settings["model"])
	}
	if settings["temperature"] != 0.7 {
		t.Errorf("expected temperature = 0.7, got %v", settings["temperature"])
	}
}

func TestGenerateSettings_NoTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	if err := os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hubDir, 0755); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{
		CcpDir: tmpDir,
		HubDir: hubDir,
	}
	manifest := &Manifest{}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}

	if _, ok := settings["hooks"]; ok {
		t.Error("expected no hooks key when no hooks configured")
	}

	if len(settings) != 0 {
		t.Errorf("expected 0 settings keys, got %d", len(settings))
	}
}

func TestGenerateSettings_TemplateMissing(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	if err := os.MkdirAll(hubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{
		CcpDir: tmpDir,
		HubDir: hubDir,
	}
	manifest := &Manifest{
		SettingsTemplate: "nonexistent",
	}

	_, err := GenerateSettings(manifest, paths, profileDir)
	if err == nil {
		t.Error("expected error for missing template")
	}
}

func TestGenerateSettings_NoTemplateNoHooks_EmptySettings(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	if err := os.MkdirAll(hubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{}, // no hooks
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty settings map, got %d keys: %v", len(settings), settings)
	}
}

func TestGenerateSettings_HooksOnly(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	if err := os.MkdirAll(hubDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a hook in the profile's hooks dir with hooks.json
	hookName := "my-hook"
	hookDir := filepath.Join(profileDir, "hooks", hookName)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "${CLAUDE_PLUGIN_ROOT}/scripts/check.sh", Timeout: 15},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(hooksJSON)
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		// No template
		Hub: HubLinks{Hooks: []string{hookName}},
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}

	hooksVal, ok := settings["hooks"]
	if !ok {
		t.Fatal("expected 'hooks' key in settings")
	}
	hooksMap, ok := hooksVal.(map[config.HookType][]config.SettingsHookEntry)
	if !ok {
		t.Fatalf("hooks value has unexpected type %T", hooksVal)
	}
	entries, ok := hooksMap[config.HookPreToolUse]
	if !ok || len(entries) != 1 {
		t.Fatalf("expected 1 PreToolUse entry, got %d", len(entries))
	}
	if entries[0].Matcher != "Bash" {
		t.Errorf("expected matcher 'Bash', got %q", entries[0].Matcher)
	}

	// Should have no template-derived keys
	if _, exists := settings["model"]; exists {
		t.Error("expected no 'model' key without template")
	}
}

func TestGenerateSettings_TemplateAndHooks_MergedCorrectly(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	// Create template
	tmplDir := filepath.Join(hubDir, "settings-templates", "my-tmpl")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmplSettings := map[string]interface{}{
		"model":       "opus",
		"temperature": 0.9,
		"maxTokens":   4096.0,
	}
	data, _ := json.Marshal(tmplSettings)
	if err := os.WriteFile(filepath.Join(tmplDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a hook
	hookName := "session-hook"
	hookDir := filepath.Join(profileDir, "hooks", hookName)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Matcher: "startup",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "${CLAUDE_PLUGIN_ROOT}/scripts/init.sh", Timeout: 45},
					},
				},
			},
		},
	}
	hookData, _ := json.Marshal(hooksJSON)
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), hookData, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		SettingsTemplate: "my-tmpl",
		Hub:              HubLinks{Hooks: []string{hookName}},
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}

	// Template keys should be present
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want 'opus'", settings["model"])
	}
	if settings["temperature"] != 0.9 {
		t.Errorf("temperature = %v, want 0.9", settings["temperature"])
	}
	if settings["maxTokens"] != 4096.0 {
		t.Errorf("maxTokens = %v, want 4096", settings["maxTokens"])
	}

	// Hooks should also be present
	hooksVal, ok := settings["hooks"]
	if !ok {
		t.Fatal("expected 'hooks' key in settings")
	}
	hooksMap := hooksVal.(map[config.HookType][]config.SettingsHookEntry)
	entries := hooksMap[config.HookSessionStart]
	if len(entries) != 1 {
		t.Fatalf("expected 1 SessionStart entry, got %d", len(entries))
	}
	if entries[0].Matcher != "startup" {
		t.Errorf("expected matcher 'startup', got %q", entries[0].Matcher)
	}
}

func TestGenerateSettings_HookDirNotFound_GracefulSkip(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	if err := os.MkdirAll(hubDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Do NOT create the hooks dir at all

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{Hooks: []string{"nonexistent-hook"}},
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() should not error for missing hook dir, got: %v", err)
	}

	// No hooks should be in the result (skipped gracefully)
	if _, ok := settings["hooks"]; ok {
		t.Error("expected no hooks key when hook directory is missing")
	}
}

func TestGenerateSettings_TemplateWithHooksKey_OverriddenByGeneratedHooks(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	// Create template that includes a "hooks" key (should be overridden)
	tmplDir := filepath.Join(hubDir, "settings-templates", "with-hooks")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmplSettings := map[string]interface{}{
		"model": "sonnet",
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{},
		},
	}
	data, _ := json.Marshal(tmplSettings)
	if err := os.WriteFile(filepath.Join(tmplDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a real hook
	hookName := "real-hook"
	hookDir := filepath.Join(profileDir, "hooks", hookName)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookStop: {
				{
					Hooks: []config.HookCommand{
						{Type: "command", Command: "/bin/cleanup.sh", Timeout: 10},
					},
				},
			},
		},
	}
	hookData, _ := json.Marshal(hooksJSON)
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), hookData, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		SettingsTemplate: "with-hooks",
		Hub:              HubLinks{Hooks: []string{hookName}},
	}

	settings, err := GenerateSettings(manifest, paths, profileDir)
	if err != nil {
		t.Fatalf("GenerateSettings() error = %v", err)
	}

	// The hooks key should be the generated hooks, not the template's
	hooksVal := settings["hooks"]
	hooksMap, ok := hooksVal.(map[config.HookType][]config.SettingsHookEntry)
	if !ok {
		t.Fatalf("hooks should be generated type, got %T", hooksVal)
	}
	if _, hasStop := hooksMap[config.HookStop]; !hasStop {
		t.Error("expected Stop hook from generated hooks")
	}
}
