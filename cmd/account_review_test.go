package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// --or-profile falls back to the profile itself; that fallback is Claude's, so
// agy detection must not turn it into a bare agy on the shared login.
func TestPrepareRunCmd_AutoHarnessOrProfileStaysClaude(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	orig := readAccountUsage
	readAccountUsage = func(accts []profile.Account) ([]accountusage.Status, error) {
		out := make([]accountusage.Status, len(accts))
		for i, a := range accts {
			out[i] = accountusage.Status{Name: a.Name, Profile: a.Profile, Harness: a.Harness, Dir: a.Dir}
		}
		return out, nil
	}
	t.Cleanup(func() { readAccountUsage = orig })

	ra, err := parseRunArgs([]string{"work", "--account", "auto", "--or-profile"})
	if err != nil {
		t.Fatal(err)
	}
	var cmd *exec.Cmd
	stderr, err := stderrOf(t, func() error {
		c, rel, err := prepareRunCmd(paths, ra)
		rel()
		cmd = c
		return err
	})
	if err != nil {
		t.Fatalf("prepareRunCmd: %v", err)
	}
	if filepath.Base(cmd.Path) != "claude" {
		t.Errorf("cmd.Path = %q, want claude (the --or-profile fallback)", cmd.Path)
	}
	if strings.Contains(stderr, "starting agy") {
		t.Errorf("stderr = %q, want no agy detection with --or-profile", stderr)
	}
}

func TestAgyAccountBusy_DataDirForms(t *testing.T) {
	const name = "antigravity-cli-ccp-a1"
	dataDir := filepath.Join(t.TempDir(), ".gemini", name)
	for _, v := range []string{name, name + "/", "./" + name, "x/../" + name, "~/.gemini/" + name, "Antigravity-CLI-ccp-a1", dataDir} {
		procs := []migration.AgyProcess{{PID: 1, AppDataDir: v, ArgsKnown: true}}
		if !agyAccountBusy(procs, nil, name, dataDir) {
			t.Errorf("--app_data_dir=%q: not busy, want busy", v)
		}
	}
	for _, v := range []string{"antigravity-cli-ccp-a2", "antigravity-cli", "antigravity-cli-ccp-a11"} {
		procs := []migration.AgyProcess{{PID: 1, AppDataDir: v, ArgsKnown: true}}
		if agyAccountBusy(procs, nil, name, dataDir) {
			t.Errorf("--app_data_dir=%q: busy, want not busy", v)
		}
	}
}

// A data dir that cannot be removed must leave the account folder, so the
// user can run the command again.
func TestAccountRemove_AgyDataDirFailureKeepsAccount(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	paths := accountCmdEnv(t, "work")
	accountDir, dataDir, _ := setupAgyAccount(t, paths, "a1", "work", "", true)
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)
	parent := filepath.Dir(dataDir)
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0755) })

	_, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--all", "--force") })
	if err == nil {
		t.Fatal("want an error when the agy data dir cannot be removed")
	}
	if _, sErr := os.Stat(accountDir); sErr != nil {
		t.Errorf("account folder is gone after a failed data dir removal: %v", sErr)
	}
}

// Claude Code hashes the absolute CLAUDE_CONFIG_DIR that ccp run passes; a
// relative CCP_DIR must not make remove look up another Keychain service.
func TestAccountRemove_RelativeCcpDirFindsLogin(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if _, _, err := profile.LinkAccount(paths, "c1", "work", false); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(paths.CcpDir))
	t.Setenv("CCP_DIR", filepath.Base(paths.CcpDir))
	abs, err := filepath.Abs(must(config.ResolvePaths()).AccountDir("c1"))
	if err != nil {
		t.Fatal(err)
	}
	key := migration.LoginKey("darwin", abs)
	store := &fakeCredStore{}
	_ = store.Set(key, "sam", []byte("secret"))
	installAccountRemoveSeams(t, "darwin", store, nil, &fakeLister{}, &fakeAgyLister{}, true)

	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "c1", "--all", "--force") }); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(store.deleted, key) {
		t.Errorf("deleted = %v, want the login %q", store.deleted, key)
	}
}

// On macOS Claude Code can keep the login in <account>/.credentials.json when
// the Keychain is not usable; the default mode must not leave that token behind.
func TestAccountRemove_CredsOnlyDarwinRemovesCredentialsFile(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	store := &fakeCredStore{}
	accountDir, _ := setupClaudeAccount(t, paths, "c1", "work", "", store, "darwin", true)
	credFile := filepath.Join(accountDir, ".credentials.json")
	if err := os.WriteFile(credFile, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	installAccountRemoveSeams(t, "darwin", store, nil, &fakeLister{}, &fakeAgyLister{}, true)

	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "c1", "--force") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(credFile); !os.IsNotExist(err) {
		t.Errorf("%s is still there after the default mode (err %v)", credFile, err)
	}
	if _, err := os.Stat(accountDir); err != nil {
		t.Errorf("the default mode removed the account folder: %v", err)
	}
}

// An agy account that lost its binding file is still an agy account: its
// token and data dir must go too.
func TestAccountRemove_AgyWithoutBindingRemovesDataDir(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	accountDir, dataDir, _ := setupAgyAccount(t, paths, "a1", "work", "", true)
	if err := os.Remove(filepath.Join(accountDir, profile.AccountFile)); err != nil {
		t.Fatal(err)
	}
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)

	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--all", "--force") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("agy data dir %s is still there (err %v)", dataDir, err)
	}
}

func TestAccountRemove_SymlinkedAccountFolderRefused(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.AccountsDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, paths.AccountDir("s1")); err != nil {
		t.Fatal(err)
	}
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)

	_, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "s1", "--all", "--force") })
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want a refusal that names the symlink", err)
	}
	if _, err := os.Lstat(paths.AccountDir("s1")); err != nil {
		t.Errorf("the link was removed: %v", err)
	}
}

// After the default mode, ccp's saved usage read must not stay next to "not
// logged in".
func TestAccountRemove_CredsOnlyDropsSavedUsage(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	accountDir, _, _ := setupAgyAccount(t, paths, "a1", "work", "", true)
	usage := filepath.Join(accountDir, accountusage.UsageFile)
	if err := os.WriteFile(usage, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)

	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--force") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(usage); !os.IsNotExist(err) {
		t.Errorf("%s is still there after the default mode (err %v)", usage, err)
	}

	// --json returns on another path; it must drop the file too.
	if err := os.WriteFile(usage, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--force", "--json") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(usage); !os.IsNotExist(err) {
		t.Errorf("%s is still there after the default mode --json (err %v)", usage, err)
	}
}

// An agy start and an agy remove take the same account lock, so a remove
// never deletes the account between a run's check and its process start.
func TestAgyAccount_RunAndRemoveTakeTheAccountLock(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	binDir := filepath.Join(t.TempDir(), "bin")
	installFakeAgy(t, binDir, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	accountDir, _, _ := setupAgyAccount(t, paths, "a1", "work", "", true)

	var locked []string
	orig := lockAccount
	lockAccount = func(dir string) (func(), error) {
		locked = append(locked, filepath.Clean(dir))
		return orig(dir)
	}
	t.Cleanup(func() { lockAccount = orig })

	ra, err := parseRunArgs([]string{"work", "--account", "a1"})
	if err != nil {
		t.Fatal(err)
	}
	_, rel, err := prepareRunCmd(paths, ra)
	rel()
	if err != nil {
		t.Fatalf("prepareRunCmd: %v", err)
	}
	if !slices.Contains(locked, accountDir) {
		t.Errorf("ccp run on an agy account locked %v, want %s", locked, accountDir)
	}

	locked = nil
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--all", "--force") }); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(locked, accountDir) {
		t.Errorf("ccp account remove on an agy account locked %v, want %s", locked, accountDir)
	}
}
