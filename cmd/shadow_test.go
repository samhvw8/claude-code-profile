package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/source"
	"github.com/samhvw8/claude-code-profile/internal/testutil"
)

// setupSourceItems makes a ccp dir (CCP_DIR) with a git source clone of
// owner/repo holding skills/same and skills/edited, both installed as hub
// copies the way ccp installed them before shadows.
func setupSourceItems(t *testing.T) (*config.Paths, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCP_DIR", dir)
	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	sourceDir := paths.SourceDir("owner/repo")
	for _, name := range []string{"same", "edited"} {
		writeFile(t, filepath.Join(sourceDir, "skills", name, "SKILL.md"), "# "+name)
	}
	testutil.Git(t, sourceDir, "init", "-q")
	testutil.CommitAll(t, sourceDir, "initial")

	registry := loadRegistry(t, paths)
	registry.AddSource("owner/repo", source.Source{Provider: "git", Path: sourceDir,
		Installed: []string{"skills/same", "skills/edited"}})
	if err := registry.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"same", "edited"} {
		if err := source.CopyTree(filepath.Join(sourceDir, "skills", name), paths.HubItemPath(config.HubSkills, name)); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(paths.HubItemPath(config.HubSkills, "edited"), "SKILL.md"), "local edit")
	return paths, sourceDir
}

func loadRegistry(t *testing.T, paths *config.Paths) *source.Registry {
	t.Helper()
	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func installedItems(t *testing.T, paths *config.Paths) []string {
	t.Helper()
	src, err := loadRegistry(t, paths).GetSource("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	return src.Installed
}

func TestCheckSourceItems_ReportsWithoutFix(t *testing.T) {
	paths, _ := setupSourceItems(t)

	issues, fixed := checkSourceItems(paths, false)
	if issues != 2 || fixed != 0 {
		t.Errorf("issues, fixed = %d, %d; want 2, 0", issues, fixed)
	}
	if testutil.IsSymlink(paths.HubItemPath(config.HubSkills, "same")) {
		t.Error("doctor without --fix changed the hub")
	}
}

// --fix replaces the unchanged copy with a shadow and leaves the edited one.
func TestCheckSourceItems_FixMigratesOnlyUnchangedCopies(t *testing.T) {
	paths, _ := setupSourceItems(t)

	issues, fixed := checkSourceItems(paths, true)
	if issues != 1 || fixed != 1 {
		t.Errorf("issues, fixed = %d, %d; want 1, 1", issues, fixed)
	}
	if !testutil.IsSymlink(paths.HubItemPath(config.HubSkills, "same")) {
		t.Error("unchanged copy not replaced by a shadow")
	}
	edited := paths.HubItemPath(config.HubSkills, "edited")
	if testutil.IsSymlink(edited) {
		t.Fatal("edited copy replaced")
	}
	if data, _ := os.ReadFile(filepath.Join(edited, "SKILL.md")); string(data) != "local edit" {
		t.Errorf("edit lost: %q", data)
	}

	// A second run finds the shadow fine and still reports the edit.
	if issues, fixed := checkSourceItems(paths, true); issues != 1 || fixed != 0 {
		t.Errorf("rerun issues, fixed = %d, %d; want 1, 0", issues, fixed)
	}
}

// Doctor's two answers for a differing copy both work: --clone keeps it as a
// local item, and hub remove plus sync takes the source version.
func TestDifferingCopy_KeepOrTakeSource(t *testing.T) {
	paths, _ := setupSourceItems(t)
	edited := paths.HubItemPath(config.HubSkills, "edited")

	// Take the source version.
	hubRemoveForce = true
	t.Cleanup(func() { hubRemoveForce = false })
	if err := runHubRemove(hubRemoveCmd, []string{"skills/edited"}); err != nil {
		t.Fatal(err)
	}
	if err := runSourceSync(); err != nil {
		t.Fatal(err)
	}
	if !testutil.IsSymlink(edited) {
		t.Fatal("hub remove + sync did not recreate the item as a shadow")
	}

	// Keep a copy as a local item.
	same := paths.HubItemPath(config.HubSkills, "same")
	writeFile(t, filepath.Join(same, "SKILL.md"), "mine now")
	sourceInstallClone = true
	t.Cleanup(func() { sourceInstallClone = false })
	if err := runSourceInstall(installCmd, []string{"owner/repo", "skills/same"}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(installedItems(t, paths), "skills/same") {
		t.Error("--clone did not untrack the copy")
	}
	if data, _ := os.ReadFile(filepath.Join(same, "SKILL.md")); string(data) != "mine now" {
		t.Errorf("copy changed: %q", data)
	}
	if issues, _ := checkSourceItems(paths, false); issues != 0 {
		t.Errorf("doctor still reports %d issue(s)", issues)
	}
}

// A sync that cannot install an item keeps it in ccp.toml for the next try.
func TestSync_FailedItemStaysListed(t *testing.T) {
	paths, _ := setupSourceItems(t)
	registry := loadRegistry(t, paths)
	registry.AddInstalled("owner/repo", "skills/gone")
	registry.Save()

	if err := runSourceSync(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(installedItems(t, paths), "skills/gone") {
		t.Error("sync dropped an item it could not install")
	}
}

// A lock that cannot be checked out (offline, force-pushed) is only warned
// about: ccp.toml keeps it. A lock that is not a SHA never reaches git.
func TestSync_KeepsLockOnFailure(t *testing.T) {
	paths, _ := setupSourceItems(t)
	for _, lock := range []string{"0123456789abcdef0123456789abcdef01234567", "--upload-pack=touch /tmp/x"} {
		registry := loadRegistry(t, paths)
		src, _ := registry.GetSource("owner/repo")
		src.Commit = lock
		registry.UpdateSource("owner/repo", *src)
		registry.Save()

		if err := runSourceSync(); err != nil {
			t.Fatal(err)
		}
		src, _ = loadRegistry(t, paths).GetSource("owner/repo")
		if src.Commit != lock {
			t.Errorf("lock %q rewritten to %q", lock, src.Commit)
		}
	}
}

// An edit made through a shadow lives in the clone; source update must not
// reset it away.
func TestSourceUpdate_BlockedByLocalEdit(t *testing.T) {
	_, sourceDir := setupSourceItems(t)
	writeFile(t, filepath.Join(sourceDir, "skills", "same", "SKILL.md"), "edited via shadow")

	if err := runSourceUpdate(sourceUpdateCmd, []string{"owner/repo"}); err == nil {
		t.Error("source update succeeded over a local edit")
	}
	if data, _ := os.ReadFile(filepath.Join(sourceDir, "skills", "same", "SKILL.md")); string(data) != "edited via shadow" {
		t.Errorf("edit lost: %q", data)
	}
}

func TestCheckSourceItems_SweepsLeftovers(t *testing.T) {
	paths, _ := setupSourceItems(t)
	checkSourceItems(paths, true) // migrate first, so only the leftover remains to report
	leftover := filepath.Join(paths.HubItemDir(config.HubSkills), ".same.ccp-reshadow")
	writeFile(t, filepath.Join(leftover, "SKILL.md"), "# same")

	if issues, _ := checkSourceItems(paths, false); issues != 2 {
		t.Errorf("issues = %d, want the leftover and the edited copy", issues)
	}
	checkSourceItems(paths, true)
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Error("leftover not swept")
	}
}

func TestInstallCloneRejectedInSyncMode(t *testing.T) {
	sourceInstallClone = true
	t.Cleanup(func() { sourceInstallClone = false })
	if err := runSourceInstall(installCmd, nil); err == nil {
		t.Error("expected an error for --clone without a package")
	}
}
