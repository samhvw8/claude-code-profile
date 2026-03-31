package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

func TestResolvePluginRootPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	hookDir := filepath.Join(home, ".ccp", "profiles", "test", "hooks", "myhook")

	tests := []struct {
		name    string
		command string
		hookDir string
		want    string
	}{
		{
			name:    "with CLAUDE_PLUGIN_ROOT",
			command: "${CLAUDE_PLUGIN_ROOT}/scripts/run.sh",
			hookDir: hookDir,
			want:    "$HOME/.ccp/profiles/test/hooks/myhook/scripts/run.sh",
		},
		{
			name:    "without CLAUDE_PLUGIN_ROOT",
			command: "/usr/local/bin/script.sh",
			hookDir: hookDir,
			want:    "/usr/local/bin/script.sh",
		},
		{
			name:    "empty command",
			command: "",
			hookDir: hookDir,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePluginRootPath(tt.command, tt.hookDir)
			if got != tt.want {
				t.Errorf("resolvePluginRootPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildLegacyCommand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	profileHooksDir := filepath.Join(home, ".ccp", "profiles", "test", "hooks")

	tests := []struct {
		name            string
		hookManifest    *mockHookManifest
		profileHooksDir string
		hookName        string
		want            string
	}{
		{
			name: "inline command",
			hookManifest: &mockHookManifest{
				Inline: "echo hello",
			},
			profileHooksDir: profileHooksDir,
			hookName:        "myhook",
			want:            "echo hello",
		},
		{
			name: "absolute path",
			hookManifest: &mockHookManifest{
				Command: "/usr/local/bin/script.sh",
			},
			profileHooksDir: profileHooksDir,
			hookName:        "myhook",
			want:            "/usr/local/bin/script.sh",
		},
		{
			name: "relative path",
			hookManifest: &mockHookManifest{
				Command: "run.sh",
			},
			profileHooksDir: profileHooksDir,
			hookName:        "myhook",
			want:            "$HOME/.ccp/profiles/test/hooks/myhook/run.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a real hub.HookManifest for testing
			manifest := createHookManifest(tt.hookManifest)
			got := buildLegacyCommand(manifest, tt.profileHooksDir, tt.hookName)
			if got != tt.want {
				t.Errorf("buildLegacyCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mockHookManifest for test setup
type mockHookManifest struct {
	Inline      string
	Command     string
	Interpreter string
}

// createHookManifest converts mock to real hub.HookManifest
func createHookManifest(mock *mockHookManifest) *hub.HookManifest {
	return &hub.HookManifest{
		Inline:      mock.Inline,
		Command:     mock.Command,
		Interpreter: mock.Interpreter,
	}
}

func TestGenerateSettingsHooks_Empty(t *testing.T) {
	paths := &config.Paths{
		HubDir: t.TempDir(),
	}
	profileDir := t.TempDir()
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks: []string{},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	if len(hooks) != 0 {
		t.Errorf("expected empty hooks, got %d", len(hooks))
	}
}

func TestGenerateSettingsHooks_WithHooksJSON(t *testing.T) {
	// Setup temp directories
	profileDir := t.TempDir()
	hubDir := t.TempDir()
	hookName := "test-hook"

	// Create hooks directory in profile
	hookDir := filepath.Join(profileDir, "hooks", hookName)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create hooks.json
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Matcher: "startup",
					Hooks: []config.HookCommand{
						{
							Type:    "command",
							Command: "${CLAUDE_PLUGIN_ROOT}/scripts/start.sh",
							Timeout: 30,
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(hooksJSON)
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks: []string{hookName},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("expected 1 hook type, got %d", len(hooks))
	}

	entries, ok := hooks[config.HookSessionStart]
	if !ok {
		t.Fatal("expected SessionStart hooks")
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Matcher != "startup" {
		t.Errorf("expected matcher 'startup', got %q", entries[0].Matcher)
	}

	if entries[0].Hooks[0].Timeout != 30 {
		t.Errorf("expected timeout 30, got %d", entries[0].Hooks[0].Timeout)
	}
}

func TestGenerateSettingsHooks_MultipleHooksPerType(t *testing.T) {
	// Setup temp directories
	profileDir := t.TempDir()
	hubDir := t.TempDir()

	// Create two hook directories
	for _, hookName := range []string{"hook1", "hook2"} {
		hookDir := filepath.Join(profileDir, "hooks", hookName)
		if err := os.MkdirAll(hookDir, 0755); err != nil {
			t.Fatal(err)
		}

		hooksJSON := config.HooksJSON{
			Hooks: map[config.HookType][]config.HookEntry{
				config.HookSessionStart: {
					{
						Matcher: hookName + "-matcher",
						Hooks: []config.HookCommand{
							{
								Type:    "command",
								Command: "${CLAUDE_PLUGIN_ROOT}/scripts/" + hookName + ".sh",
								Timeout: 60,
							},
						},
					},
				},
			},
		}
		data, _ := json.Marshal(hooksJSON)
		if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	paths := &config.Paths{HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks: []string{"hook1", "hook2"},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	entries := hooks[config.HookSessionStart]
	if len(entries) != 2 {
		t.Errorf("expected 2 SessionStart entries, got %d", len(entries))
	}
}

func TestGenerateSettingsHooks_DefaultTimeout(t *testing.T) {
	profileDir := t.TempDir()
	hubDir := t.TempDir()
	hookName := "no-timeout-hook"

	hookDir := filepath.Join(profileDir, "hooks", hookName)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create hook without timeout specified
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookUserPromptSubmit: {
				{
					Hooks: []config.HookCommand{
						{
							Type:    "command",
							Command: "/path/to/script.sh",
							// No timeout specified
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(hooksJSON)
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks: []string{hookName},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	entries := hooks[config.HookUserPromptSubmit]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Should use default timeout
	if entries[0].Hooks[0].Timeout != config.DefaultHookTimeout() {
		t.Errorf("expected default timeout %d, got %d", config.DefaultHookTimeout(), entries[0].Hooks[0].Timeout)
	}
}

func TestGenerateSettingsHooks_SkipsMissingHook(t *testing.T) {
	profileDir := t.TempDir()
	hubDir := t.TempDir()

	paths := &config.Paths{HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks: []string{"nonexistent-hook"},
		},
	}

	// Should not error, just skip missing hooks
	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	if len(hooks) != 0 {
		t.Errorf("expected 0 hooks (missing skipped), got %d", len(hooks))
	}
}

func TestResolvePluginRootPath_MultipleOccurrences(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	hookDir := filepath.Join(home, ".ccp", "profiles", "test", "hooks", "myhook")

	// Test command with multiple CLAUDE_PLUGIN_ROOT occurrences
	command := "${CLAUDE_PLUGIN_ROOT}/bin/cmd --config ${CLAUDE_PLUGIN_ROOT}/config.json"
	got := resolvePluginRootPath(command, hookDir)

	expected := "$HOME/.ccp/profiles/test/hooks/myhook/bin/cmd --config $HOME/.ccp/profiles/test/hooks/myhook/config.json"
	if got != expected {
		t.Errorf("resolvePluginRootPath() = %q, want %q", got, expected)
	}
}

func TestRegenerateSettings_WritesValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	os.MkdirAll(hubDir, 0755)
	os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755)

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{},
	}

	if err := RegenerateSettings(paths, profileDir, manifest); err != nil {
		t.Fatalf("RegenerateSettings() error: %v", err)
	}

	// Verify settings.json was written
	settingsPath := filepath.Join(profileDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
}

func TestRegenerateSettings_WithTemplateAndHooks(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profile")

	// Create template
	tmplDir := filepath.Join(hubDir, "settings-templates", "regen-tmpl")
	os.MkdirAll(tmplDir, 0755)
	tmplSettings := map[string]interface{}{
		"model":       "opus",
		"temperature": 0.5,
	}
	tmplData, _ := json.Marshal(tmplSettings)
	os.WriteFile(filepath.Join(tmplDir, "settings.json"), tmplData, 0644)

	// Create a hook
	hookDir := filepath.Join(profileDir, "hooks", "regen-hook")
	os.MkdirAll(hookDir, 0755)
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Matcher: "startup",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "${CLAUDE_PLUGIN_ROOT}/scripts/start.sh", Timeout: 30},
					},
				},
			},
		},
	}
	hookData, _ := json.Marshal(hooksJSON)
	os.WriteFile(filepath.Join(hookDir, "hooks.json"), hookData, 0644)

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		SettingsTemplate: "regen-tmpl",
		Hub:              HubLinks{Hooks: []string{"regen-hook"}},
	}

	if err := RegenerateSettings(paths, profileDir, manifest); err != nil {
		t.Fatalf("RegenerateSettings() error: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if parsed["model"] != "opus" {
		t.Errorf("model = %v, want 'opus'", parsed["model"])
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Error("expected 'hooks' key in settings.json")
	}
}

func TestWriteJSONFile_NoHTMLEscaping(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.json")

	data := map[string]interface{}{
		"url": "https://example.com?foo=bar&baz=qux",
		"tag": "<script>alert('xss')</script>",
	}

	if err := writeJSONFile(outPath, data); err != nil {
		t.Fatalf("writeJSONFile() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	// Verify no HTML escaping happened
	s := string(content)
	if !contains(s, "&") {
		t.Error("expected literal '&' in output, got HTML-escaped version")
	}
	if !contains(s, "<script>") {
		t.Error("expected literal '<script>' in output, got HTML-escaped version")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestProcessLegacyHook(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	profileHooksDir := filepath.Join(home, ".ccp", "profiles", "test", "hooks")

	tests := []struct {
		name        string
		manifest    *hub.HookManifest
		hookName    string
		wantType    config.HookType
		wantMatcher string
	}{
		{
			name: "basic hook with command",
			manifest: &hub.HookManifest{
				Name:    "test-hook",
				Type:    config.HookSessionStart,
				Timeout: 30,
				Command: "start.sh",
				Matcher: "startup",
			},
			hookName:    "test-hook",
			wantType:    config.HookSessionStart,
			wantMatcher: "startup",
		},
		{
			name: "hook with interpreter",
			manifest: &hub.HookManifest{
				Name:        "node-hook",
				Type:        config.HookPreToolUse,
				Command:     "check.js",
				Interpreter: "node",
				Matcher:     "Bash",
			},
			hookName:    "node-hook",
			wantType:    config.HookPreToolUse,
			wantMatcher: "Bash",
		},
		{
			name: "inline hook",
			manifest: &hub.HookManifest{
				Name:   "inline-hook",
				Type:   config.HookStop,
				Inline: "echo goodbye",
			},
			hookName:    "inline-hook",
			wantType:    config.HookStop,
			wantMatcher: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks := make(map[config.HookType][]config.SettingsHookEntry)
			processLegacyHook(tt.manifest, profileHooksDir, tt.hookName, hooks)

			entries, ok := hooks[tt.wantType]
			if !ok {
				t.Fatalf("expected hook type %s in result", tt.wantType)
			}
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}
			if entries[0].Matcher != tt.wantMatcher {
				t.Errorf("Matcher = %q, want %q", entries[0].Matcher, tt.wantMatcher)
			}
			if len(entries[0].Hooks) != 1 {
				t.Fatalf("expected 1 hook command, got %d", len(entries[0].Hooks))
			}
		})
	}
}

func TestWriteJSONFile_EmptyMap(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "empty.json")

	if err := writeJSONFile(outPath, map[string]interface{}{}); err != nil {
		t.Fatalf("writeJSONFile() error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty map, got %v", parsed)
	}
}

