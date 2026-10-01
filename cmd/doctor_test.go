package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

func setupDoctorTest(t *testing.T) (*config.Paths, string) {
	t.Helper()
	dir := t.TempDir()
	paths := &config.Paths{
		CcpDir:          dir,
		ClaudeDir:       filepath.Join(dir, "claude"),
		GlobalClaudeDir: filepath.Join(dir, "claude"),
		HubDir:          filepath.Join(dir, "hub"),
		ProfilesDir:     filepath.Join(dir, "profiles"),
		SharedDir:       filepath.Join(dir, "profiles", "shared"),
		StoreDir:        filepath.Join(dir, "store"),
	}
	for _, itemType := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(itemType), 0755)
	}
	gone := paths.HubItemPath(config.HubSkills, "gone")
	os.MkdirAll(gone, 0755)

	manifest := profile.NewManifest("p", "")
	manifest.AddHubItem(config.HubSkills, "gone")
	if _, err := profile.NewManager(paths).Create("p", manifest); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The hub item disappears behind ccp's back (deleted by hand)
	os.RemoveAll(gone)

	// Claude Code's own dangling link is not ccp's business
	profileDir := paths.ProfileDir("p")
	os.MkdirAll(filepath.Join(profileDir, "debug"), 0755)
	os.Symlink("missing.txt", filepath.Join(profileDir, "debug", "latest"))
	return paths, profileDir
}

func TestFindBrokenSymlinks_OnlyHubItemDirs(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)

	got := findBrokenSymlinks(paths)
	want := filepath.Join(profileDir, "skills", "gone")
	if len(got) != 1 || got[0] != want {
		t.Errorf("findBrokenSymlinks = %v, want [%s]", got, want)
	}
}

func TestPruneBrokenLinks_RemovesLinkAndManifestEntry(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)

	removed, err := pruneBrokenLinks(paths, findBrokenSymlinks(paths))
	if err != nil {
		t.Fatalf("pruneBrokenLinks: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if left := findBrokenSymlinks(paths); len(left) != 0 {
		t.Errorf("broken links remain after prune: %v", left)
	}
	if _, err := os.Lstat(filepath.Join(profileDir, "debug", "latest")); err != nil {
		t.Errorf("prune touched Claude Code's debug/latest: %v", err)
	}
	p, _ := profile.NewManager(paths).Get("p")
	if items := p.Manifest.GetHubItems(config.HubSkills); len(items) != 0 {
		t.Errorf("manifest still lists %v", items)
	}
}

// A relative ~/.claude link resolves against its own directory, whatever the cwd.
func TestCheckHealth_RelativeClaudeLinkFromOtherCwd(t *testing.T) {
	paths, _ := setupDoctorTest(t)
	if err := os.Symlink("profiles/p", paths.ClaudeDir); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	for _, issue := range checkHealth(paths) {
		t.Errorf("unexpected health issue: %s", issue)
	}
}

// A dangling link inside a profile-local item is the user's, not ccp's.
func TestFindBrokenSymlinks_IgnoresLinksInsideLocalItems(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	bin := filepath.Join(profileDir, "skills", "mylocal", ".venv", "bin")
	os.MkdirAll(bin, 0755)
	os.Symlink("/nonexistent/python", filepath.Join(bin, "python"))

	got := findBrokenSymlinks(paths)
	want := filepath.Join(profileDir, "skills", "gone")
	if len(got) != 1 || got[0] != want {
		t.Errorf("findBrokenSymlinks = %v, want only [%s]", got, want)
	}
}

// With the hub unreachable every link dangles; pruning must not empty manifests.
func TestPruneBrokenLinks_RefusesWhenHubUnreachable(t *testing.T) {
	paths, _ := setupDoctorTest(t)
	links := findBrokenSymlinks(paths)
	os.RemoveAll(paths.HubDir)

	if _, err := pruneBrokenLinks(paths, links); err == nil {
		t.Error("pruned with the hub unreachable")
	}
	p, _ := profile.NewManager(paths).Get("p")
	if len(p.Manifest.GetHubItems(config.HubSkills)) != 1 {
		t.Error("manifest entries removed while the hub was unreachable")
	}
}

// Pruning a dangling hook link also drops its entry from settings.json.
func TestPruneBrokenLinks_SyncsHooks(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	hookDir := paths.HubItemPath(config.HubHooks, "stop")
	os.MkdirAll(hookDir, 0755)
	os.WriteFile(filepath.Join(hookDir, "hooks.json"),
		[]byte(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "bash ${CLAUDE_PLUGIN_ROOT}/s.sh"}]}]}}`), 0644)
	if err := profile.NewManager(paths).LinkHubItem("p", config.HubHooks, "stop"); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(hookDir) // deleted behind ccp's back

	if _, err := pruneBrokenLinks(paths, findBrokenSymlinks(paths)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(profileDir, "settings.json"))
	if strings.Contains(string(data), "Stop") {
		t.Errorf("pruned hook still in settings.json: %s", data)
	}
}
