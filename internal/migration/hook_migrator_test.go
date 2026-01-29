package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

// setupTestPaths creates a test environment and returns config.Paths
func setupTestPaths(t *testing.T) (*config.Paths, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create fake home structure
	claudeDir := filepath.Join(tmpDir, ".claude")
	ccpDir := filepath.Join(tmpDir, ".ccp")

	paths := &config.Paths{
		ClaudeDir:   claudeDir,
		CcpDir:      ccpDir,
		HubDir:      filepath.Join(ccpDir, "hub"),
		ProfilesDir: filepath.Join(ccpDir, "profiles"),
		SharedDir:   filepath.Join(ccpDir, "profiles", "shared"),
	}

	return paths, tmpDir
}

func TestHookMigrator_MigrateHooks_Empty(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create hub dir
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create profile dir
	profileDir := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	plan := &HookMigrationPlan{}

	migrated, err := migrator.MigrateHooks(plan, profileDir)
	if err != nil {
		t.Fatalf("MigrateHooks failed: %v", err)
	}

	if len(migrated) != 0 {
		t.Errorf("expected 0 migrated hooks, got %d", len(migrated))
	}
}

func TestHookMigrator_MigrateInlineHook(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create hub dir
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create profile dir
	profileDir := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	plan := &HookMigrationPlan{
		Inline: []ClassifiedHook{
			{
				ExtractedHook: ExtractedHook{
					HookType: config.HookType("SessionStart"),
					Command:  "echo 'Hello World'",
					IsInline: true,
					Timeout:  5000,
				},
				Location: HookLocationInline,
			},
		},
	}

	migrated, err := migrator.MigrateHooks(plan, profileDir)
	if err != nil {
		t.Fatalf("MigrateHooks failed: %v", err)
	}

	if len(migrated) != 1 {
		t.Fatalf("expected 1 migrated hook, got %d", len(migrated))
	}

	// Verify hook.yaml was created
	hookDir := migrated[0].HubPath
	manifestPath := filepath.Join(hookDir, "hook.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Error("hook.yaml should exist")
	}

	// Verify manifest content
	if migrated[0].Manifest.Inline != "echo 'Hello World'" {
		t.Errorf("Inline = %q, want 'echo 'Hello World''", migrated[0].Manifest.Inline)
	}
}

func TestHookMigrator_MigrateInsideHook(t *testing.T) {
	paths, tmpDir := setupTestPaths(t)

	// Create the source hook file inside ~/.claude
	hooksDir := filepath.Join(paths.ClaudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(hooksDir, "test-hook.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create hub and profile dirs
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	plan := &HookMigrationPlan{
		Inside: []ClassifiedHook{
			{
				ExtractedHook: ExtractedHook{
					HookType:    config.HookType("PreToolUse"),
					Command:     "bash " + scriptPath,
					FilePath:    scriptPath,
					Interpreter: "bash",
					IsInside:    true,
					Timeout:     3000,
				},
				Location:     HookLocationInside,
				RelativePath: "hooks/test-hook.sh",
			},
		},
	}

	migrated, err := migrator.MigrateHooks(plan, profileDir)
	if err != nil {
		t.Fatalf("MigrateHooks failed: %v", err)
	}

	if len(migrated) != 1 {
		t.Fatalf("expected 1 migrated hook, got %d", len(migrated))
	}

	// Verify script was moved to hub
	hubHookScript := filepath.Join(migrated[0].HubPath, "test-hook.sh")
	if _, err := os.Stat(hubHookScript); os.IsNotExist(err) {
		t.Error("script should be moved to hub")
	}

	// Verify original was removed (moved, not copied for inside hooks)
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("original script should be removed after move")
	}

	_ = tmpDir // used for paths setup
}

func TestHookMigrator_MigrateOutsideHook_Copy(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create external script
	externalDir := filepath.Join(filepath.Dir(paths.ClaudeDir), "external-scripts")
	if err := os.MkdirAll(externalDir, 0755); err != nil {
		t.Fatal(err)
	}
	externalScript := filepath.Join(externalDir, "external.sh")
	if err := os.WriteFile(externalScript, []byte("#!/bin/bash\necho external"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create hub and profile dirs
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	plan := &HookMigrationPlan{
		Decisions: []HookMigrationDecision{
			{
				Hook: ClassifiedHook{
					ExtractedHook: ExtractedHook{
						HookType: config.HookType("PostToolUse"),
						FilePath: externalScript,
						Timeout:  2000,
					},
					Location: HookLocationOutside,
				},
				Choice: HookChoiceCopy,
			},
		},
	}

	migrated, err := migrator.MigrateHooks(plan, profileDir)
	if err != nil {
		t.Fatalf("MigrateHooks failed: %v", err)
	}

	if len(migrated) != 1 {
		t.Fatalf("expected 1 migrated hook, got %d", len(migrated))
	}

	// Verify script was copied to hub (not moved)
	hubHookScript := filepath.Join(migrated[0].HubPath, "external.sh")
	if _, err := os.Stat(hubHookScript); os.IsNotExist(err) {
		t.Error("script should be copied to hub")
	}

	// Verify original still exists (copied, not moved)
	if _, err := os.Stat(externalScript); os.IsNotExist(err) {
		t.Error("original external script should still exist")
	}
}

func TestHookMigrator_MigrateOutsideHook_Skip(t *testing.T) {
	paths, _ := setupTestPaths(t)

	// Create hub and profile dirs
	if err := os.MkdirAll(paths.HubDir, 0755); err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(paths.ProfilesDir, "default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	plan := &HookMigrationPlan{
		Decisions: []HookMigrationDecision{
			{
				Hook: ClassifiedHook{
					ExtractedHook: ExtractedHook{
						HookType: config.HookType("SessionStart"),
						FilePath: "/some/external/script.sh",
					},
					Location: HookLocationOutside,
				},
				Choice: HookChoiceSkip,
			},
		},
	}

	migrated, err := migrator.MigrateHooks(plan, profileDir)
	if err != nil {
		t.Fatalf("MigrateHooks failed: %v", err)
	}

	// Skip should result in no migrated hooks
	if len(migrated) != 0 {
		t.Errorf("expected 0 migrated hooks for skip, got %d", len(migrated))
	}
}

func TestHookMigrator_UniqueName(t *testing.T) {
	paths, _ := setupTestPaths(t)
	rollback := NewRollback()
	migrator := NewHookMigrator(paths, rollback)

	usedNames := make(map[string]int)

	name1 := migrator.uniqueName("test-hook", usedNames)
	name2 := migrator.uniqueName("test-hook", usedNames)
	name3 := migrator.uniqueName("test-hook", usedNames)
	name4 := migrator.uniqueName("other-hook", usedNames)

	if name1 != "test-hook" {
		t.Errorf("first name should be 'test-hook', got %q", name1)
	}
	if name2 != "test-hook-2" {
		t.Errorf("second name should be 'test-hook-2', got %q", name2)
	}
	if name3 != "test-hook-3" {
		t.Errorf("third name should be 'test-hook-3', got %q", name3)
	}
	if name4 != "other-hook" {
		t.Errorf("different base should be 'other-hook', got %q", name4)
	}
}

func TestBuildSettingsCommand(t *testing.T) {
	home, _ := os.UserHomeDir()
	profileHooksDir := filepath.Join(home, ".ccp", "profiles", "default", "hooks")

	tests := []struct {
		name     string
		manifest *HookManifest
		wantCmd  string
	}{
		{
			name: "inline command",
			manifest: &HookManifest{
				Name:   "inline-hook",
				Inline: "echo 'hello'",
			},
			wantCmd: "echo 'hello'",
		},
		{
			name: "relative command with interpreter",
			manifest: &HookManifest{
				Name:        "script-hook",
				Command:     "script.sh",
				Interpreter: "bash",
			},
			wantCmd: "bash $HOME/.ccp/profiles/default/hooks/script-hook/script.sh",
		},
		{
			name: "relative command without interpreter",
			manifest: &HookManifest{
				Name:    "script-hook",
				Command: "script.sh",
			},
			wantCmd: "$HOME/.ccp/profiles/default/hooks/script-hook/script.sh",
		},
		{
			name: "absolute path external",
			manifest: &HookManifest{
				Name:    "external-hook",
				Command: "/usr/local/bin/external.sh",
			},
			wantCmd: "/usr/local/bin/external.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSettingsCommand(tt.manifest, profileHooksDir)
			if result != tt.wantCmd {
				t.Errorf("BuildSettingsCommand() = %q, want %q", result, tt.wantCmd)
			}
		})
	}
}

func TestMoveItem(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("move file", func(t *testing.T) {
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := moveItem(src, dst); err != nil {
			t.Fatalf("moveItem failed: %v", err)
		}

		if _, err := os.Stat(dst); os.IsNotExist(err) {
			t.Error("dest should exist")
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("source should be removed")
		}
	})

	t.Run("move directory", func(t *testing.T) {
		srcDir := filepath.Join(tmpDir, "srcdir")
		dstDir := filepath.Join(tmpDir, "dstdir")

		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := moveItem(srcDir, dstDir); err != nil {
			t.Fatalf("moveItem failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
			t.Error("file in dest should exist")
		}
	})
}
