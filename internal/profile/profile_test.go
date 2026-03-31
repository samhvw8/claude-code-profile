package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samhoang/ccp/internal/config"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest("test-profile", "Test description")

	if m.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", m.Name, "test-profile")
	}

	if m.Description != "Test description" {
		t.Errorf("Description = %q, want %q", m.Description, "Test description")
	}

	if time.Since(m.Created) > time.Second {
		t.Error("Created timestamp is too old")
	}
}

func TestManifestSaveLoad(t *testing.T) {
	testDir := t.TempDir()
	manifestPath := filepath.Join(testDir, "profile.yaml")

	// Create and save
	m := NewManifest("save-test", "Save test description")
	m.Hub.Skills = []string{"skill1", "skill2"}
	m.Hub.Hooks = []string{"hook1"}

	if err := m.Save(manifestPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load
	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "save-test")
	}

	if len(loaded.Hub.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(loaded.Hub.Skills))
	}

	if len(loaded.Hub.Hooks) != 1 {
		t.Errorf("len(Hooks) = %d, want 1", len(loaded.Hub.Hooks))
	}
}

func TestManifestAddRemoveHubItem(t *testing.T) {
	m := NewManifest("test", "")

	// Add item
	m.AddHubItem(config.HubSkills, "new-skill")
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(m.Hub.Skills))
	}

	// Add duplicate (should not add)
	m.AddHubItem(config.HubSkills, "new-skill")
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) after duplicate = %d, want 1", len(m.Hub.Skills))
	}

	// Add another
	m.AddHubItem(config.HubSkills, "another-skill")
	if len(m.Hub.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(m.Hub.Skills))
	}

	// Remove
	removed := m.RemoveHubItem(config.HubSkills, "new-skill")
	if !removed {
		t.Error("RemoveHubItem() = false, want true")
	}
	if len(m.Hub.Skills) != 1 {
		t.Errorf("len(Skills) after remove = %d, want 1", len(m.Hub.Skills))
	}

	// Remove non-existent
	removed = m.RemoveHubItem(config.HubSkills, "nonexistent")
	if removed {
		t.Error("RemoveHubItem(nonexistent) = true, want false")
	}
}

func TestManifestGetSetHubItems(t *testing.T) {
	m := NewManifest("test", "")

	items := []string{"a", "b", "c"}
	m.SetHubItems(config.HubRules, items)

	got := m.GetHubItems(config.HubRules)
	if len(got) != 3 {
		t.Errorf("len(Rules) = %d, want 3", len(got))
	}
}

func TestManifestAllHubItemsFlat(t *testing.T) {
	m := NewManifest("test", "")
	m.Hub.Skills = []string{"s1", "s2"}
	m.Hub.Hooks = []string{"h1"}
	m.Hub.Rules = []string{"r1", "r2", "r3"}

	flat := m.AllHubItemsFlat()
	if len(flat) != 6 {
		t.Errorf("len(AllHubItemsFlat) = %d, want 6", len(flat))
	}
}

func TestProfileManager_SetActive_GlobalVsProject(t *testing.T) {
	testDir := t.TempDir()

	// Simulate: CLAUDE_CONFIG_DIR points to a project-specific profile
	projectClaudeDir := filepath.Join(testDir, "project", "profiles", "dev")
	globalClaudeDir := filepath.Join(testDir, "global-claude-link")

	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      projectClaudeDir, // as if CLAUDE_CONFIG_DIR is set
		GlobalClaudeDir: globalClaudeDir,
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
	}

	// Create hub + profile
	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}
	mgr := NewManager(paths)
	manifest := NewManifest("myprofile", "test")
	if _, err := mgr.Create("myprofile", manifest); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// SetActive on the default manager uses ClaudeDir (project path)
	os.MkdirAll(filepath.Dir(projectClaudeDir), 0755)
	if err := mgr.SetActive("myprofile"); err != nil {
		t.Fatalf("SetActive() with project ClaudeDir error: %v", err)
	}

	// Verify symlink was created at project path, NOT global path
	info, _ := os.Lstat(projectClaudeDir)
	if info == nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink at project ClaudeDir")
	}
	if _, err := os.Lstat(globalClaudeDir); !os.IsNotExist(err) {
		t.Error("global path should not have been touched")
	}

	// Now create a manager with GlobalClaudeDir — simulates `ccp use -g`
	globalPaths := *paths
	globalPaths.ClaudeDir = paths.GlobalClaudeDir
	globalMgr := NewManager(&globalPaths)

	if err := globalMgr.SetActive("myprofile"); err != nil {
		t.Fatalf("SetActive() with GlobalClaudeDir error: %v", err)
	}

	// Verify symlink was created at global path
	info, _ = os.Lstat(globalClaudeDir)
	if info == nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink at global ClaudeDir")
	}
}

func TestProfileManager_SetActive_NonexistentProfile(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
	}

	mgr := NewManager(paths)

	err := mgr.SetActive("nonexistent")
	if err == nil {
		t.Error("expected error when setting active to nonexistent profile")
	}
}

func TestProfileManager_GetActive_NotSymlink(t *testing.T) {
	testDir := t.TempDir()
	claudeDir := filepath.Join(testDir, "claude")
	os.MkdirAll(claudeDir, 0755) // real dir, not symlink

	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      claudeDir,
		GlobalClaudeDir: claudeDir,
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
	}

	mgr := NewManager(paths)
	active, err := mgr.GetActive()
	if err != nil {
		t.Fatalf("GetActive() error: %v", err)
	}
	if active != nil {
		t.Error("expected nil active profile when ClaudeDir is a real directory")
	}
}

func TestProfileManager_SetActive_SwapExistingSymlink(t *testing.T) {
	testDir := t.TempDir()
	claudeLink := filepath.Join(testDir, "claude-link")

	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      claudeLink,
		GlobalClaudeDir: claudeLink,
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
	}

	// Create hub + two profiles
	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}
	mgr := NewManager(paths)
	mgr.Create("profile-a", NewManifest("profile-a", "A"))
	mgr.Create("profile-b", NewManifest("profile-b", "B"))

	// Activate A
	if err := mgr.SetActive("profile-a"); err != nil {
		t.Fatalf("SetActive(a) error: %v", err)
	}
	active, _ := mgr.GetActive()
	if active == nil || active.Name != "profile-a" {
		t.Fatalf("expected active = profile-a, got %v", active)
	}

	// Swap to B
	if err := mgr.SetActive("profile-b"); err != nil {
		t.Fatalf("SetActive(b) error: %v", err)
	}
	active, _ = mgr.GetActive()
	if active == nil || active.Name != "profile-b" {
		t.Fatalf("expected active = profile-b, got %v", active)
	}
}

func TestProfileManager(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
	}

	// Create hub structure
	for _, itemType := range config.AllHubItemTypes() {
		if err := os.MkdirAll(paths.HubItemDir(itemType), 0755); err != nil {
			t.Fatal(err)
		}
	}

	mgr := NewManager(paths)

	// Test create
	manifest := NewManifest("test-profile", "Test")
	p, err := mgr.Create("test-profile", manifest)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if p.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", p.Name, "test-profile")
	}

	// Test exists
	if !mgr.Exists("test-profile") {
		t.Error("Exists() = false, want true")
	}

	// Test get
	got, err := mgr.Get("test-profile")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", got.Name, "test-profile")
	}

	// Test list
	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List) = %d, want 1", len(list))
	}

	// Test delete
	if err := mgr.Delete("test-profile"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if mgr.Exists("test-profile") {
		t.Error("profile still exists after delete")
	}
}

func TestProfileManager_Create_AlreadyExists(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	mgr := NewManager(paths)

	manifest := NewManifest("dup", "first")
	_, err := mgr.Create("dup", manifest)
	if err != nil {
		t.Fatalf("Create() first call error: %v", err)
	}

	// Second create should fail
	manifest2 := NewManifest("dup", "second")
	_, err = mgr.Create("dup", manifest2)
	if err == nil {
		t.Error("expected error creating duplicate profile")
	}
	if !os.IsExist(err) {
		t.Errorf("expected os.ErrExist, got: %v", err)
	}
}

func TestProfileManager_Create_WithHubItemSymlinks(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Create hub items
	skillDir := filepath.Join(paths.HubItemDir(config.HubSkills), "coding")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Coding Skill"), 0644)

	ruleDir := filepath.Join(paths.HubItemDir(config.HubRules), "style-guide")
	os.MkdirAll(ruleDir, 0755)
	os.WriteFile(filepath.Join(ruleDir, "rules.md"), []byte("# Style Guide"), 0644)

	mgr := NewManager(paths)

	manifest := NewManifest("linked-profile", "Profile with hub items")
	manifest.Hub.Skills = []string{"coding"}
	manifest.Hub.Rules = []string{"style-guide"}

	p, err := mgr.Create("linked-profile", manifest)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Verify skill symlink exists
	skillSymlink := filepath.Join(p.Path, "skills", "coding")
	info, err := os.Lstat(skillSymlink)
	if err != nil {
		t.Fatalf("skill symlink stat error: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected skill to be a symlink")
	}

	// Verify the symlink target resolves to the hub item
	resolved, err := filepath.EvalSymlinks(skillSymlink)
	if err != nil {
		t.Fatalf("EvalSymlinks error: %v", err)
	}
	// EvalSymlinks resolves /var → /private/var on macOS, so resolve expected too
	expectedSkillDir, _ := filepath.EvalSymlinks(skillDir)
	if resolved != expectedSkillDir {
		t.Errorf("skill symlink resolved = %q, want %q", resolved, expectedSkillDir)
	}

	// Verify rule symlink exists
	ruleSymlink := filepath.Join(p.Path, "rules", "style-guide")
	info, err = os.Lstat(ruleSymlink)
	if err != nil {
		t.Fatalf("rule symlink stat error: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected rule to be a symlink")
	}
}

func TestProfileManager_Create_SharedDataDirs(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	mgr := NewManager(paths)
	manifest := NewManifest("data-profile", "test data dirs")
	p, err := mgr.Create("data-profile", manifest)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Verify data directories are symlinks to shared
	for _, dataType := range config.AllDataItemTypes() {
		dataPath := filepath.Join(p.Path, string(dataType))
		info, err := os.Lstat(dataPath)
		if err != nil {
			t.Errorf("data dir %s stat error: %v", dataType, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("data dir %s should be a symlink", dataType)
			continue
		}
		resolved, err := filepath.EvalSymlinks(dataPath)
		if err != nil {
			t.Errorf("data dir %s EvalSymlinks error: %v", dataType, err)
			continue
		}
		expectedTarget, _ := filepath.EvalSymlinks(paths.SharedDataDir(dataType))
		if resolved != expectedTarget {
			t.Errorf("data dir %s resolved = %q, want %q", dataType, resolved, expectedTarget)
		}
	}
}

func TestProfileManager_Create_SettingsJSON_WithHooks(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Create a hook in the hub
	hookName := "test-hook"
	hookHubDir := filepath.Join(paths.HubItemDir(config.HubHooks), hookName)
	os.MkdirAll(hookHubDir, 0755)
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
	data, _ := json.Marshal(hooksJSON)
	os.WriteFile(filepath.Join(hookHubDir, "hooks.json"), data, 0644)

	mgr := NewManager(paths)
	manifest := NewManifest("hooks-profile", "has hooks")
	manifest.Hub.Hooks = []string{hookName}

	p, err := mgr.Create("hooks-profile", manifest)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// settings.json should have been generated
	settingsPath := filepath.Join(p.Path, "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("settings.json invalid JSON: %v", err)
	}

	if _, ok := settings["hooks"]; !ok {
		t.Error("settings.json should contain 'hooks' key")
	}
}

func TestProfileManager_LinkHubItem(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Create a hub skill
	skillDir := filepath.Join(paths.HubItemDir(config.HubSkills), "new-skill")
	os.MkdirAll(skillDir, 0755)

	mgr := NewManager(paths)
	manifest := NewManifest("link-test", "")
	mgr.Create("link-test", manifest)

	// Link the skill
	err := mgr.LinkHubItem("link-test", config.HubSkills, "new-skill")
	if err != nil {
		t.Fatalf("LinkHubItem() error: %v", err)
	}

	// Verify symlink
	profileSkillPath := filepath.Join(paths.ProfileDir("link-test"), "skills", "new-skill")
	info, err := os.Lstat(profileSkillPath)
	if err != nil {
		t.Fatalf("symlink stat error: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink after LinkHubItem")
	}

	// Verify manifest was updated
	p, _ := mgr.Get("link-test")
	skills := p.Manifest.GetHubItems(config.HubSkills)
	if len(skills) != 1 || skills[0] != "new-skill" {
		t.Errorf("manifest skills = %v, want [new-skill]", skills)
	}
}

func TestProfileManager_LinkHubItem_NonexistentProfile(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	mgr := NewManager(paths)
	err := mgr.LinkHubItem("nonexistent", config.HubSkills, "some-skill")
	if err == nil {
		t.Error("expected error linking to nonexistent profile")
	}
}

func TestProfileManager_LinkHubItem_MissingHubItem(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	mgr := NewManager(paths)
	manifest := NewManifest("link-missing", "")
	mgr.Create("link-missing", manifest)

	// Try to link a hub item that doesn't exist
	err := mgr.LinkHubItem("link-missing", config.HubSkills, "does-not-exist")
	if err == nil {
		t.Error("expected error linking missing hub item")
	}
}

func TestProfileManager_UnlinkHubItem(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Create a hub skill
	skillDir := filepath.Join(paths.HubItemDir(config.HubSkills), "unlink-skill")
	os.MkdirAll(skillDir, 0755)

	mgr := NewManager(paths)
	manifest := NewManifest("unlink-test", "")
	manifest.Hub.Skills = []string{"unlink-skill"}
	mgr.Create("unlink-test", manifest)

	// Unlink
	err := mgr.UnlinkHubItem("unlink-test", config.HubSkills, "unlink-skill")
	if err != nil {
		t.Fatalf("UnlinkHubItem() error: %v", err)
	}

	// Verify symlink was removed
	profileSkillPath := filepath.Join(paths.ProfileDir("unlink-test"), "skills", "unlink-skill")
	if _, err := os.Lstat(profileSkillPath); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed after UnlinkHubItem")
	}

	// Verify manifest was updated
	p, _ := mgr.Get("unlink-test")
	skills := p.Manifest.GetHubItems(config.HubSkills)
	if len(skills) != 0 {
		t.Errorf("manifest skills = %v, want empty", skills)
	}
}

func TestProfileManager_UnlinkHubItem_NonexistentProfile(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	mgr := NewManager(paths)
	err := mgr.UnlinkHubItem("nonexistent", config.HubSkills, "some-skill")
	if err == nil {
		t.Error("expected error unlinking from nonexistent profile")
	}
}

func TestProfileManager_Get_NonexistentReturnsNil(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	mgr := NewManager(paths)
	p, err := mgr.Get("does-not-exist")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil for nonexistent profile")
	}
}

func TestProfileManager_Get_DirWithoutManifest(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	// Create profile dir without manifest
	os.MkdirAll(filepath.Join(paths.ProfilesDir, "bare"), 0755)

	mgr := NewManager(paths)
	p, err := mgr.Get("bare")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil profile for existing dir")
	}
	if p.Manifest.Name != "bare" {
		t.Errorf("expected auto-generated manifest name = 'bare', got %q", p.Manifest.Name)
	}
}

func TestProfileManager_List_SkipsSharedDir(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	// Create shared dir and a real profile
	os.MkdirAll(paths.SharedDir, 0755)

	mgr := NewManager(paths)
	mgr.Create("real-profile", NewManifest("real-profile", ""))

	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 profile (shared should be skipped), got %d", len(list))
	}
	if list[0].Name != "real-profile" {
		t.Errorf("expected 'real-profile', got %q", list[0].Name)
	}
}

func TestProfileManager_List_EmptyProfilesDir(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:         testDir,
		ClaudeDir:      filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:         filepath.Join(testDir, "hub"),
		ProfilesDir:    filepath.Join(testDir, "profiles"),
		SharedDir:      filepath.Join(testDir, "profiles", "shared"),
		StoreDir:       filepath.Join(testDir, "store"),
	}

	// No profiles dir at all
	mgr := NewManager(paths)
	list, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if list != nil {
		t.Errorf("expected nil, got %v", list)
	}
}

func TestProfileManager_Exists_NotDir(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      testDir,
		ProfilesDir: filepath.Join(testDir, "profiles"),
	}
	os.MkdirAll(paths.ProfilesDir, 0755)

	// Create a file (not a directory) with profile name
	os.WriteFile(filepath.Join(paths.ProfilesDir, "not-a-dir"), []byte("file"), 0644)

	mgr := NewManager(paths)
	if mgr.Exists("not-a-dir") {
		t.Error("Exists() should return false for a file (not dir)")
	}
}

// === Additional manifest tests ===

func TestLoadManifest_TOMLRoundTrip(t *testing.T) {
	testDir := t.TempDir()
	manifestPath := filepath.Join(testDir, "profile.toml")

	m := NewManifest("roundtrip", "TOML round-trip test")
	m.Hub.Skills = []string{"sk1", "sk2"}
	m.Hub.Agents = []string{"ag1"}
	m.Hub.Hooks = []string{"h1", "h2"}
	m.Hub.Rules = []string{"r1"}
	m.Hub.Commands = []string{"c1", "c2", "c3"}
	m.SettingsTemplate = "my-template"

	if err := m.Save(manifestPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	if loaded.Version != ManifestVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, ManifestVersion)
	}
	if loaded.Name != "roundtrip" {
		t.Errorf("Name = %q, want %q", loaded.Name, "roundtrip")
	}
	if loaded.Description != "TOML round-trip test" {
		t.Errorf("Description = %q, want %q", loaded.Description, "TOML round-trip test")
	}
	if loaded.SettingsTemplate != "my-template" {
		t.Errorf("SettingsTemplate = %q, want %q", loaded.SettingsTemplate, "my-template")
	}
	if len(loaded.Hub.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(loaded.Hub.Skills))
	}
	if len(loaded.Hub.Agents) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(loaded.Hub.Agents))
	}
	if len(loaded.Hub.Hooks) != 2 {
		t.Errorf("len(Hooks) = %d, want 2", len(loaded.Hub.Hooks))
	}
	if len(loaded.Hub.Rules) != 1 {
		t.Errorf("len(Rules) = %d, want 1", len(loaded.Hub.Rules))
	}
	if len(loaded.Hub.Commands) != 3 {
		t.Errorf("len(Commands) = %d, want 3", len(loaded.Hub.Commands))
	}
}

func TestLoadManifest_YAMLFallback(t *testing.T) {
	testDir := t.TempDir()
	yamlPath := filepath.Join(testDir, "profile.yaml")

	// Write a YAML manifest (old format, no version field)
	yamlContent := `name: yaml-profile
description: Old YAML profile
hub:
  skills:
    - old-skill
  hooks:
    - old-hook
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	loaded, err := LoadManifest(yamlPath)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}

	if loaded.Name != "yaml-profile" {
		t.Errorf("Name = %q, want %q", loaded.Name, "yaml-profile")
	}
	if loaded.Version != 1 {
		t.Errorf("Version = %d, want 1 (YAML fallback)", loaded.Version)
	}
	if len(loaded.Hub.Skills) != 1 || loaded.Hub.Skills[0] != "old-skill" {
		t.Errorf("Skills = %v, want [old-skill]", loaded.Hub.Skills)
	}
	if len(loaded.Hub.Hooks) != 1 || loaded.Hub.Hooks[0] != "old-hook" {
		t.Errorf("Hooks = %v, want [old-hook]", loaded.Hub.Hooks)
	}
}

func TestManifestPath_TOMLExists(t *testing.T) {
	testDir := t.TempDir()
	tomlPath := filepath.Join(testDir, "profile.toml")
	os.WriteFile(tomlPath, []byte("version = 3\nname = 'test'\n"), 0644)

	got := ManifestPath(testDir)
	if got != tomlPath {
		t.Errorf("ManifestPath() = %q, want %q (TOML should take priority)", got, tomlPath)
	}
}

func TestManifestPath_FallbackToYAML(t *testing.T) {
	testDir := t.TempDir()
	// No .toml file, should fall back to .yaml
	expected := filepath.Join(testDir, "profile.yaml")

	got := ManifestPath(testDir)
	if got != expected {
		t.Errorf("ManifestPath() = %q, want %q (should fall back to YAML)", got, expected)
	}
}

func TestSetHubItems_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		itemType config.HubItemType
		items    []string
	}{
		{"skills", config.HubSkills, []string{"s1", "s2"}},
		{"agents", config.HubAgents, []string{"a1"}},
		{"hooks", config.HubHooks, []string{"h1", "h2", "h3"}},
		{"rules", config.HubRules, []string{"r1"}},
		{"commands", config.HubCommands, []string{"c1", "c2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManifest("test", "")
			m.SetHubItems(tt.itemType, tt.items)

			got := m.GetHubItems(tt.itemType)
			if len(got) != len(tt.items) {
				t.Errorf("GetHubItems(%s) returned %d items, want %d", tt.itemType, len(got), len(tt.items))
			}
			for i, item := range got {
				if item != tt.items[i] {
					t.Errorf("item[%d] = %q, want %q", i, item, tt.items[i])
				}
			}
		})
	}
}

func TestSetHubItems_UnknownType(t *testing.T) {
	m := NewManifest("test", "")
	m.SetHubItems(config.HubItemType("unknown"), []string{"item1"})

	got := m.GetHubItems(config.HubItemType("unknown"))
	if got != nil {
		t.Errorf("GetHubItems(unknown) = %v, want nil", got)
	}
}

func TestNeedsMigration(t *testing.T) {
	tests := []struct {
		version int
		want    bool
	}{
		{0, true},
		{1, true},
		{2, false},
		{3, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("version_%d", tt.version), func(t *testing.T) {
			m := &Manifest{Version: tt.version}
			if got := m.NeedsMigration(); got != tt.want {
				t.Errorf("NeedsMigration() = %v, want %v for version %d", got, tt.want, tt.version)
			}
		})
	}
}

func TestSaveTOML(t *testing.T) {
	testDir := t.TempDir()

	m := NewManifest("toml-test", "SaveTOML test")
	m.Hub.Skills = []string{"sk1"}
	m.Hub.Agents = []string{"ag1"}

	if err := m.SaveTOML(testDir); err != nil {
		t.Fatalf("SaveTOML() error: %v", err)
	}

	// Verify the file was created at profile.toml
	expectedPath := filepath.Join(testDir, "profile.toml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("profile.toml not created: %v", err)
	}

	// Load it back to verify
	loaded, err := LoadManifest(expectedPath)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if loaded.Name != "toml-test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "toml-test")
	}
	if len(loaded.Hub.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(loaded.Hub.Skills))
	}
}

func TestLoadManifest_NonexistentFile(t *testing.T) {
	_, err := LoadManifest("/nonexistent/path/profile.toml")
	if err == nil {
		t.Error("expected error loading nonexistent manifest")
	}
}

// === Additional profile manager tests ===

func TestProfileManager_GetActive_SymlinkExists(t *testing.T) {
	testDir := t.TempDir()
	claudeLink := filepath.Join(testDir, "claude-link")

	paths := &config.Paths{
		CcpDir:          testDir,
		ClaudeDir:       claudeLink,
		GlobalClaudeDir: claudeLink,
		HubDir:          filepath.Join(testDir, "hub"),
		ProfilesDir:     filepath.Join(testDir, "profiles"),
		SharedDir:       filepath.Join(testDir, "profiles", "shared"),
		StoreDir:        filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	mgr := NewManager(paths)
	mgr.Create("active-test", NewManifest("active-test", "test"))

	// Set as active
	if err := mgr.SetActive("active-test"); err != nil {
		t.Fatalf("SetActive() error: %v", err)
	}

	// Get active
	active, err := mgr.GetActive()
	if err != nil {
		t.Fatalf("GetActive() error: %v", err)
	}
	if active == nil {
		t.Fatal("GetActive() returned nil, expected active profile")
	}
	if active.Name != "active-test" {
		t.Errorf("active.Name = %q, want %q", active.Name, "active-test")
	}
}

func TestProfileManager_GetActive_PathDoesNotExist(t *testing.T) {
	testDir := t.TempDir()
	claudeLink := filepath.Join(testDir, "claude-link-nonexistent")

	paths := &config.Paths{
		CcpDir:          testDir,
		ClaudeDir:       claudeLink,
		GlobalClaudeDir: claudeLink,
		HubDir:          filepath.Join(testDir, "hub"),
		ProfilesDir:     filepath.Join(testDir, "profiles"),
		SharedDir:       filepath.Join(testDir, "profiles", "shared"),
		StoreDir:        filepath.Join(testDir, "store"),
	}

	mgr := NewManager(paths)
	// When the ClaudeDir path doesn't exist, IsSymlink returns an error
	_, err := mgr.GetActive()
	if err == nil {
		t.Error("GetActive() should return error when ClaudeDir path does not exist")
	}
}

func TestProfileManager_Delete_Nonexistent(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      testDir,
		ProfilesDir: filepath.Join(testDir, "profiles"),
	}

	mgr := NewManager(paths)
	// Delete nonexistent should not error (RemoveAll is idempotent)
	err := mgr.Delete("nonexistent")
	if err != nil {
		t.Errorf("Delete(nonexistent) error: %v", err)
	}
}

func TestProfileManager_Exists_TrueAndFalse(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:          testDir,
		ClaudeDir:       filepath.Join(testDir, "claude-link"),
		GlobalClaudeDir: filepath.Join(testDir, "claude-link"),
		HubDir:          filepath.Join(testDir, "hub"),
		ProfilesDir:     filepath.Join(testDir, "profiles"),
		SharedDir:       filepath.Join(testDir, "profiles", "shared"),
		StoreDir:        filepath.Join(testDir, "store"),
	}

	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}

	mgr := NewManager(paths)

	if mgr.Exists("not-here") {
		t.Error("Exists(not-here) = true, want false")
	}

	mgr.Create("here", NewManifest("here", ""))
	if !mgr.Exists("here") {
		t.Error("Exists(here) = false, want true")
	}
}

// === Settings Manager tests ===

func TestSettingsManager_LoadSettings_NonexistentFile(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{CcpDir: testDir}
	sm := NewSettingsManager(paths)

	settings, err := sm.LoadSettings(testDir)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if settings == nil {
		t.Fatal("LoadSettings() returned nil, expected empty settings")
	}
	if len(settings.Hooks) != 0 {
		t.Errorf("len(Hooks) = %d, want 0", len(settings.Hooks))
	}
}

func TestSettingsManager_LoadSettings_WithHooks(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{CcpDir: testDir}
	sm := NewSettingsManager(paths)

	settingsContent := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {"command": "/bin/start.sh", "type": "command", "timeout": 30}
        ]
      }
    ]
  }
}`
	os.WriteFile(filepath.Join(testDir, "settings.json"), []byte(settingsContent), 0644)

	settings, err := sm.LoadSettings(testDir)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if len(settings.Hooks) != 1 {
		t.Fatalf("len(Hooks) = %d, want 1", len(settings.Hooks))
	}
	entries := settings.Hooks[config.HookSessionStart]
	if len(entries) != 1 {
		t.Fatalf("len(SessionStart entries) = %d, want 1", len(entries))
	}
	if entries[0].Matcher != "startup" {
		t.Errorf("Matcher = %q, want %q", entries[0].Matcher, "startup")
	}
	if entries[0].Hooks[0].Command != "/bin/start.sh" {
		t.Errorf("Command = %q, want %q", entries[0].Hooks[0].Command, "/bin/start.sh")
	}
	if entries[0].Hooks[0].Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", entries[0].Hooks[0].Timeout)
	}
}

func TestSettingsManager_SaveSettings(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{CcpDir: testDir}
	sm := NewSettingsManager(paths)

	settings := &Settings{
		Hooks: map[config.HookType][]config.SettingsHookEntry{
			config.HookPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []config.SettingsHookCommand{
						{Type: "command", Command: "/bin/check.sh", Timeout: 15},
					},
				},
			},
		},
		rawData: make(map[string]interface{}),
	}

	if err := sm.SaveSettings(testDir, settings); err != nil {
		t.Fatalf("SaveSettings() error: %v", err)
	}

	// Read it back
	data, err := os.ReadFile(filepath.Join(testDir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Error("saved settings.json should contain 'hooks' key")
	}
}

func TestSettingsManager_SyncHooksFromManifest(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{CcpDir: testDir}
	sm := NewSettingsManager(paths)

	manifest := &Manifest{
		Hooks: []config.HookConfig{
			{
				Name:    "my-hook",
				Type:    config.HookSessionStart,
				Command: "/usr/local/bin/hook.sh",
				Timeout: 45,
				Matcher: "startup",
			},
		},
	}

	if err := sm.SyncHooksFromManifest(testDir, manifest); err != nil {
		t.Fatalf("SyncHooksFromManifest() error: %v", err)
	}

	// Verify settings.json was created
	data, err := os.ReadFile(filepath.Join(testDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Error("settings.json should contain 'hooks' key")
	}
}

func TestSettingsManager_SyncHooksFromManifest_DefaultCommand(t *testing.T) {
	testDir := t.TempDir()
	paths := &config.Paths{CcpDir: testDir}
	sm := NewSettingsManager(paths)

	// Hook with no command, should default to bash <hookPath>
	manifest := &Manifest{
		Hooks: []config.HookConfig{
			{
				Name: "auto-hook",
				Type: config.HookStop,
			},
		},
	}

	if err := sm.SyncHooksFromManifest(testDir, manifest); err != nil {
		t.Fatalf("SyncHooksFromManifest() error: %v", err)
	}

	// Load back to verify default command
	loaded, err := sm.LoadSettings(testDir)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	entries := loaded.Hooks[config.HookStop]
	if len(entries) != 1 {
		t.Fatalf("expected 1 Stop entry, got %d", len(entries))
	}
	expectedCmd := fmt.Sprintf("bash %s", filepath.Join(testDir, "hooks", "auto-hook"))
	if entries[0].Hooks[0].Command != expectedCmd {
		t.Errorf("Command = %q, want %q", entries[0].Hooks[0].Command, expectedCmd)
	}
	if entries[0].Hooks[0].Timeout != config.DefaultHookTimeout() {
		t.Errorf("Timeout = %d, want %d", entries[0].Hooks[0].Timeout, config.DefaultHookTimeout())
	}
}

func TestManifest_GetHookConfig(t *testing.T) {
	m := &Manifest{
		Hooks: []config.HookConfig{
			{Name: "hook-a", Type: config.HookSessionStart},
			{Name: "hook-b", Type: config.HookStop},
		},
	}

	got := m.GetHookConfig("hook-a")
	if got == nil {
		t.Fatal("GetHookConfig(hook-a) returned nil")
	}
	if got.Type != config.HookSessionStart {
		t.Errorf("Type = %q, want %q", got.Type, config.HookSessionStart)
	}

	got = m.GetHookConfig("nonexistent")
	if got != nil {
		t.Errorf("GetHookConfig(nonexistent) = %v, want nil", got)
	}
}

func TestManifest_SetHookConfig(t *testing.T) {
	m := &Manifest{
		Hooks: []config.HookConfig{
			{Name: "existing", Type: config.HookSessionStart, Timeout: 30},
		},
	}

	// Update existing
	m.SetHookConfig(config.HookConfig{Name: "existing", Type: config.HookSessionStart, Timeout: 60})
	if m.Hooks[0].Timeout != 60 {
		t.Errorf("updated Timeout = %d, want 60", m.Hooks[0].Timeout)
	}

	// Add new
	m.SetHookConfig(config.HookConfig{Name: "new-hook", Type: config.HookStop})
	if len(m.Hooks) != 2 {
		t.Errorf("len(Hooks) = %d, want 2", len(m.Hooks))
	}
}

func TestManifest_RemoveHookConfig(t *testing.T) {
	m := &Manifest{
		Hooks: []config.HookConfig{
			{Name: "remove-me", Type: config.HookSessionStart},
			{Name: "keep-me", Type: config.HookStop},
		},
	}

	removed := m.RemoveHookConfig("remove-me")
	if !removed {
		t.Error("RemoveHookConfig(remove-me) = false, want true")
	}
	if len(m.Hooks) != 1 {
		t.Errorf("len(Hooks) = %d, want 1", len(m.Hooks))
	}
	if m.Hooks[0].Name != "keep-me" {
		t.Errorf("remaining hook = %q, want %q", m.Hooks[0].Name, "keep-me")
	}

	removed = m.RemoveHookConfig("nonexistent")
	if removed {
		t.Error("RemoveHookConfig(nonexistent) = true, want false")
	}
}
