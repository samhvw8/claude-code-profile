package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/profile"
)

func useAccounts(t *testing.T, args ...string) error {
	t.Helper()
	t.Cleanup(func() {
		useAccountsFlag, useGlobalFlag = false, false
		useCmd.Flags().Lookup("accounts").Changed = false
		useCmd.Flags().Lookup("global").Changed = false
	})
	_, err := captureStdout(t, func() error { return runCcp(t, args...) })
	return err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUseAccounts_Mise(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	t.Chdir(t.TempDir())
	os.WriteFile("mise.toml", []byte("[env]\n# gh wrapper\n_.path = [\"{{config_root}}/bin\"]\n\n[tools]\ngo = \"1\"\n"), 0o644)
	for range 2 { // the second run must change nothing
		if err := useAccounts(t, "use", "work", "--accounts"); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, "mise.toml")
	for _, want := range []string{
		"CLAUDE_CONFIG_DIR = \"" + paths.ProfileDir("work") + "\"",
		"_.path = [\"{{config_root}}/.ccp/bin\", \"{{config_root}}/bin\"]",
		"# gh wrapper", "[tools]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mise.toml lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, ".ccp/bin"); n != 1 {
		t.Errorf(".ccp/bin appears %d times:\n%s", n, got)
	}
	info, err := os.Stat(".ccp/bin/claude")
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("shim missing or not executable: %v", err)
	}
	if shim := readFile(t, ".ccp/bin/claude"); !strings.Contains(shim, "ccp run --account auto --or-profile") || strings.Contains(shim, "work") {
		t.Errorf("shim must call run --or-profile and name no profile:\n%s", shim)
	}
}

func TestUseAccounts_EnvrcAndGitExclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	accountCmdEnv(t, "work")
	t.Chdir(t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	os.WriteFile(".envrc", []byte("export FOO=1"), 0o644)
	for range 2 {
		if err := useAccounts(t, "use", "work", "--accounts"); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, ".envrc"); strings.Count(got, "PATH_add .ccp/bin") != 1 || !strings.Contains(got, "export FOO=1") {
		t.Errorf(".envrc:\n%s", got)
	}
	if got := readFile(t, ".git/info/exclude"); strings.Count(got, ".ccp/") != 1 {
		t.Errorf("exclude:\n%s", got)
	}
}

func TestUseAccounts_Refusals(t *testing.T) {
	accountCmdEnv(t, "work")
	t.Chdir(t.TempDir())
	if err := useAccounts(t, "use", "work", "--accounts", "-g"); err == nil || !strings.Contains(err.Error(), "cannot be used with -g") {
		t.Errorf("-g: err %v", err)
	}
}

func TestEnsureMisePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[env]\nA = \"1\"\n", "[env]\n_.path = [\"{{config_root}}/.ccp/bin\"]\nA = \"1\"\n"},
		{"[tools]\ngo = \"1\"\n", "[tools]\ngo = \"1\"\n\n[env]\n_.path = [\"{{config_root}}/.ccp/bin\"]\n"},
		{"[env]\n_.path = \"./bin\"\n", "[env]\n_.path = [\"{{config_root}}/.ccp/bin\", \"./bin\"]\n"},
		{"[env]\n_.path = []\n", "[env]\n_.path = [\"{{config_root}}/.ccp/bin\"]\n"},
		// _.path of another table is not the env PATH.
		{"[tasks.x.env]\n_.path = \"y\"\n[env]\n", "[tasks.x.env]\n_.path = \"y\"\n[env]\n_.path = [\"{{config_root}}/.ccp/bin\"]\n"},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), "mise.toml")
		os.WriteFile(path, []byte(c.in), 0o644)
		if err := ensureMisePath(path); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := readFile(t, path); got != c.want {
			t.Errorf("in %q\ngot  %q\nwant %q", c.in, got, c.want)
		}
	}
	path := filepath.Join(t.TempDir(), "mise.toml")
	os.WriteFile(path, []byte("[env]\n_.path = [\n  \"a\",\n]\n"), 0o644)
	if err := ensureMisePath(path); err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Errorf("multi-line array: err %v", err)
	}
}

// The shim runs the next real claude on PATH (never itself or a mise shim)
// through ccp run --account auto --or-profile, passing its arguments on.
func TestAccountShim_Exec(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "proj", ".ccp", "bin")
	fakeBin := filepath.Join(dir, "fake")
	miseShims := filepath.Join(dir, "mise", "shims")
	for _, d := range []string{bin, fakeBin, miseShims} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(bin, "claude"), []byte(accountShim), 0o755)
	os.WriteFile(filepath.Join(miseShims, "claude"), []byte("#!/bin/sh\necho WRONG mise shim\n"), 0o755)
	os.WriteFile(filepath.Join(fakeBin, "claude"), []byte("#!/bin/sh\necho real\n"), 0o755)
	// A fake ccp that prints what it was asked to run.
	os.WriteFile(filepath.Join(fakeBin, "ccp"), []byte("#!/bin/sh\necho \"ccp $*\"\n"), 0o755)
	cmd := exec.Command(filepath.Join(bin, "claude"), "-c", "hi there")
	cmd.Env = []string{"PATH=" + strings.Join([]string{bin, miseShims, fakeBin, "/usr/bin", "/bin"}, ":")}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shim: %v\n%s", err, out)
	}
	want := "ccp run --account auto --or-profile -- " + filepath.Join(fakeBin, "claude") + " -c hi there\n"
	if string(out) != want {
		t.Errorf("shim ran\n%q\nwant\n%q", out, want)
	}
}

func TestRunOrProfile(t *testing.T) {
	paths := accountCmdEnv(t, "dev")
	out := filepath.Join(t.TempDir(), "dir")
	run := func() string {
		t.Helper()
		os.Remove(out)
		if err := runCcp(t, "run", "dev", "--account", "auto", "--or-profile", "--", "sh", "-c", "printf %s \"$CLAUDE_CONFIG_DIR\" > "+out); err != nil {
			t.Fatal(err)
		}
		return readFile(t, out)
	}
	if got := run(); got != paths.ProfileDir("dev") {
		t.Errorf("empty pool: ran in %q, want the profile", got)
	}
	acct, _, err := profile.LinkAccount(paths, "w1", "dev", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := run(); got != paths.ProfileDir("dev") {
		t.Errorf("no account logged in: ran in %q, want the profile", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", acct)
	if got := run(); got != acct {
		t.Errorf("already on an account: ran in %q, want %q", got, acct)
	}
	if _, err := parseRunArgs([]string{"dev", "--account", "w1", "--or-profile"}); err == nil {
		t.Error("--or-profile with a named account was accepted")
	}
}
