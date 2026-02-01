package profile

import (
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

// mockHookProcessor is a test double for HookProcessor
type mockHookProcessorImpl struct {
	hooks map[config.HookType][]config.SettingsHookEntry
	err   error
}

func (m *mockHookProcessorImpl) ProcessAll(manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error) {
	return m.hooks, m.err
}

// mockFragmentProcessor is a test double for FragmentProcessor
type mockFragmentProcessorImpl struct {
	settings map[string]interface{}
	err      error
}

func (m *mockFragmentProcessorImpl) ProcessAll(manifest *Manifest) (map[string]interface{}, error) {
	return m.settings, m.err
}

func TestDefaultSettingsBuilder_Build(t *testing.T) {
	// Setup mock processors
	hookProcessor := &mockHookProcessorImpl{
		hooks: map[config.HookType][]config.SettingsHookEntry{
			config.HookSessionStart: {
				config.NewSettingsHookEntry("startup", "/path/to/script.sh", 60),
			},
		},
	}

	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: map[string]interface{}{
			"model":        "claude-sonnet-4-20250514",
			"temperature": 0.7,
		},
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	settings, err := builder.Build(manifest)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify fragments are included
	if settings["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model = 'claude-sonnet-4-20250514', got %v", settings["model"])
	}
	if settings["temperature"] != 0.7 {
		t.Errorf("expected temperature = 0.7, got %v", settings["temperature"])
	}

	// Verify hooks are included
	hooks, ok := settings["hooks"]
	if !ok {
		t.Fatal("expected hooks to be present")
	}
	hooksMap, ok := hooks.(map[config.HookType][]config.SettingsHookEntry)
	if !ok {
		t.Fatal("hooks should be map[config.HookType][]config.SettingsHookEntry")
	}
	if len(hooksMap[config.HookSessionStart]) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(hooksMap[config.HookSessionStart]))
	}
}

func TestDefaultSettingsBuilder_Build_EmptyHooks(t *testing.T) {
	hookProcessor := &mockHookProcessorImpl{
		hooks: map[config.HookType][]config.SettingsHookEntry{},
	}

	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: map[string]interface{}{
			"key": "value",
		},
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	settings, err := builder.Build(manifest)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Hooks should not be present when empty
	if _, ok := settings["hooks"]; ok {
		t.Error("expected hooks to not be present when empty")
	}

	// Fragment should still be present
	if settings["key"] != "value" {
		t.Errorf("expected key = 'value', got %v", settings["key"])
	}
}

func TestBuilderFromPaths(t *testing.T) {
	paths := &config.Paths{
		HubDir: "/test/hub",
	}
	profileDir := "/test/profile"

	builder := BuilderFromPaths(paths, profileDir)
	if builder == nil {
		t.Error("BuilderFromPaths() returned nil")
	}
}

func TestDefaultSettingsBuilder_Build_ProcessorError(t *testing.T) {
	// Test hook processor error propagation
	hookProcessor := &mockHookProcessorImpl{
		hooks: nil,
		err:   errTestError,
	}
	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: map[string]interface{}{},
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	_, err := builder.Build(manifest)
	if err == nil {
		t.Error("expected error from hook processor to propagate")
	}
}

func TestDefaultSettingsBuilder_Build_FragmentProcessorError(t *testing.T) {
	// Test fragment processor error propagation
	hookProcessor := &mockHookProcessorImpl{
		hooks: map[config.HookType][]config.SettingsHookEntry{},
	}
	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: nil,
		err:      errTestError,
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	_, err := builder.Build(manifest)
	if err == nil {
		t.Error("expected error from fragment processor to propagate")
	}
}

func TestDefaultSettingsBuilder_Build_MultipleHookTypes(t *testing.T) {
	hookProcessor := &mockHookProcessorImpl{
		hooks: map[config.HookType][]config.SettingsHookEntry{
			config.HookSessionStart: {
				config.NewSettingsHookEntry("startup", "/path/start.sh", 60),
			},
			config.HookPreToolUse: {
				config.NewSettingsHookEntry("Bash", "/path/pre-bash.sh", 30),
				config.NewSettingsHookEntry("Edit", "/path/pre-edit.sh", 30),
			},
			config.HookUserPromptSubmit: {
				config.NewSettingsHookEntry("", "/path/prompt.sh", 60),
			},
		},
	}

	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: map[string]interface{}{},
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	settings, err := builder.Build(manifest)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	hooks, ok := settings["hooks"].(map[config.HookType][]config.SettingsHookEntry)
	if !ok {
		t.Fatal("expected hooks map")
	}

	if len(hooks) != 3 {
		t.Errorf("expected 3 hook types, got %d", len(hooks))
	}

	if len(hooks[config.HookPreToolUse]) != 2 {
		t.Errorf("expected 2 PreToolUse hooks, got %d", len(hooks[config.HookPreToolUse]))
	}
}

func TestDefaultSettingsBuilder_Build_OnlyFragments(t *testing.T) {
	hookProcessor := &mockHookProcessorImpl{
		hooks: map[config.HookType][]config.SettingsHookEntry{},
	}

	fragmentProcessor := &mockFragmentProcessorImpl{
		settings: map[string]interface{}{
			"model":                  "claude-sonnet-4-20250514",
			"temperature":            0.5,
			"permissions.allow_edit": true,
		},
	}

	builder := NewSettingsBuilder(hookProcessor, fragmentProcessor)
	manifest := &Manifest{}

	settings, err := builder.Build(manifest)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have 3 fragment keys, no hooks key
	if len(settings) != 3 {
		t.Errorf("expected 3 settings keys, got %d", len(settings))
	}

	if _, hasHooks := settings["hooks"]; hasHooks {
		t.Error("expected no hooks key when hooks are empty")
	}
}

// Error for testing
var errTestError = &testError{msg: "test error"}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
