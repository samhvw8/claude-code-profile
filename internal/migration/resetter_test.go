package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestNewResetter(t *testing.T) {
	paths, _ := setupTestPaths(t)
	r := NewResetter(paths)

	if r == nil {
		t.Fatal("NewResetter returned nil")
	}
	if r.paths != paths {
		t.Error("paths not set correctly")
	}
}

func TestResetter_Execute_HappyPath(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Set up a complete ccp structure to reset
	// 1. Create ccp directory structure
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.SharedDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Create default profile
	defaultProfile := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(defaultProfile, 0755); err != nil {
		t.Fatal(err)
	}

	// Create profile content
	if err := os.WriteFile(filepath.Join(defaultProfile, "CLAUDE.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultProfile, "profile.yaml"), []byte("name: default"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Create ~/.claude symlink pointing to default profile
	if err := os.Symlink(defaultProfile, paths.ClaudeDir); err != nil {
		t.Fatal(err)
	}

	// Execute reset
	r := NewResetter(paths)
	if err := r.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify ~/.claude is now a regular directory
	info, err := os.Lstat(paths.ClaudeDir)
	if err != nil {
		t.Fatalf("ClaudeDir should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("~/.claude should be a regular directory, not a symlink")
	}

	// Verify CLAUDE.md was restored
	if _, err := os.Stat(filepath.Join(paths.ClaudeDir, "CLAUDE.md")); os.IsNotExist(err) {
		t.Error("CLAUDE.md should be restored")
	}

	// Verify profile.yaml was NOT restored (ccp-specific)
	if _, err := os.Stat(filepath.Join(paths.ClaudeDir, "profile.yaml")); !os.IsNotExist(err) {
		t.Error("profile.yaml should not be restored")
	}

	// Verify ~/.ccp was removed
	if _, err := os.Stat(paths.CcpDir); !os.IsNotExist(err) {
		t.Error("ccp dir should be removed")
	}
}

func TestResetter_Execute_NotSymlink(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create ~/.claude as a regular directory (not a symlink)
	if err := os.MkdirAll(paths.ClaudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := NewResetter(paths)
	err := r.Execute()

	// Should fail because ~/.claude is not a symlink
	if err == nil {
		t.Error("expected error when ~/.claude is not a symlink")
	}
}

func TestResetter_Execute_BrokenSymlink(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create a broken symlink
	if err := os.Symlink("/nonexistent/path", paths.ClaudeDir); err != nil {
		t.Fatal(err)
	}

	r := NewResetter(paths)
	err := r.Execute()

	// Should fail because symlink target doesn't exist
	if err == nil {
		t.Error("expected error for broken symlink")
	}
}

func TestResetter_Execute_WithSymlinks(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create hub with a skill
	skillDir := filepath.Join(paths.HubDir, "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create default profile
	defaultProfile := filepath.Join(paths.ProfilesDir, "default")
	profileSkillsDir := filepath.Join(defaultProfile, "skills")
	if err := os.MkdirAll(profileSkillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink in profile pointing to hub
	if err := os.Symlink(skillDir, filepath.Join(profileSkillsDir, "test-skill")); err != nil {
		t.Fatal(err)
	}

	// Create manifest
	if err := os.WriteFile(filepath.Join(defaultProfile, "profile.yaml"), []byte("name: default"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create ~/.claude symlink
	if err := os.Symlink(defaultProfile, paths.ClaudeDir); err != nil {
		t.Fatal(err)
	}

	// Execute reset
	r := NewResetter(paths)
	if err := r.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify skill was resolved (copied, not symlink)
	restoredSkill := filepath.Join(paths.ClaudeDir, "skills", "test-skill")
	info, err := os.Lstat(restoredSkill)
	if err != nil {
		t.Fatalf("skill should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("skill should be a regular directory, not a symlink")
	}

	// Verify SKILL.md content was copied
	if _, err := os.Stat(filepath.Join(restoredSkill, "SKILL.md")); os.IsNotExist(err) {
		t.Error("SKILL.md should be restored")
	}
}

func TestResetter_CopyProfileContents(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create profile structure
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Add files
	if err := os.WriteFile(filepath.Join(profileDir, "CLAUDE.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte("name: test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Add subdir
	if err := os.MkdirAll(filepath.Join(profileDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "subdir", "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResetter(paths)
	destDir := filepath.Join(paths.ClaudeDir, "-restore")

	if err := r.copyProfileContents(profileDir, destDir); err != nil {
		t.Fatalf("copyProfileContents failed: %v", err)
	}

	// Verify CLAUDE.md was copied
	if _, err := os.Stat(filepath.Join(destDir, "CLAUDE.md")); os.IsNotExist(err) {
		t.Error("CLAUDE.md should be copied")
	}

	// Verify profile.yaml was skipped
	if _, err := os.Stat(filepath.Join(destDir, "profile.yaml")); !os.IsNotExist(err) {
		t.Error("profile.yaml should not be copied")
	}

	// Verify subdir was copied
	if _, err := os.Stat(filepath.Join(destDir, "subdir", "file.txt")); os.IsNotExist(err) {
		t.Error("subdir/file.txt should be copied")
	}
}

func TestResetter_CopyDirResolvingSymlinks(t *testing.T) {
	paths, tmpDir := setupTestPaths(t)

	// Create a target directory that will be symlinked
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "data.txt"), []byte("target data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create source directory with a symlink
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "regular.txt"), []byte("regular"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, filepath.Join(srcDir, "linked")); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	r := NewResetter(paths)

	if err := r.copyDirResolvingSymlinks(srcDir, destDir, false); err != nil {
		t.Fatalf("copyDirResolvingSymlinks failed: %v", err)
	}

	// Verify regular file was copied
	if _, err := os.Stat(filepath.Join(destDir, "regular.txt")); os.IsNotExist(err) {
		t.Error("regular.txt should exist")
	}

	// Verify symlink was resolved and content copied
	linkedDest := filepath.Join(destDir, "linked")
	info, err := os.Lstat(linkedDest)
	if err != nil {
		t.Fatalf("linked should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("linked should be a regular directory, not a symlink")
	}
	if _, err := os.Stat(filepath.Join(linkedDest, "data.txt")); os.IsNotExist(err) {
		t.Error("data.txt should exist in resolved symlink")
	}
}

func TestResetter_RewriteSettingsHooks(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create restored ~/.claude directory
	if err := os.MkdirAll(paths.ClaudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	oldProfileDir := filepath.Join(home, ".ccp", "profiles", "default")

	// Create settings.json with old hook paths
	settingsContent := `{
		"hooks": {
			"SessionStart": [{
				"hooks": [{"command": "$HOME/.ccp/profiles/default/hooks/test-hook/script.sh", "timeout": 5000, "type": "command"}]
			}]
		}
	}`
	if err := os.WriteFile(filepath.Join(paths.ClaudeDir, "settings.json"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResetter(paths)
	if err := r.rewriteSettingsHooks(oldProfileDir); err != nil {
		t.Fatalf("rewriteSettingsHooks failed: %v", err)
	}

	// Verify hook paths were rewritten
	content, err := os.ReadFile(filepath.Join(paths.ClaudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Should now reference ~/.claude/hooks/
	if !contains(string(content), "$HOME/.claude/hooks/") {
		t.Errorf("hook path should be rewritten to ~/.claude/hooks/, got: %s", content)
	}
}

func TestResetter_RewriteSettingsHooks_NoSettings(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create ~/.claude without settings.json
	if err := os.MkdirAll(paths.ClaudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := NewResetter(paths)
	// Should not error when settings.json doesn't exist
	if err := r.rewriteSettingsHooks("/old/profile"); err != nil {
		t.Errorf("should not error when settings.json doesn't exist: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper to check config.Paths has ProfileDir method
func init() {
	// Ensure config.Paths has the methods we need
	var _ = (&config.Paths{}).ProfileDir
}
