package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

func TestHarnessCLI_ListCheckSyncRemoveAndReset(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if err := os.Symlink(paths.ProfileDir("work"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	// Add a hub skill and CLAUDE.md.
	skillDir := filepath.Join(paths.HubItemDir(config.HubSkills), "s1")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: s1\ndescription: Skill 1\n---\n"), 0644)
	if err := profile.NewManager(paths).LinkHubItem("work", config.HubSkills, "s1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(paths.ProfileDir("work"), "CLAUDE.md"), []byte("# Work\n"), 0644)

	targetRoot := filepath.Join(filepath.Dir(paths.GlobalClaudeDir), ".custom-tool")
	os.MkdirAll(paths.HarnessesDir(), 0755)
	os.WriteFile(filepath.Join(paths.HarnessesDir(), "custom.toml"), []byte(
		"root = \""+targetRoot+"\"\n[context]\nlink = \"AGENTS.md\"\n[skills]\nmode = \"link\"\ndir = \"skills\"\n",
	), 0644)

	out, err := captureStdout(t, func() error { return runCcp(t, "harness", "list") })
	if err != nil || !strings.Contains(out, "custom") || !strings.Contains(out, "toml") {
		t.Fatalf("harness list = %q (%v)", out, err)
	}

	out, err = captureStdout(t, func() error { return runCcp(t, "harness", "check", "custom") })
	if err != nil || !strings.Contains(out, "custom: OK") {
		t.Fatalf("harness check = %q (%v)", out, err)
	}

	out, err = captureStdout(t, func() error { return runCcp(t, "harness", "sync", "custom") })
	if err != nil || !strings.Contains(out, "Mirrored profile 'work'") {
		t.Fatalf("harness sync = %q (%v)", out, err)
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md link missing: %v", err)
	}

	// Reset removes custom harness links pointing into ~/.ccp.
	resetForce = true
	t.Cleanup(func() { resetForce = false })
	if _, err := captureStdout(t, func() error { return runCcp(t, "reset", "--force") }); err != nil {
		t.Fatalf("reset --force: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md link survived reset: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, "skills", "s1")); !os.IsNotExist(err) {
		t.Errorf("skills/s1 link survived reset: %v", err)
	}
}
