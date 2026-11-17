package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// accountCmdEnv points HOME and CCP_DIR at a temp tree with the given
// profiles; CLAUDE_CONFIG_DIR is unset.
func accountCmdEnv(t *testing.T, profiles ...string) *config.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	os.MkdirAll(filepath.Join(root, "home"), 0755)
	paths := must(config.ResolvePaths())
	for _, it := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(it), 0755)
	}
	mgr := profile.NewManager(paths)
	for _, p := range profiles {
		if _, err := mgr.Create(p, profile.NewManifest(p, "")); err != nil {
			t.Fatalf("Create %s: %v", p, err)
		}
	}
	return paths
}

// runCcp runs the CLI with args and returns its error.
func runCcp(t *testing.T, args ...string) error {
	t.Helper()
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	return rootCmd.Execute()
}

func TestProfileDelete_RefusesWithBoundAccounts(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	if _, _, err := profile.LinkAccount(paths, "work1", "a", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteForce = false })
	err := runCcp(t, "profile", "delete", "a", "--force")
	if err == nil || !strings.Contains(err.Error(), "ccp env <other-profile> --account work1 --relink") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(paths.ProfileDir("a")); err != nil {
		t.Error("profile deleted despite a bound account")
	}
	if err := runCcp(t, "profile", "delete", "b", "--force"); err != nil {
		t.Errorf("unbound profile: %v", err)
	}
}

func TestProfileDelete_RefusesProfileInUseThroughEnv(t *testing.T) {
	for _, viaAccount := range []bool{false, true} {
		t.Run(map[bool]string{false: "profile folder", true: "account folder"}[viaAccount], func(t *testing.T) {
			paths := accountCmdEnv(t, "a", "b")
			dir := paths.ProfileDir("a")
			if viaAccount {
				var err error
				if dir, _, err = profile.LinkAccount(paths, "w", "a", false); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			t.Cleanup(func() { deleteForce = false })
			err := runCcp(t, "profile", "delete", "a", "--force")
			if err == nil || !strings.Contains(err.Error(), "cannot delete active profile") {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestProfileRename_MovesAccounts(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	os.WriteFile(filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), []byte("x"), 0644)
	dir, _, err := profile.LinkAccount(paths, "work1", "a", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runCcp(t, "profile", "rename", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if bound, _ := profile.AccountProfile(dir); bound != "b" {
		t.Errorf("account bound to %q, want b", bound)
	}
	target, err := os.Readlink(filepath.Join(dir, "CLAUDE.md"))
	if err != nil || target != filepath.Join(must(filepath.Abs(paths.ProfileDir("b"))), "CLAUDE.md") {
		t.Errorf("CLAUDE.md -> %q (%v)", target, err)
	}
}

func TestProfileRename_RefusesProfileFolderInEnv(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("a")+"/")
	if err := runCcp(t, "profile", "rename", "a", "b"); err == nil || !strings.Contains(err.Error(), "CLAUDE_CONFIG_DIR") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(paths.ProfileDir("a")); err != nil {
		t.Error("profile renamed")
	}
}

func TestCheckAccounts(t *testing.T) {
	paths := accountCmdEnv(t, "a", "gone")
	os.WriteFile(filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), []byte("x"), 0644)
	for _, acc := range [][2]string{{"ok", "a"}, {"stale", "a"}, {"orphan", "gone"}} {
		if _, _, err := profile.LinkAccount(paths, acc[0], acc[1], false); err != nil {
			t.Fatal(err)
		}
	}
	if problems := checkAccounts(paths); len(problems) != 0 {
		t.Fatalf("healthy accounts reported: %v", problems)
	}
	os.Symlink(filepath.Join(paths.ProfileDir("a"), "missing"), filepath.Join(paths.AccountDir("stale"), "missing"))
	os.RemoveAll(paths.ProfileDir("gone"))

	got := strings.Join(checkAccounts(paths), "\n")
	for _, want := range []string{"account orphan is linked to missing profile 'gone'", "account stale has 1 broken link(s) (missing)"} {
		if !strings.Contains(got, want) {
			t.Errorf("checkAccounts lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "account ok") {
		t.Errorf("healthy account reported:\n%s", got)
	}
}

func TestCheckGlobalSymlink_InsideAccountSession(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	if err := os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	dir, _, err := profile.LinkAccount(paths, "w", "a", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	inSession := must(config.ResolvePaths())
	if target, problems := checkGlobalSymlink(inSession); len(problems) != 0 || filepath.Base(target) != "a" {
		t.Errorf("target %q problems %v", target, problems)
	}
	if issues := checkHealth(inSession); len(issues) != 0 {
		t.Errorf("status health inside an account session: %v", issues)
	}
}

func TestLink_RelinksAccounts(t *testing.T) {
	paths := accountCmdEnv(t, "p")
	os.MkdirAll(filepath.Join(paths.HubItemDir(config.HubSkills), "s"), 0755)
	os.WriteFile(filepath.Join(paths.HubItemDir(config.HubSkills), "s", "SKILL.md"), []byte("---\nname: s\ndescription: d\n---\n"), 0644)
	dir, _, err := profile.LinkAccount(paths, "w", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	// A top-level entry appears in the profile after the account was linked.
	os.WriteFile(filepath.Join(paths.ProfileDir("p"), "NEW.md"), []byte("x"), 0644)

	if err := runCcp(t, "link", "p", "skills/s"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(dir, "NEW.md")); err != nil {
		t.Errorf("NEW.md not linked into the account after ccp link: %v", err)
	}
}

func TestAccountResetLines(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	if lines := accountResetLines(paths, "darwin"); lines != nil {
		t.Errorf("no accounts: %v", lines)
	}
	dir, _, err := profile.LinkAccount(paths, "work1", "a", false)
	if err != nil {
		t.Fatal(err)
	}
	mac := strings.Join(accountResetLines(paths, "darwin"), "\n")
	for _, want := range []string{"Will remove: 1 account folder(s)", "work1 (a)", "Claude Code-credentials-", "left orphaned", "log in again"} {
		if !strings.Contains(mac, want) {
			t.Errorf("darwin plan lacks %q:\n%s", want, mac)
		}
	}
	linux := strings.Join(accountResetLines(paths, "linux"), "\n")
	if !strings.Contains(linux, filepath.Join(dir, ".credentials.json")+" is deleted with the folder") {
		t.Errorf("linux plan:\n%s", linux)
	}
}
