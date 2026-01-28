package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	// Test with CCP_CLAUDE_DIR set
	testDir := t.TempDir()
	os.Setenv("CCP_CLAUDE_DIR", testDir)
	defer os.Unsetenv("CCP_CLAUDE_DIR")

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error: %v", err)
	}

	if paths.ClaudeDir != testDir {
		t.Errorf("ClaudeDir = %q, want %q", paths.ClaudeDir, testDir)
	}

	if paths.HubDir != filepath.Join(testDir, "hub") {
		t.Errorf("HubDir = %q, want %q", paths.HubDir, filepath.Join(testDir, "hub"))
	}

	if paths.ProfilesDir != filepath.Join(testDir, "profiles") {
		t.Errorf("ProfilesDir = %q, want %q", paths.ProfilesDir, filepath.Join(testDir, "profiles"))
	}
}

func TestPathsProfileDir(t *testing.T) {
	paths := &Paths{
		ClaudeDir:   "/home/user/.claude",
		ProfilesDir: "/home/user/.claude/profiles",
	}

	got := paths.ProfileDir("test")
	want := "/home/user/.claude/profiles/test"
	if got != want {
		t.Errorf("ProfileDir() = %q, want %q", got, want)
	}
}

func TestPathsHubItemPath(t *testing.T) {
	paths := &Paths{
		HubDir: "/home/user/.claude/hub",
	}

	got := paths.HubItemPath(HubSkills, "debugging")
	want := "/home/user/.claude/hub/skills/debugging"
	if got != want {
		t.Errorf("HubItemPath() = %q, want %q", got, want)
	}
}

func TestAllHubItemTypes(t *testing.T) {
	types := AllHubItemTypes()
	if len(types) != 5 {
		t.Errorf("AllHubItemTypes() returned %d types, want 5", len(types))
	}

	expected := []HubItemType{HubSkills, HubHooks, HubRules, HubCommands, HubMdFragments}
	for i, typ := range types {
		if typ != expected[i] {
			t.Errorf("types[%d] = %q, want %q", i, typ, expected[i])
		}
	}
}

func TestDefaultDataConfig(t *testing.T) {
	config := DefaultDataConfig()

	if config[DataTasks] != ShareModeShared {
		t.Errorf("DataTasks should be shared")
	}

	if config[DataHistory] != ShareModeIsolated {
		t.Errorf("DataHistory should be isolated")
	}
}

func TestPathsIsInitialized(t *testing.T) {
	testDir := t.TempDir()

	paths := &Paths{
		ClaudeDir: testDir,
		HubDir:    filepath.Join(testDir, "hub"),
	}

	// Not initialized yet
	if paths.IsInitialized() {
		t.Error("IsInitialized() = true, want false")
	}

	// Create hub directory
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Now initialized
	if !paths.IsInitialized() {
		t.Error("IsInitialized() = false, want true")
	}
}
