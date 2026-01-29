package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestClassifyHook(t *testing.T) {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")

	tests := []struct {
		name             string
		hook             ExtractedHook
		expectedLocation HookLocation
	}{
		{
			name: "inside hook",
			hook: ExtractedHook{
				FilePath: claudeDir + "/hooks/test.sh",
				IsInside: true,
			},
			expectedLocation: HookLocationInside,
		},
		{
			name: "outside hook",
			hook: ExtractedHook{
				FilePath: "/usr/local/bin/test.sh",
				IsInside: false,
			},
			expectedLocation: HookLocationOutside,
		},
		{
			name: "inline hook by flag",
			hook: ExtractedHook{
				Command:  "echo hello",
				IsInline: true,
			},
			expectedLocation: HookLocationInline,
		},
		{
			name: "inline hook by empty path",
			hook: ExtractedHook{
				Command:  "echo hello",
				FilePath: "",
			},
			expectedLocation: HookLocationInline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyHook(tt.hook, claudeDir)
			if result.Location != tt.expectedLocation {
				t.Errorf("Location = %v, want %v", result.Location, tt.expectedLocation)
			}
		})
	}
}

func TestClassifyHooks(t *testing.T) {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")

	hooks := []ExtractedHook{
		{FilePath: claudeDir + "/hooks/inside.sh", IsInside: true},
		{FilePath: "/external/path/outside.sh", IsInside: false},
		{Command: "echo inline", IsInline: true},
		{FilePath: claudeDir + "/hooks/another.sh", IsInside: true},
	}

	result := ClassifyHooks(hooks, claudeDir)

	if len(result.Inside) != 2 {
		t.Errorf("expected 2 inside hooks, got %d", len(result.Inside))
	}
	if len(result.Outside) != 1 {
		t.Errorf("expected 1 outside hook, got %d", len(result.Outside))
	}
	if len(result.Inline) != 1 {
		t.Errorf("expected 1 inline hook, got %d", len(result.Inline))
	}
}

func TestHookLocation_String(t *testing.T) {
	tests := []struct {
		location HookLocation
		expected string
	}{
		{HookLocationInside, "inside"},
		{HookLocationOutside, "outside"},
		{HookLocationInline, "inline"},
		{HookLocation(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.location.String() != tt.expected {
			t.Errorf("HookLocation(%d).String() = %q, want %q", tt.location, tt.location.String(), tt.expected)
		}
	}
}

func TestHookMigrationChoice_String(t *testing.T) {
	tests := []struct {
		choice   HookMigrationChoice
		expected string
	}{
		{HookChoiceCopy, "copy"},
		{HookChoiceSkip, "skip"},
		{HookChoiceKeep, "keep"},
		{HookMigrationChoice(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.choice.String() != tt.expected {
			t.Errorf("HookMigrationChoice(%d).String() = %q, want %q", tt.choice, tt.choice.String(), tt.expected)
		}
	}
}

func TestHookMigrationPlan_GetHooksToMigrate(t *testing.T) {
	inside := ClassifiedHook{ExtractedHook: ExtractedHook{FilePath: "/inside.sh"}, Location: HookLocationInside}
	inline := ClassifiedHook{ExtractedHook: ExtractedHook{Command: "echo"}, Location: HookLocationInline}
	outside1 := ClassifiedHook{ExtractedHook: ExtractedHook{FilePath: "/out1.sh"}, Location: HookLocationOutside}
	outside2 := ClassifiedHook{ExtractedHook: ExtractedHook{FilePath: "/out2.sh"}, Location: HookLocationOutside}

	plan := &HookMigrationPlan{
		Inside: []ClassifiedHook{inside},
		Inline: []ClassifiedHook{inline},
		Decisions: []HookMigrationDecision{
			{Hook: outside1, Choice: HookChoiceCopy},
			{Hook: outside2, Choice: HookChoiceSkip},
		},
	}

	toMigrate := plan.GetHooksToMigrate()

	// inside + inline + outside1 (copy) = 3
	if len(toMigrate) != 3 {
		t.Errorf("expected 3 hooks to migrate, got %d", len(toMigrate))
	}
}

func TestHookMigrationPlan_GetHooksToKeep(t *testing.T) {
	outside1 := ClassifiedHook{ExtractedHook: ExtractedHook{FilePath: "/out1.sh"}, Location: HookLocationOutside}
	outside2 := ClassifiedHook{ExtractedHook: ExtractedHook{FilePath: "/out2.sh"}, Location: HookLocationOutside}

	plan := &HookMigrationPlan{
		Decisions: []HookMigrationDecision{
			{Hook: outside1, Choice: HookChoiceKeep},
			{Hook: outside2, Choice: HookChoiceCopy},
		},
	}

	toKeep := plan.GetHooksToKeep()

	if len(toKeep) != 1 {
		t.Errorf("expected 1 hook to keep, got %d", len(toKeep))
	}
}

func TestHookManifest_Fields(t *testing.T) {
	manifest := &HookManifest{
		Name:        "test-hook",
		Type:        config.HookType("SessionStart"),
		Timeout:     5000,
		Command:     "script.sh",
		Interpreter: "bash",
		Matcher:     "Bash",
		Inline:      "",
	}

	if manifest.Name != "test-hook" {
		t.Error("Name mismatch")
	}
	if manifest.Type != "SessionStart" {
		t.Error("Type mismatch")
	}
}

func TestGetRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		baseDir  string
		expected string
	}{
		{"simple relative", "/home/user/.claude/hooks/test.sh", "/home/user/.claude", "hooks/test.sh"},
		{"same dir", "/home/user/.claude/file.txt", "/home/user/.claude", "file.txt"},
		{"nested", "/home/user/.claude/a/b/c.txt", "/home/user/.claude", "a/b/c.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRelativePath(tt.path, tt.baseDir)
			if result != tt.expected {
				t.Errorf("getRelativePath(%q, %q) = %q, want %q", tt.path, tt.baseDir, result, tt.expected)
			}
		})
	}
}

func TestGetParentDirs(t *testing.T) {
	result := getParentDirs("/home/user/scripts/hooks/test.sh")

	// Should get up to 3 parent directories
	if len(result) == 0 {
		t.Error("expected at least one parent dir")
	}
	if len(result) > 3 {
		t.Error("should return at most 3 parent dirs")
	}
}
