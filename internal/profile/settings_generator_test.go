package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/hub"
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

	var parsed map[string]any
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
	tmplSettings := map[string]any{
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

	var parsed map[string]any
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

	data := map[string]any{
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
	var parsed map[string]any
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

	if err := writeJSONFile(outPath, map[string]any{}); err != nil {
		t.Fatalf("writeJSONFile() error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty map, got %v", parsed)
	}
}

func TestGenerateSettingsHooks_HubFallbackWithoutProfileSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profiles", "default")
	if err := os.MkdirAll(filepath.Join(profileDir, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a dangling symlink for impeccable and leave bundle-hook absent,
	// simulating `ccp use -g` or `SyncHooks` before `ccp profile fix` / `ccp bootstrap`.
	if err := os.Symlink(filepath.Join(tmpDir, "nonexistent-target"), filepath.Join(profileDir, "hooks", "impeccable")); err != nil {
		t.Fatal(err)
	}

	// 1. Multi-event hub hook with ${CLAUDE_PLUGIN_ROOT}
	hubHookDir := filepath.Join(hubDir, "hooks", "impeccable")
	if err := os.MkdirAll(hubHookDir, 0755); err != nil {
		t.Fatal(err)
	}
	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Hooks: []config.HookCommand{
						{Type: "command", Command: "bash ${CLAUDE_PLUGIN_ROOT}/scripts/start.sh", Timeout: 15},
					},
				},
			},
			config.HookPostToolUse: {
				{
					Matcher: "Edit|Write",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "bash ${CLAUDE_PLUGIN_ROOT}/scripts/post.sh", Timeout: 20},
					},
				},
			},
			config.HookStop: {
				{
					Hooks: []config.HookCommand{
						{Type: "command", Command: "bash ${CLAUDE_PLUGIN_ROOT}/scripts/stop.sh", Timeout: 25},
					},
				},
			},
		},
	}
	data, err := json.Marshal(hooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hubHookDir, "hooks.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Bundle hook whose symlink is also missing from profileDir/hooks/
	bundleDir := filepath.Join(hubDir, "bundles", "demo-bundle")
	bundleHookDir := filepath.Join(bundleDir, "hooks", "bundle-hook")
	if err := os.MkdirAll(bundleHookDir, 0755); err != nil {
		t.Fatal(err)
	}
	bundleYAML := "name: demo-bundle\nmembers:\n  hooks:\n    - bundle-hook\n"
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(bundleYAML), 0644); err != nil {
		t.Fatal(err)
	}
	bundleHooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "sh ${CLAUDE_PLUGIN_ROOT}/scripts/check.sh", Timeout: 10},
					},
				},
			},
		},
	}
	bundleData, err := json.Marshal(bundleHooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleHookDir, "hooks.json"), bundleData, 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks:   []string{"impeccable"},
			Bundles: []string{"demo-bundle"},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}

	wantHookRoot := config.ToPortablePath(filepath.Join(profileDir, "hooks", "impeccable"))
	type wantSpec struct {
		cmd     string
		matcher string
		timeout int
	}
	wantByEvent := map[config.HookType]wantSpec{
		config.HookSessionStart: {cmd: "bash " + wantHookRoot + "/scripts/start.sh", matcher: "", timeout: 15},
		config.HookPostToolUse:  {cmd: "bash " + wantHookRoot + "/scripts/post.sh", matcher: "Edit|Write", timeout: 20},
		config.HookStop:         {cmd: "bash " + wantHookRoot + "/scripts/stop.sh", matcher: "", timeout: 25},
	}
	for event, want := range wantByEvent {
		entries := hooks[event]
		if len(entries) != 1 {
			t.Errorf("%s: got %d entries, want 1", event, len(entries))
			continue
		}
		if entries[0].Matcher != want.matcher {
			t.Errorf("%s matcher = %q, want %q", event, entries[0].Matcher, want.matcher)
		}
		if got := entries[0].Hooks[0].Command; got != want.cmd {
			t.Errorf("%s command = %q, want %q", event, got, want.cmd)
		}
		if got := entries[0].Hooks[0].Timeout; got != want.timeout {
			t.Errorf("%s timeout = %d, want %d", event, got, want.timeout)
		}
		if got := entries[0].Hooks[0].Type; got != "command" {
			t.Errorf("%s type = %q, want %q", event, got, "command")
		}
	}

	wantBundleRoot := config.ToPortablePath(filepath.Join(profileDir, "hooks", "bundle-hook"))
	wantBundleCmd := "sh " + wantBundleRoot + "/scripts/check.sh"
	preEntries := hooks[config.HookPreToolUse]
	if len(preEntries) != 1 {
		t.Fatalf("PreToolUse (bundle hook): got %d entries, want 1", len(preEntries))
	}
	if preEntries[0].Matcher != "Bash" {
		t.Errorf("PreToolUse matcher = %q, want %q", preEntries[0].Matcher, "Bash")
	}
	if got := preEntries[0].Hooks[0].Command; got != wantBundleCmd {
		t.Errorf("PreToolUse command = %q, want %q", got, wantBundleCmd)
	}
	if got := preEntries[0].Hooks[0].Timeout; got != 10 {
		t.Errorf("PreToolUse timeout = %d, want 10", got)
	}

	// End-to-end SyncHooks without profile symlinks writes clean settings.json
	if err := SyncHooks(paths, profileDir, manifest); err != nil {
		t.Fatalf("SyncHooks() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if strings.Contains(string(raw), "${CLAUDE_PLUGIN_ROOT}") {
		t.Errorf("settings.json still contains unresolved ${CLAUDE_PLUGIN_ROOT}: %s", raw)
	}
}

func TestGenerateSettingsHooks_DeduplicatesHubAndBundleHooks(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profiles", "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Direct hub hook "shared-hook"
	hubHookDir := filepath.Join(hubDir, "hooks", "shared-hook")
	if err := os.MkdirAll(hubHookDir, 0755); err != nil {
		t.Fatal(err)
	}
	directJSON := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"bash ${CLAUDE_PLUGIN_ROOT}/direct.sh","timeout":15}]}]}}`
	if err := os.WriteFile(filepath.Join(hubHookDir, "hooks.json"), []byte(directJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Bundle with same-named hook "shared-hook"
	bundleDir := filepath.Join(hubDir, "bundles", "b1")
	bundleHookDir := filepath.Join(bundleDir, "hooks", "shared-hook")
	if err := os.MkdirAll(bundleHookDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte("name: b1\nmembers:\n  hooks:\n    - shared-hook\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bundleJSON := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"bash ${CLAUDE_PLUGIN_ROOT}/bundle.sh","timeout":15}]}]}}`
	if err := os.WriteFile(filepath.Join(bundleHookDir, "hooks.json"), []byte(bundleJSON), 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{
		Hub: HubLinks{
			Hooks:   []string{"shared-hook"},
			Bundles: []string{"b1"},
		},
	}

	hooks, err := GenerateSettingsHooks(paths, profileDir, manifest)
	if err != nil {
		t.Fatalf("GenerateSettingsHooks() error = %v", err)
	}
	entries := hooks[config.HookSessionStart]
	if len(entries) != 1 {
		t.Fatalf("expected 1 deduplicated entry (direct hub hook wins), got %d: %+v", len(entries), entries)
	}
	if !strings.HasSuffix(entries[0].Hooks[0].Command, "/direct.sh") {
		t.Errorf("expected direct hub hook to win, got %q", entries[0].Hooks[0].Command)
	}
}

func TestGenerateSettingsHooks_MalformedHooksJSONReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	hubDir := filepath.Join(tmpDir, "hub")
	profileDir := filepath.Join(tmpDir, "profiles", "default")
	hookDir := filepath.Join(hubDir, "hooks", "broken-json")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{CcpDir: tmpDir, HubDir: hubDir}
	manifest := &Manifest{Hub: HubLinks{Hooks: []string{"broken-json"}}}

	if _, err := GenerateSettingsHooks(paths, profileDir, manifest); err == nil {
		t.Fatal("GenerateSettingsHooks() error = nil for malformed hooks.json, want error")
	}
}

func TestManifest_RejectsTraversingBundleName(t *testing.T) {
	dir := t.TempDir()
	content := "version = 2\nname = \"evil\"\n[hub]\nbundles = [\"../../outside\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "profile.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(filepath.Join(dir, "profile.toml"))
	if !errors.Is(err, hub.ErrInvalidItemName) {
		t.Fatalf("LoadManifest() error = %v, want hub.ErrInvalidItemName", err)
	}
}
