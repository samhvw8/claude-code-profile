package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// Promoting a profile-local item must leave the profile linked to the hub copy.
func TestHubAddFromProfile_LinksBack(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	local := filepath.Join(profileDir, "skills", "local")
	os.MkdirAll(local, 0755)
	os.WriteFile(filepath.Join(local, "SKILL.md"), []byte("# local"), 0644)

	hubAddFromProfile = "p"
	t.Cleanup(func() { hubAddFromProfile = "" })
	if err := runHubAddFromProfile(paths, config.HubSkills, "local"); err != nil {
		t.Fatalf("runHubAddFromProfile: %v", err)
	}

	if _, err := os.Stat(filepath.Join(paths.HubItemPath(config.HubSkills, "local"), "SKILL.md")); err != nil {
		t.Errorf("item not copied to hub: %v", err)
	}
	if info, err := os.Lstat(local); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("profile item was not replaced with a hub link")
	}
	p, _ := profile.NewManager(paths).Get("p")
	found := false
	for _, name := range p.Manifest.GetHubItems(config.HubSkills) {
		found = found || name == "local"
	}
	if !found {
		t.Errorf("manifest does not list skills/local: %v", p.Manifest.GetHubItems(config.HubSkills))
	}
}

// An item that already is the hub item (reached through another path) must not
// be promoted: with --replace that deleted the only copy.
func TestPromoteToHub_RefusesItemThatIsTheHubItem(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	hubItem := paths.HubItemPath(config.HubSkills, "shared")
	os.MkdirAll(hubItem, 0755)
	os.WriteFile(filepath.Join(hubItem, "SKILL.md"), []byte("# shared"), 0644)
	os.Symlink(hubItem, filepath.Join(profileDir, "skills", "shared")) // absolute, not ccp's relative form

	hubAddReplace = true
	t.Cleanup(func() { hubAddReplace = false })
	p, _ := profile.NewManager(paths).Get("p")
	if err := promoteToHub(paths, p, config.HubSkills, "shared"); err == nil {
		t.Error("promoteToHub accepted an item that is the hub item")
	}
	if _, err := os.Stat(filepath.Join(hubItem, "SKILL.md")); err != nil {
		t.Errorf("hub item lost: %v", err)
	}
}

// --replace with a dangling profile item must fail before deleting the hub item.
func TestPromoteToHub_ReplaceWithDanglingSourceKeepsHubItem(t *testing.T) {
	paths, profileDir := setupDoctorTest(t)
	hubItem := paths.HubItemPath(config.HubSkills, "x")
	os.MkdirAll(hubItem, 0755)
	os.WriteFile(filepath.Join(hubItem, "SKILL.md"), []byte("# x"), 0644)
	os.Symlink("/nonexistent/x", filepath.Join(profileDir, "skills", "x"))

	hubAddReplace = true
	t.Cleanup(func() { hubAddReplace = false })
	p, _ := profile.NewManager(paths).Get("p")
	if err := promoteToHub(paths, p, config.HubSkills, "x"); err == nil {
		t.Error("promoteToHub accepted a dangling source")
	}
	if _, err := os.Stat(filepath.Join(hubItem, "SKILL.md")); err != nil {
		t.Errorf("hub item deleted: %v", err)
	}
}
