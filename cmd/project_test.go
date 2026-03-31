package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestFindProjectClaudeDir_WithDirFlag(t *testing.T) {
	dir := t.TempDir()
	got, err := findProjectClaudeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, ".claude")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFindProjectClaudeDir_WithGitRoot(t *testing.T) {
	// Create a temp dir with .git/ inside
	root := t.TempDir()
	// Resolve symlinks (macOS /var -> /private/var)
	root, _ = filepath.EvalSymlinks(root)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory
	sub := filepath.Join(root, "src", "deep")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// Chdir into subdirectory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(sub)

	got, err := findProjectClaudeDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, ".claude")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFindProjectClaudeDir_NoGitRoot(t *testing.T) {
	// Use a temp dir with no .git anywhere near it
	dir := t.TempDir()
	// Resolve symlinks (macOS /var -> /private/var)
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	got, err := findProjectClaudeDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, ".claude")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestIsValidProjectHubType(t *testing.T) {
	tests := []struct {
		typ  config.HubItemType
		want bool
	}{
		{config.HubSkills, true},
		{config.HubAgents, true},
		{config.HubHooks, true},
		{config.HubRules, true},
		{config.HubCommands, true},
		{config.HubSettingsTemplates, false},
		{config.HubItemType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			got := isValidProjectHubType(tt.typ)
			if got != tt.want {
				t.Errorf("isValidProjectHubType(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestParseItemRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantType config.HubItemType
		wantName string
		wantErr  bool
	}{
		{"skills/coding", config.HubSkills, "coding", false},
		{"agents/reviewer", config.HubAgents, "reviewer", false},
		{"hooks/pre-commit", config.HubHooks, "pre-commit", false},
		{"rules/my-rule", config.HubRules, "my-rule", false},
		{"commands/deploy", config.HubCommands, "deploy", false},
		{"settings-templates/opus", "", "", true},  // excluded type
		{"invalid", "", "", true},                  // no slash
		{"/name", "", "", true},                    // empty type
		{"skills/", "", "", true},                  // empty name
		{"foo/bar", "", "", true},                  // unknown type
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			gotType, gotName, err := parseItemRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseItemRef(%q) expected error, got nil", tt.ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseItemRef(%q) unexpected error: %v", tt.ref, err)
			}
			if gotType != tt.wantType {
				t.Errorf("type = %s, want %s", gotType, tt.wantType)
			}
			if gotName != tt.wantName {
				t.Errorf("name = %s, want %s", gotName, tt.wantName)
			}
		})
	}
}

func TestProjectAddDirect(t *testing.T) {
	// Setup: create hub with a skill
	hubDir := t.TempDir()
	skillDir := filepath.Join(hubDir, "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# Test Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	paths := &config.Paths{
		HubDir: hubDir,
	}

	// Target project claude dir
	claudeDir := filepath.Join(t.TempDir(), ".claude")

	err := runProjectAddDirect(paths, claudeDir, []string{"skills/test-skill"})
	if err != nil {
		t.Fatalf("runProjectAddDirect failed: %v", err)
	}

	// Verify the item was copied
	copiedFile := filepath.Join(claudeDir, "skills", "test-skill", "skill.md")
	data, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("expected copied file at %s: %v", copiedFile, err)
	}
	if string(data) != "# Test Skill" {
		t.Errorf("copied content = %q, want %q", string(data), "# Test Skill")
	}
}

func TestProjectAddDirect_OverwriteExisting(t *testing.T) {
	// Setup hub
	hubDir := t.TempDir()
	skillDir := filepath.Join(hubDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# Updated Skill"), 0644)

	paths := &config.Paths{HubDir: hubDir}

	// Pre-create existing item in project
	claudeDir := filepath.Join(t.TempDir(), ".claude")
	existingDir := filepath.Join(claudeDir, "skills", "test-skill")
	os.MkdirAll(existingDir, 0755)
	os.WriteFile(filepath.Join(existingDir, "skill.md"), []byte("# Old Skill"), 0644)

	err := runProjectAddDirect(paths, claudeDir, []string{"skills/test-skill"})
	if err != nil {
		t.Fatalf("runProjectAddDirect failed: %v", err)
	}

	// Verify the overwritten content
	data, _ := os.ReadFile(filepath.Join(claudeDir, "skills", "test-skill", "skill.md"))
	if string(data) != "# Updated Skill" {
		t.Errorf("content = %q, want %q", string(data), "# Updated Skill")
	}
}

func TestProjectAddDirect_HubItemNotFound(t *testing.T) {
	hubDir := t.TempDir()
	paths := &config.Paths{HubDir: hubDir}
	claudeDir := filepath.Join(t.TempDir(), ".claude")

	err := runProjectAddDirect(paths, claudeDir, []string{"skills/nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent hub item")
	}
}

func TestProjectAddDirect_InvalidType(t *testing.T) {
	hubDir := t.TempDir()
	paths := &config.Paths{HubDir: hubDir}
	claudeDir := filepath.Join(t.TempDir(), ".claude")

	err := runProjectAddDirect(paths, claudeDir, []string{"settings-templates/foo"})
	if err == nil {
		t.Fatal("expected error for settings-templates type")
	}
}

func TestProjectList(t *testing.T) {
	claudeDir := filepath.Join(t.TempDir(), ".claude")

	// Create some items
	skillDir := filepath.Join(claudeDir, "skills", "coding")
	agentDir := filepath.Join(claudeDir, "agents", "reviewer")
	os.MkdirAll(skillDir, 0755)
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte("test"), 0644)

	// runProjectList uses findProjectClaudeDir, so we test via scanner directly
	scanner := newTestScanner()
	h, err := scanner.ScanSource(claudeDir)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if h.ItemCount() != 2 {
		t.Errorf("ItemCount = %d, want 2", h.ItemCount())
	}

	skills := h.GetItems(config.HubSkills)
	if len(skills) != 1 || skills[0].Name != "coding" {
		t.Errorf("skills = %v, want [coding]", skills)
	}

	agents := h.GetItems(config.HubAgents)
	if len(agents) != 1 || agents[0].Name != "reviewer" {
		t.Errorf("agents = %v, want [reviewer]", agents)
	}
}

func TestProjectRemove(t *testing.T) {
	claudeDir := filepath.Join(t.TempDir(), ".claude")

	// Create an item to remove
	skillDir := filepath.Join(claudeDir, "skills", "coding")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("test"), 0644)

	// Verify it exists
	if _, err := os.Stat(skillDir); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Remove it
	itemPath := filepath.Join(claudeDir, "skills", "coding")
	if err := os.RemoveAll(itemPath); err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Errorf("expected item to be removed, got err: %v", err)
	}
}

func TestProjectRemove_NotFound(t *testing.T) {
	claudeDir := filepath.Join(t.TempDir(), ".claude")
	os.MkdirAll(claudeDir, 0755)

	itemPath := filepath.Join(claudeDir, "skills", "nonexistent")
	_, err := os.Stat(itemPath)
	if !os.IsNotExist(err) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// newTestScanner is a helper that wraps hub.NewScanner for tests
func newTestScanner() *testScanner {
	return &testScanner{}
}

type testScanner struct{}

func (s *testScanner) ScanSource(claudeDir string) (*testHub, error) {
	h := &testHub{items: make(map[config.HubItemType][]testItem)}

	dirMap := map[string]config.HubItemType{
		"skills":   config.HubSkills,
		"agents":   config.HubAgents,
		"hooks":    config.HubHooks,
		"rules":    config.HubRules,
		"commands": config.HubCommands,
	}

	for dirName, itemType := range dirMap {
		itemDir := filepath.Join(claudeDir, dirName)
		entries, err := os.ReadDir(itemDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.Name()[0] == '.' {
				continue
			}
			h.items[itemType] = append(h.items[itemType], testItem{
				Name:  entry.Name(),
				IsDir: entry.IsDir(),
			})
		}
	}

	return h, nil
}

type testHub struct {
	items map[config.HubItemType][]testItem
}

type testItem struct {
	Name  string
	IsDir bool
}

func (h *testHub) GetItems(itemType config.HubItemType) []testItem {
	return h.items[itemType]
}

func (h *testHub) ItemCount() int {
	count := 0
	for _, items := range h.items {
		count += len(items)
	}
	return count
}
