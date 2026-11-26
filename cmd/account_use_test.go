package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// accountUse runs 'ccp account use' and resets its flags afterwards.
func accountUse(t *testing.T, args ...string) (string, error) {
	t.Helper()
	reset := func() {
		accountUseProfile, accountUseClear = "", false
		for _, f := range []string{"profile", "clear"} {
			accountUseCmd.Flags().Lookup(f).Changed = false
		}
	}
	reset() // cobra keeps flag values between Execute calls
	t.Cleanup(reset)
	return captureStdout(t, func() error { return runCcp(t, append([]string{"account", "use"}, args...)...) })
}

func ruleOf(t *testing.T, paths *config.Paths, name string) string {
	t.Helper()
	m, err := profile.LoadManifest(profile.ManifestPath(paths.ProfileDir(name)))
	if err != nil {
		t.Fatal(err)
	}
	return m.Account
}

func TestAccountUse_SetShowClear(t *testing.T) {
	paths := accountCmdEnv(t, "dev", "other")
	if _, _, err := profile.LinkAccount(paths, "w1", "dev", false); err != nil {
		t.Fatal(err)
	}
	out, err := accountUse(t, "w1", "--profile", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(t, paths, "dev"); got != "w1" {
		t.Errorf("rule = %q, want w1", got)
	}
	shim := globalShimPath(paths)
	if got := readFile(t, shim); got != globalShimScript() {
		t.Errorf("global shim:\n%s", got)
	}
	if !strings.Contains(out, "_.path") {
		t.Errorf("no PATH hint while claude is not the shim:\n%s", out)
	}
	if out, _ := accountUse(t, "--profile", "dev"); !strings.Contains(out, "account rule w1") {
		t.Errorf("show: %q", out)
	}
	if _, err := accountUse(t, "auto:company", "--profile", "other"); err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(t, paths, "other"); got != "auto:company" {
		t.Errorf("rule = %q", got)
	}
	if _, err := accountUse(t, "--clear", "--profile", "dev"); err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(t, paths, "dev"); got != "" {
		t.Errorf("cleared rule = %q", got)
	}
	// The manifest keeps every other field.
	m, _ := profile.LoadManifest(profile.ManifestPath(paths.ProfileDir("dev")))
	if m.Name != "dev" {
		t.Errorf("manifest name = %q", m.Name)
	}
}

// Without --profile the rule goes to the current profile: the project's
// CLAUDE_CONFIG_DIR, else the ~/.claude target.
func TestAccountUse_CurrentProfile(t *testing.T) {
	paths := accountCmdEnv(t, "dev", "proj")
	if err := os.Symlink(paths.ProfileDir("dev"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	if _, err := accountUse(t, "auto"); err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(t, paths, "dev"); got != "auto" {
		t.Errorf("global profile rule = %q", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("proj"))
	if _, err := accountUse(t, "auto:p1"); err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(t, paths, "proj"); got != "auto:p1" {
		t.Errorf("project profile rule = %q", got)
	}
	if got := ruleOf(t, paths, "dev"); got != "auto" {
		t.Errorf("global rule changed to %q", got)
	}
}

func TestAccountUse_Refusals(t *testing.T) {
	paths := accountCmdEnv(t, "dev", "other")
	if _, _, err := profile.LinkAccount(paths, "w2", "other", false); err != nil {
		t.Fatal(err)
	}
	if _, err := profile.CreateAgyAccount(paths, "g1", "dev"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"nope", "--profile", "dev"}, "does not exist"},
		{[]string{"w2", "--profile", "dev"}, "linked to profile"},
		{[]string{"g1", "--profile", "dev"}, "agy account"},
		{[]string{"auto:", "--profile", "dev"}, "pool name"},
		{[]string{"w2", "--clear", "--profile", "dev"}, "not both"},
		{[]string{"auto", "--profile", "missing"}, "profile not found"},
	} {
		if _, err := accountUse(t, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
	if got := ruleOf(t, paths, "dev"); got != "" {
		t.Errorf("a refused rule was saved: %q", got)
	}
}

// ccp run --profile-rule (the global shim) runs on the loaded profile's rule,
// and with no rule or no usable account leaves the environment unchanged.
func TestRunProfileRule(t *testing.T) {
	paths := accountCmdEnv(t, "dev")
	if err := os.Symlink(paths.ProfileDir("dev"), paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "dir")
	run := func() string {
		t.Helper()
		os.Remove(out)
		if err := runCcp(t, "run", "--profile-rule", "--", "sh", "-c", "printf %s \"$CLAUDE_CONFIG_DIR\" > "+out); err != nil {
			t.Fatal(err)
		}
		return readFile(t, out)
	}
	setRule := func(rule string) {
		t.Helper()
		m, _ := profile.LoadManifest(profile.ManifestPath(paths.ProfileDir("dev")))
		m.Account = rule
		if err := m.Save(profile.ManifestPath(paths.ProfileDir("dev"))); err != nil {
			t.Fatal(err)
		}
	}

	if got := run(); got != "" {
		t.Errorf("no rule: CLAUDE_CONFIG_DIR = %q, want unset", got)
	}
	setRule("auto")
	if got := run(); got != "" {
		t.Errorf("auto, no account: CLAUDE_CONFIG_DIR = %q, want unset (the global login)", got)
	}
	acct, _, err := profile.LinkAccount(paths, "w1", "dev", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := run(); got != "" {
		t.Errorf("auto, none logged in: CLAUDE_CONFIG_DIR = %q, want unset", got)
	}
	setRule("w1")
	if got := run(); got != acct {
		t.Errorf("named rule: CLAUDE_CONFIG_DIR = %q, want %q", got, acct)
	}
	setRule("gone")
	if got := run(); got != "" {
		t.Errorf("rule naming a missing account: CLAUDE_CONFIG_DIR = %q, want unset", got)
	}

	// A project profile folder stays as it is when no account is usable.
	setRule("auto")
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("dev"))
	if got := run(); got != paths.ProfileDir("dev") {
		t.Errorf("project, none usable: CLAUDE_CONFIG_DIR = %q", got)
	}
	// A session already on an account keeps it.
	t.Setenv("CLAUDE_CONFIG_DIR", acct)
	if got := run(); got != acct {
		t.Errorf("inside an account session: CLAUDE_CONFIG_DIR = %q, want %q", got, acct)
	}

	for _, args := range [][]string{
		{"--profile-rule", "--account", "w1", "--", "x"},
		{"dev", "--profile-rule", "--", "x"},
		{"--profile-rule", "--or-profile", "--", "x"},
	} {
		if _, err := parseRunArgs(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

// The global shim runs 'ccp run --profile-rule' with the next real claude,
// skipping itself, mise shims and any project .ccp/bin shim.
func TestGlobalShim_Exec(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ccp", "bin")
	projBin := filepath.Join(dir, "proj", ".ccp", "bin")
	fakeBin := filepath.Join(dir, "fake")
	for _, d := range []string{bin, projBin, fakeBin} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(bin, "claude"), []byte(globalShimScript()), 0o755)
	os.WriteFile(filepath.Join(projBin, "claude"), []byte("#!/bin/sh\necho WRONG project shim\n"), 0o755)
	os.WriteFile(filepath.Join(fakeBin, "claude"), []byte("#!/bin/sh\necho real\n"), 0o755)
	os.WriteFile(filepath.Join(fakeBin, "ccp"), []byte("#!/bin/sh\necho \"ccp $*\"\n"), 0o755)
	cmd := exec.Command(filepath.Join(bin, "claude"), "-c", "hi")
	cmd.Env = []string{"PATH=" + strings.Join([]string{bin, projBin, fakeBin, "/usr/bin", "/bin"}, ":")}
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shim: %v\n%s", err, got)
	}
	want := "ccp run --profile-rule -- " + filepath.Join(fakeBin, "claude") + " -c hi\n"
	if string(got) != want {
		t.Errorf("shim ran\n%q\nwant\n%q", got, want)
	}
}
