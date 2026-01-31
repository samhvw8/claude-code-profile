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
