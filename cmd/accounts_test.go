package cmd

import (
	"io"
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
	// Never scan this machine's real Claude sessions from a test.
	installFakeLister(t, &fakeLister{})
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

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	w.Close()
	os.Stderr = orig
	return <-done
}

func TestInit_SymlinkedClaudeKeepsMessages(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	if err := os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { initForce = false })
	if err := runCcp(t, "init"); err == nil || !strings.Contains(err.Error(), "already a symlink") {
		t.Errorf("init: %v", err)
	}
	if err := runCcp(t, "init", "--force"); err == nil || !strings.Contains(err.Error(), "run 'ccp reset' first") {
		t.Errorf("init --force: %v", err)
	}
}

func TestProfileDelete_RefusesGlobalProfileWhileEnvElsewhere(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir)
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("b"))
	t.Cleanup(func() { deleteForce = false })
	if err := runCcp(t, "profile", "delete", "a", "--force"); err == nil || !strings.Contains(err.Error(), "cannot delete active profile") {
		t.Errorf("err = %v", err)
	}
}

func TestProfileRename_SwapsGlobalWhileEnvElsewhere(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir)
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("b"))
	if err := runCcp(t, "profile", "rename", "a", "c"); err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(paths.GlobalClaudeDir); err != nil || filepath.Base(got) != "c" {
		t.Errorf("~/.claude -> %q (%v), want profile c", got, err)
	}
}

func TestProfileRename_RejectsBadNewName(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	for _, name := range []string{"../foo", "shared", "x/y", ".."} {
		if err := runCcp(t, "profile", "rename", "a", name); err == nil {
			t.Errorf("rename to %q accepted", name)
		}
	}
	if _, err := os.Stat(paths.ProfileDir("a")); err != nil {
		t.Errorf("profile moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.CcpDir, "foo")); err == nil {
		t.Error("profile moved out of profiles/")
	}
}

func TestProfileDelete_RefusesUnreadableBinding(t *testing.T) {
	for _, content := range []string{"profile = [", "profile = \"shared\"\n"} {
		paths := accountCmdEnv(t, "a", "b")
		os.MkdirAll(paths.AccountDir("broken"), 0755)
		os.WriteFile(filepath.Join(paths.AccountDir("broken"), profile.AccountFile), []byte(content), 0644)
		t.Cleanup(func() { deleteForce = false })
		err := runCcp(t, "profile", "delete", "b", "--force")
		if err == nil || !strings.Contains(err.Error(), "broken") {
			t.Errorf("binding %q: err = %v", content, err)
		}
		if _, err := os.Stat(paths.ProfileDir("b")); err != nil {
			t.Errorf("binding %q: profile deleted", content)
		}
	}
}

func TestProfileRename_WarnsAboutUnreadableBinding(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	os.MkdirAll(paths.AccountDir("broken"), 0755)
	os.WriteFile(filepath.Join(paths.AccountDir("broken"), profile.AccountFile), []byte("profile = ["), 0644)
	var err error
	out := captureStderr(t, func() { err = runCcp(t, "profile", "rename", "a", "b") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "account broken was not moved") {
		t.Errorf("stderr:\n%s", out)
	}
}

// accountWithoutSkills links account w to profile p while p has no skills/,
// so a later change that creates skills/ is a new top-level entry.
func accountWithoutSkills(t *testing.T, paths *config.Paths, p string) string {
	t.Helper()
	os.RemoveAll(filepath.Join(paths.ProfileDir(p), "skills"))
	dir, _, err := profile.LinkAccount(paths, "w", p, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "skills")); err == nil {
		t.Fatal("setup: account already has skills")
	}
	return dir
}

func hubSkill(t *testing.T, paths *config.Paths, name string) {
	t.Helper()
	dir := filepath.Join(paths.HubItemDir(config.HubSkills), name)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: d\n---\n"), 0644)
}

func TestInstallLink_RelinksAccountsOfNonActiveProfile(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir) // a is active, b is not
	hubSkill(t, paths, "s")
	dir := accountWithoutSkills(t, paths, "b")
	if err := linkInstalled(paths, "b", []string{"skills/s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "s")); err != nil {
		t.Errorf("account of non-active profile b does not see skills/s: %v", err)
	}
}

func TestBootstrap_RelinksAccounts(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir)
	hubSkill(t, paths, "s")
	if err := profile.NewManager(paths).LinkHubItem("b", config.HubSkills, "s"); err != nil {
		t.Fatal(err)
	}
	dir := accountWithoutSkills(t, paths, "b") // drift: b lost skills/
	t.Cleanup(func() { fixForce = false })
	if err := runCcp(t, "bootstrap"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.ProfileDir("b"), "skills", "s")); err != nil {
		t.Fatalf("setup: bootstrap did not fix b: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "s")); err != nil {
		t.Errorf("account of b does not see the fixed skills/: %v", err)
	}
}

func TestActiveProfileLabel(t *testing.T) {
	paths := accountCmdEnv(t, "a")
	if got := activeProfileLabel(paths); got != "none" {
		t.Errorf("no link: %q", got)
	}
	os.Symlink(paths.ProfileDir("gone"), paths.GlobalClaudeDir)
	if got := activeProfileLabel(paths); got != "gone (missing)" {
		t.Errorf("dangling link: %q", got)
	}
}

func TestReset_FromAccountAndSymlinkedSessions(t *testing.T) {
	for _, mode := range []string{"account", "symlinked CLAUDE_CONFIG_DIR"} {
		t.Run(mode, func(t *testing.T) {
			paths := accountCmdEnv(t, "a", "b")
			os.WriteFile(filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), []byte("A"), 0644)
			os.WriteFile(filepath.Join(paths.ProfileDir("b"), "CLAUDE.md"), []byte("B"), 0644)
			os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir)
			switch mode {
			case "account":
				dir, _, err := profile.LinkAccount(paths, "w", "a", false)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("CLAUDE_CONFIG_DIR", dir)
			default:
				link := filepath.Join(filepath.Dir(paths.GlobalClaudeDir), "project-claude")
				os.Symlink(paths.ProfileDir("b"), link)
				t.Setenv("CLAUDE_CONFIG_DIR", link)
			}
			t.Cleanup(func() { resetForce = false })
			if err := runCcp(t, "reset", "--force"); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(paths.GlobalClaudeDir)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				t.Fatalf("~/.claude not restored as a folder: %v %v", info, err)
			}
			if data, _ := os.ReadFile(filepath.Join(paths.GlobalClaudeDir, "CLAUDE.md")); string(data) != "A" {
				t.Errorf("~/.claude restored from the wrong profile: %q", data)
			}
		})
	}
}

func TestHarnessProfile_AccountSessionOnOtherProfile(t *testing.T) {
	paths := accountCmdEnv(t, "a", "b")
	os.Symlink(paths.ProfileDir("a"), paths.GlobalClaudeDir)
	dir, _, err := profile.LinkAccount(paths, "w", "b", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	inSession := must(config.ResolvePaths())
	if p := harnessProfile(inSession, "b"); p != nil {
		t.Errorf("harnesses would follow the account's profile b")
	}
	if p := harnessProfile(inSession, "a"); p == nil || p.Name != "a" {
		t.Errorf("harnesses do not follow the global profile a: %v", p)
	}
}

func TestAgyAccount_GuardsDoctorRenameDeleteAndReset(t *testing.T) {
	paths := accountCmdEnv(t, "a", "gone")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "a"); err != nil {
		t.Fatal(err)
	}
	// checkAccounts skips broken-link checks on agy accounts (even if a broken link is dropped in).
	os.Symlink(filepath.Join(paths.ProfileDir("a"), "missing"), filepath.Join(paths.AccountDir("agy1"), "missing"))
	if problems := checkAccounts(paths); len(problems) != 0 {
		t.Errorf("checkAccounts reported agy broken link: %v", problems)
	}

	// profile delete is not blocked by an agy account (it runs on any
	// profile); it notes the account's default and leaves its binding.
	if _, err := profile.CreateAgyAccount(paths, "agy2", "gone"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteForce = false })
	out, err := captureStdout(t, func() error { return runCcp(t, "profile", "delete", "gone", "--force") })
	if err != nil || !strings.Contains(out, "agy account(s) agy2 defaulted to 'gone'") {
		t.Errorf("delete with agy account: %v\n%s", err, out)
	}
	if bound, _, err := profile.AccountInfo(paths.AccountDir("agy2")); err != nil || bound != "gone" {
		t.Errorf("delete rewrote agy2's binding: %q %v", bound, err)
	}
	os.RemoveAll(paths.AccountDir("agy2"))

	// profile rename rewrites agy1's binding without creating symlinks.
	os.WriteFile(filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), []byte("x"), 0644)
	if err := runCcp(t, "profile", "rename", "a", "b"); err != nil {
		t.Fatal(err)
	}
	bound, harness, err := profile.AccountInfo(paths.AccountDir("agy1"))
	if err != nil || bound != "b" || harness != profile.HarnessAgy {
		t.Errorf("after rename: %q %q %v", bound, harness, err)
	}
	if _, err := os.Lstat(filepath.Join(paths.AccountDir("agy1"), "CLAUDE.md")); err == nil {
		t.Error("rename created CLAUDE.md symlink in agy account")
	}

	// reset plan notes agy token lives outside ~/.ccp (including when binding is corrupt).
	resetText := strings.Join(accountResetLines(paths, "darwin"), "\n")
	if !strings.Contains(resetText, "antigravity-cli-ccp-agy1") || !strings.Contains(resetText, "outside ~/.ccp") {
		t.Errorf("reset plan:\n%s", resetText)
	}
	origBinding, _ := os.ReadFile(filepath.Join(paths.AccountDir("agy1"), profile.AccountFile))
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), profile.AccountFile), []byte("profile = [\n"), 0644)
	corruptResetText := strings.Join(accountResetLines(paths, "darwin"), "\n")
	if !strings.Contains(corruptResetText, "antigravity-cli-ccp-agy1") || !strings.Contains(corruptResetText, "outside ~/.ccp") {
		t.Errorf("corrupt agy reset plan:\n%s", corruptResetText)
	}
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), profile.AccountFile), origBinding, 0644)

	// A missing default profile on an agy account is a note with the
	// --relink hint, not a problem.
	os.RemoveAll(paths.ProfileDir("b"))
	if got := checkAccounts(paths); len(got) != 0 {
		t.Errorf("missing default profile reported as a problem: %v", got)
	}
	if got := strings.Join(accountNotes(paths), "\n"); !strings.Contains(got, "ccp run <profile> --account agy1 --relink") {
		t.Errorf("missing default profile note:\n%s", got)
	}
}

func TestEnvAndSession_AutoExcludesAgy(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := chooseAccount(paths, "work", profile.AutoAccount, false, profile.HarnessClaude); err == nil || !strings.Contains(err.Error(), "no account is linked") {
		t.Errorf("chooseAccount(claude) on agy-only profile = %v", err)
	}
}

func TestReset_AgyAccountKeepsGeminiDataDir(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if err := os.Symlink(paths.ProfileDir("work"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	dataDir, err := profile.AgyAccountDataDir("agy1")
	if err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(dataDir, "antigravity-oauth-token")
	if err := os.WriteFile(tokPath, []byte("keep-token"), 0600); err != nil {
		t.Fatal(err)
	}
	resetForce = true
	t.Cleanup(func() { resetForce = false })
	if _, err := captureStdout(t, func() error { return runCcp(t, "reset", "--force") }); err != nil {
		t.Fatalf("reset --force: %v", err)
	}
	if _, err := os.Stat(paths.CcpDir); !os.IsNotExist(err) {
		t.Errorf("~/.ccp still exists after reset: %v", err)
	}
	got, err := os.ReadFile(tokPath)
	if err != nil || string(got) != "keep-token" {
		t.Errorf("agy token after reset = %q, %v", got, err)
	}
}
