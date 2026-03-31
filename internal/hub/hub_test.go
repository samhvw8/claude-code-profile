package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestHub(t *testing.T) {
	h := New("/test/hub")

	if h.Path != "/test/hub" {
		t.Errorf("Path = %q, want %q", h.Path, "/test/hub")
	}

	if h.ItemCount() != 0 {
		t.Errorf("ItemCount() = %d, want 0", h.ItemCount())
	}
}

func TestHubItems(t *testing.T) {
	h := New("/test/hub")

	h.Items[config.HubSkills] = []Item{
		{Name: "skill1", Type: config.HubSkills, Path: "/test/hub/skills/skill1", IsDir: true},
		{Name: "skill2", Type: config.HubSkills, Path: "/test/hub/skills/skill2", IsDir: true},
	}

	if h.ItemCount() != 2 {
		t.Errorf("ItemCount() = %d, want 2", h.ItemCount())
	}

	if !h.HasItem(config.HubSkills, "skill1") {
		t.Error("HasItem(skills, skill1) = false, want true")
	}

	if h.HasItem(config.HubSkills, "nonexistent") {
		t.Error("HasItem(skills, nonexistent) = true, want false")
	}

	item := h.GetItem(config.HubSkills, "skill1")
	if item == nil {
		t.Fatal("GetItem() returned nil")
	}
	if item.Name != "skill1" {
		t.Errorf("item.Name = %q, want %q", item.Name, "skill1")
	}
}

func TestScanner(t *testing.T) {
	// Create test hub structure
	testDir := t.TempDir()
	hubDir := filepath.Join(testDir, "hub")

	// Create skills directory with items
	skillsDir := filepath.Join(hubDir, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "test-skill"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create hooks directory with a file
	hooksDir := filepath.Join(hubDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "pre-commit.sh"), []byte("#!/bin/bash"), 0755); err != nil {
		t.Fatal(err)
	}

	// Scan
	scanner := NewScanner()
	h, err := scanner.Scan(hubDir)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if h.ItemCount() != 2 {
		t.Errorf("ItemCount() = %d, want 2", h.ItemCount())
	}

	// Check skill
	skill := h.GetItem(config.HubSkills, "test-skill")
	if skill == nil {
		t.Error("skill not found")
	} else if !skill.IsDir {
		t.Error("skill.IsDir = false, want true")
	}

	// Check hook
	hook := h.GetItem(config.HubHooks, "pre-commit.sh")
	if hook == nil {
		t.Error("hook not found")
	} else if hook.IsDir {
		t.Error("hook.IsDir = true, want false")
	}
}

func TestScannerSkipsHiddenFiles(t *testing.T) {
	testDir := t.TempDir()
	skillsDir := filepath.Join(testDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create visible and hidden items
	if err := os.MkdirAll(filepath.Join(skillsDir, "visible"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner()
	h, err := scanner.Scan(testDir)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(h.GetItems(config.HubSkills)) != 1 {
		t.Errorf("expected 1 skill (hidden should be skipped), got %d", len(h.GetItems(config.HubSkills)))
	}
}

// === AllItems tests ===

func TestHub_AllItems(t *testing.T) {
	h := New("/test/hub")

	h.Items[config.HubSkills] = []Item{
		{Name: "skill1", Type: config.HubSkills},
		{Name: "skill2", Type: config.HubSkills},
	}
	h.Items[config.HubHooks] = []Item{
		{Name: "hook1", Type: config.HubHooks},
	}
	h.Items[config.HubRules] = []Item{
		{Name: "rule1", Type: config.HubRules},
		{Name: "rule2", Type: config.HubRules},
	}

	all := h.AllItems()
	if len(all) != 5 {
		t.Errorf("AllItems() returned %d items, want 5", len(all))
	}
}

func TestHub_AllItems_Empty(t *testing.T) {
	h := New("/test/hub")

	all := h.AllItems()
	if len(all) != 0 {
		t.Errorf("AllItems() returned %d items, want 0", len(all))
	}
}

func TestHub_ItemCountByType(t *testing.T) {
	h := New("/test/hub")

	h.Items[config.HubSkills] = []Item{
		{Name: "s1"}, {Name: "s2"}, {Name: "s3"},
	}
	h.Items[config.HubAgents] = []Item{
		{Name: "a1"},
	}
	h.Items[config.HubHooks] = []Item{}

	counts := h.ItemCountByType()
	if counts[config.HubSkills] != 3 {
		t.Errorf("skills count = %d, want 3", counts[config.HubSkills])
	}
	if counts[config.HubAgents] != 1 {
		t.Errorf("agents count = %d, want 1", counts[config.HubAgents])
	}
	if counts[config.HubHooks] != 0 {
		t.Errorf("hooks count = %d, want 0", counts[config.HubHooks])
	}
}

// === GetHooksJSON tests ===

func TestGetHooksJSON(t *testing.T) {
	hookDir := t.TempDir()

	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Matcher: "startup",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "/bin/start.sh", Timeout: 30},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(hooksJSON)
	os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644)

	loaded, err := GetHooksJSON(hookDir)
	if err != nil {
		t.Fatalf("GetHooksJSON() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetHooksJSON() returned nil")
	}
	entries := loaded.Hooks[config.HookSessionStart]
	if len(entries) != 1 {
		t.Errorf("expected 1 SessionStart entry, got %d", len(entries))
	}
	if entries[0].Matcher != "startup" {
		t.Errorf("Matcher = %q, want %q", entries[0].Matcher, "startup")
	}
}

func TestGetHooksJSON_NotFound(t *testing.T) {
	hookDir := t.TempDir()

	_, err := GetHooksJSON(hookDir)
	if err == nil {
		t.Error("expected error for missing hooks.json")
	}
}

func TestGetHooksJSON_InvalidJSON(t *testing.T) {
	hookDir := t.TempDir()
	os.WriteFile(filepath.Join(hookDir, "hooks.json"), []byte("not valid json{{{"), 0644)

	_, err := GetHooksJSON(hookDir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// === SaveHooksJSON tests ===

func TestSaveHooksJSON(t *testing.T) {
	hookDir := t.TempDir()

	hooksJSON := &config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "/bin/check.sh", Timeout: 15},
					},
				},
			},
		},
	}

	if err := SaveHooksJSON(hookDir, hooksJSON); err != nil {
		t.Fatalf("SaveHooksJSON() error: %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(filepath.Join(hookDir, "hooks.json"))
	if err != nil {
		t.Fatalf("hooks.json not created: %v", err)
	}

	// Parse back
	loaded, err := GetHooksJSON(hookDir)
	if err != nil {
		t.Fatalf("GetHooksJSON() error: %v", err)
	}
	entries := loaded.Hooks[config.HookPreToolUse]
	if len(entries) != 1 {
		t.Errorf("expected 1 PreToolUse entry, got %d", len(entries))
	}
	_ = data
}

// === GetHookManifest tests ===

func TestGetHookManifest_HooksJSON(t *testing.T) {
	hubDir := t.TempDir()
	hookDir := filepath.Join(hubDir, "hooks", "my-hook")
	os.MkdirAll(hookDir, 0755)

	hooksJSON := config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookSessionStart: {
				{
					Matcher: "startup",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "/bin/start.sh", Timeout: 30},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(hooksJSON)
	os.WriteFile(filepath.Join(hookDir, "hooks.json"), data, 0644)

	manifest, err := GetHookManifest(hubDir, "my-hook")
	if err != nil {
		t.Fatalf("GetHookManifest() error: %v", err)
	}
	if manifest == nil {
		t.Fatal("GetHookManifest() returned nil")
	}
	if manifest.Name != "my-hook" {
		t.Errorf("Name = %q, want %q", manifest.Name, "my-hook")
	}
	if manifest.Type != config.HookSessionStart {
		t.Errorf("Type = %q, want %q", manifest.Type, config.HookSessionStart)
	}
}

func TestGetHookManifest_LegacyYAML(t *testing.T) {
	hubDir := t.TempDir()
	hookDir := filepath.Join(hubDir, "hooks", "legacy-hook")
	os.MkdirAll(hookDir, 0755)

	yamlContent := `name: legacy-hook
type: PreToolUse
timeout: 15
command: check.sh
matcher: Bash
`
	os.WriteFile(filepath.Join(hookDir, "hook.yaml"), []byte(yamlContent), 0644)

	manifest, err := GetHookManifest(hubDir, "legacy-hook")
	if err != nil {
		t.Fatalf("GetHookManifest() error: %v", err)
	}
	if manifest.Name != "legacy-hook" {
		t.Errorf("Name = %q, want %q", manifest.Name, "legacy-hook")
	}
	if manifest.Type != config.HookPreToolUse {
		t.Errorf("Type = %q, want %q", manifest.Type, config.HookPreToolUse)
	}
	if manifest.Timeout != 15 {
		t.Errorf("Timeout = %d, want 15", manifest.Timeout)
	}
	if manifest.Matcher != "Bash" {
		t.Errorf("Matcher = %q, want %q", manifest.Matcher, "Bash")
	}
}

func TestGetHookManifest_NotFound(t *testing.T) {
	hubDir := t.TempDir()

	_, err := GetHookManifest(hubDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent hook")
	}
}

// === hooksJSONToManifest tests ===

func TestHooksJSONToManifest_EmptyHooks(t *testing.T) {
	hooksJSON := &config.HooksJSON{
		Hooks: make(map[config.HookType][]config.HookEntry),
	}

	manifest, err := hooksJSONToManifest(hooksJSON, "empty-hook")
	if err != nil {
		t.Fatalf("hooksJSONToManifest() error: %v", err)
	}
	if manifest.Name != "empty-hook" {
		t.Errorf("Name = %q, want %q", manifest.Name, "empty-hook")
	}
}

func TestHooksJSONToManifest_WithEntries(t *testing.T) {
	hooksJSON := &config.HooksJSON{
		Hooks: map[config.HookType][]config.HookEntry{
			config.HookStop: {
				{
					Matcher: "all",
					Hooks: []config.HookCommand{
						{Type: "command", Command: "/bin/cleanup.sh", Timeout: 10},
					},
				},
			},
		},
	}

	manifest, err := hooksJSONToManifest(hooksJSON, "test-hook")
	if err != nil {
		t.Fatalf("hooksJSONToManifest() error: %v", err)
	}
	if manifest.Type != config.HookStop {
		t.Errorf("Type = %q, want %q", manifest.Type, config.HookStop)
	}
	if manifest.Command != "/bin/cleanup.sh" {
		t.Errorf("Command = %q, want %q", manifest.Command, "/bin/cleanup.sh")
	}
	if manifest.Timeout != 10 {
		t.Errorf("Timeout = %d, want 10", manifest.Timeout)
	}
	if manifest.Matcher != "all" {
		t.Errorf("Matcher = %q, want %q", manifest.Matcher, "all")
	}
}

// === GetHookCommand tests ===

func TestHookManifest_GetHookCommand(t *testing.T) {
	hookDir := "/some/hook/dir"

	tests := []struct {
		name     string
		manifest *HookManifest
		want     string
	}{
		{
			name:     "inline command",
			manifest: &HookManifest{Inline: "echo hello"},
			want:     "echo hello",
		},
		{
			name:     "absolute path",
			manifest: &HookManifest{Command: "/usr/local/bin/hook.sh"},
			want:     "/usr/local/bin/hook.sh",
		},
		{
			name:     "relative path",
			manifest: &HookManifest{Command: "run.sh"},
			want:     filepath.Join(hookDir, "run.sh"),
		},
		{
			name:     "empty command returns hookDir",
			manifest: &HookManifest{Command: ""},
			want:     hookDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.GetHookCommand(hookDir)
			if got != tt.want {
				t.Errorf("GetHookCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

// === ScanSource tests ===

func TestScanner_ScanSource(t *testing.T) {
	testDir := t.TempDir()

	// Create source directory structure
	os.MkdirAll(filepath.Join(testDir, "skills", "code-review"), 0755)
	os.MkdirAll(filepath.Join(testDir, "agents", "debugger"), 0755)
	os.MkdirAll(filepath.Join(testDir, "hooks", "pre-commit"), 0755)
	os.MkdirAll(filepath.Join(testDir, "rules", "style"), 0755)
	os.MkdirAll(filepath.Join(testDir, "commands", "deploy"), 0755)

	scanner := NewScanner()
	h, err := scanner.ScanSource(testDir)
	if err != nil {
		t.Fatalf("ScanSource() error: %v", err)
	}

	if len(h.GetItems(config.HubSkills)) != 1 {
		t.Errorf("skills count = %d, want 1", len(h.GetItems(config.HubSkills)))
	}
	if len(h.GetItems(config.HubAgents)) != 1 {
		t.Errorf("agents count = %d, want 1", len(h.GetItems(config.HubAgents)))
	}
	if len(h.GetItems(config.HubHooks)) != 1 {
		t.Errorf("hooks count = %d, want 1", len(h.GetItems(config.HubHooks)))
	}
	if len(h.GetItems(config.HubRules)) != 1 {
		t.Errorf("rules count = %d, want 1", len(h.GetItems(config.HubRules)))
	}
	if len(h.GetItems(config.HubCommands)) != 1 {
		t.Errorf("commands count = %d, want 1", len(h.GetItems(config.HubCommands)))
	}

	if h.ItemCount() != 5 {
		t.Errorf("ItemCount() = %d, want 5", h.ItemCount())
	}
}

func TestScanner_ScanSource_EmptyDir(t *testing.T) {
	testDir := t.TempDir()

	scanner := NewScanner()
	h, err := scanner.ScanSource(testDir)
	if err != nil {
		t.Fatalf("ScanSource() error: %v", err)
	}
	if h.ItemCount() != 0 {
		t.Errorf("ItemCount() = %d, want 0", h.ItemCount())
	}
}
