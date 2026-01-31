package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	// Test with CCP_DIR set
	testDir := t.TempDir()
	os.Setenv("CCP_DIR", testDir)
	defer os.Unsetenv("CCP_DIR")

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error: %v", err)
	}

	if paths.CcpDir != testDir {
		t.Errorf("CcpDir = %q, want %q", paths.CcpDir, testDir)
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
		CcpDir:      "/home/user/.ccp",
		ProfilesDir: "/home/user/.ccp/profiles",
	}

	got := paths.ProfileDir("test")
	want := "/home/user/.ccp/profiles/test"
	if got != want {
		t.Errorf("ProfileDir() = %q, want %q", got, want)
	}
}

func TestPathsHubItemPath(t *testing.T) {
	paths := &Paths{
		HubDir: "/home/user/.ccp/hub",
	}

	got := paths.HubItemPath(HubSkills, "debugging")
	want := "/home/user/.ccp/hub/skills/debugging"
	if got != want {
		t.Errorf("HubItemPath() = %q, want %q", got, want)
	}
}

func TestAllHubItemTypes(t *testing.T) {
	types := AllHubItemTypes()
	if len(types) != 6 {
		t.Errorf("AllHubItemTypes() returned %d types, want 6", len(types))
	}

	expected := []HubItemType{HubSkills, HubAgents, HubHooks, HubRules, HubCommands, HubSettingFragments}
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
		CcpDir: testDir,
		HubDir: filepath.Join(testDir, "hub"),
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

func TestToPortablePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "path under home",
			path: filepath.Join(home, ".ccp", "hooks", "test"),
			want: "$HOME/.ccp/hooks/test",
		},
		{
			name: "path not under home",
			path: "/usr/local/bin/script.sh",
			want: "/usr/local/bin/script.sh",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name: "home path exactly",
			path: home,
			want: "$HOME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPortablePath(tt.path)
			if got != tt.want {
				t.Errorf("ToPortablePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
