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

func TestMergeSettingFragments(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test fragment
	fragmentContent := `name: test-fragment
description: A test fragment
key: testKey
value: testValue
`
	if err := os.WriteFile(filepath.Join(fragmentsDir, "test.yaml"), []byte(fragmentContent), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := mergeSettingFragments(hubDir, []string{"test"})
	if err != nil {
		t.Fatalf("mergeSettingFragments() error = %v", err)
	}

	if val, ok := settings["testKey"]; !ok || val != "testValue" {
		t.Errorf("expected settings[testKey] = 'testValue', got %v", settings["testKey"])
	}
}

func TestMergeSettingFragments_NotFound(t *testing.T) {
	hubDir := t.TempDir()

	_, err := mergeSettingFragments(hubDir, []string{"nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent fragment")
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

func TestMergeSettingFragments_MultipleFragments(t *testing.T) {
	hubDir := t.TempDir()
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	if err := os.MkdirAll(fragmentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple fragments
	fragments := map[string]string{
		"model": `name: model-config
key: model
value: claude-sonnet-4-20250514
`,
		"temp": `name: temp-config
key: temperature
value: 0.7
`,
		"perms": `name: perms-config
key: permissions
value:
  allow_edit: true
  allow_bash: false
`,
	}

	for name, content := range fragments {
		if err := os.WriteFile(filepath.Join(fragmentsDir, name+".yaml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	settings, err := mergeSettingFragments(hubDir, []string{"model", "temp", "perms"})
	if err != nil {
		t.Fatalf("mergeSettingFragments() error = %v", err)
	}

	if len(settings) != 3 {
		t.Errorf("expected 3 settings, got %d", len(settings))
	}

	if settings["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model = 'claude-sonnet-4-20250514', got %v", settings["model"])
	}

	if settings["temperature"] != 0.7 {
		t.Errorf("expected temperature = 0.7, got %v", settings["temperature"])
	}

	perms, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected permissions to be a map, got %T", settings["permissions"])
	}
	if perms["allow_edit"] != true {
		t.Errorf("expected permissions.allow_edit = true, got %v", perms["allow_edit"])
	}
}
