package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

func TestParseRunArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    runArgs
		wantErr bool
	}{
		{args: []string{"dev", "--", "claude", "-p", "hi"}, want: runArgs{profile: "dev", command: []string{"claude", "-p", "hi"}}},
		{args: []string{"default", "--account", "work1", "--", "claude"}, want: runArgs{profile: "default", account: "work1", command: []string{"claude"}}},
		{args: []string{"--account=work1", "--relink", "default", "--", "claude", "--account", "x"}, want: runArgs{profile: "default", account: "work1", relink: true, command: []string{"claude", "--account", "x"}}},
		{args: []string{"dev", "--relink", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--account", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--bogus", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "extra", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--"}, wantErr: true},
		{args: []string{"dev", "claude"}, wantErr: true},
		{args: []string{"--", "claude"}, wantErr: true},
		{args: []string{"dev", "--account=", "--", "claude"}, wantErr: true},
		{args: []string{"dev", "--account", "", "--", "claude"}, wantErr: true},
		// The profile may be left out with --account, and the command defaults to claude.
		{args: []string{"--account", "work1"}, want: runArgs{account: "work1", command: []string{"claude"}}},
		{args: []string{"--account=auto", "--", "claude", "-c"}, want: runArgs{account: "auto", command: []string{"claude", "-c"}}},
		{args: []string{"dev"}, want: runArgs{profile: "dev", command: []string{"claude"}}},
		{args: []string{}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseRunArgs(tt.args)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseRunArgs(%q) err = %v, wantErr %v", tt.args, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if got.profile != tt.want.profile || got.account != tt.want.account || got.relink != tt.want.relink || !slices.Equal(got.command, tt.want.command) {
			t.Errorf("parseRunArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
		}
	}
}

func TestActivationDir_EmptyAccountFlag(t *testing.T) {
	paths := &config.Paths{CcpDir: t.TempDir()}
	paths.ProfilesDir = filepath.Join(paths.CcpDir, "profiles")
	if _, err := activationDir(paths, "dev", "", true, false); err == nil || !strings.Contains(err.Error(), "invalid account name") {
		t.Errorf("--account= gave err %v, want invalid account name", err)
	}
	dir, err := activationDir(paths, "dev", "", false, false)
	if err != nil || dir != paths.ProfileDir("dev") {
		t.Errorf("no flag: %q, %v", dir, err)
	}
}

func TestEnvCmd_EmptyAccountFlag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)
	mgr := profile.NewManager(must(config.ResolvePaths()))
	if _, err := mgr.Create("dev", profile.NewManifest("dev", "")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		envAccount = ""
		envCmd.Flags().Lookup("account").Changed = false
	})
	rootCmd.SetArgs([]string{"env", "dev", "--account="})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "invalid account name") {
		t.Errorf("ccp env dev --account= gave err %v", err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestActivationProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)
	paths := must(config.ResolvePaths())
	mgr := profile.NewManager(paths)
	for _, name := range []string{"default", "companyA"} {
		if _, err := mgr.Create(name, profile.NewManifest(name, "")); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := profile.LinkAccount(paths, "ca1", "companyA", false); err != nil {
		t.Fatal(err)
	}

	if got, err := activationProfile(paths, "", "ca1"); err != nil || got != "companyA" {
		t.Errorf("named account: %q, %v; want its own profile", got, err)
	}
	if _, err := activationProfile(paths, "", "ca9"); err == nil || !strings.Contains(err.Error(), "does not exist yet") {
		t.Errorf("unknown account: %v", err)
	}
	if got, err := activationProfile(paths, "default", "ca9"); err != nil || got != "default" {
		t.Errorf("a named profile wins: %q, %v", got, err)
	}
	if _, err := activationProfile(paths, "nope", ""); err == nil {
		t.Error("unknown profile must fail")
	}
	// auto takes the current profile: here CLAUDE_CONFIG_DIR, as a project's mise.toml sets it.
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("companyA"))
	if got, err := activationProfile(must(config.ResolvePaths()), "", "auto"); err != nil || got != "companyA" {
		t.Errorf("auto: %q, %v", got, err)
	}
	// Inside an account session, auto stays on the account's profile.
	t.Setenv("CLAUDE_CONFIG_DIR", paths.AccountDir("ca1"))
	if got, err := activationProfile(must(config.ResolvePaths()), "", "auto"); err != nil || got != "companyA" {
		t.Errorf("auto inside an account: %q, %v", got, err)
	}
}

func TestChooseAccount_EmptyPool(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)
	paths := must(config.ResolvePaths())
	if _, err := profile.NewManager(paths).Create("dev", profile.NewManifest("dev", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := chooseAccount(paths, "dev", "auto", false, profile.HarnessClaude); err == nil || !strings.Contains(err.Error(), "no account is linked") {
		t.Errorf("empty pool: %v", err)
	}
	if _, err := chooseAccount(paths, "dev", "auto", true, profile.HarnessClaude); err == nil || !strings.Contains(err.Error(), "--relink") {
		t.Errorf("auto with --relink: %v", err)
	}
	if got, err := chooseAccount(paths, "dev", "w1", false, profile.HarnessClaude); err != nil || got != "w1" {
		t.Errorf("named account passes through: %q, %v", got, err)
	}
}

func TestEnvCmd_AutoOnlyForShell(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)
	if _, err := profile.NewManager(must(config.ResolvePaths())).Create("dev", profile.NewManifest("dev", "")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		envAccount, envFormat = "", "shell"
		envCmd.Flags().Lookup("account").Changed = false
		envCmd.Flags().Lookup("format").Changed = false
	})
	rootCmd.SetArgs([]string{"env", "dev", "--account", "auto", "--format", "mise"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "picks once") {
		t.Errorf("auto into mise.toml: %v", err)
	}
}

func installFakeAgy(t *testing.T, binDir, recordFile string) {
	t.Helper()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n{\n  printf 'ARGV=%s\\n' \"$*\"\n  printf 'SSH=%s\\n' \"$SSH_CLIENT\"\n  printf 'CCD=%s\\n' \"$CLAUDE_CONFIG_DIR\"\n} > " + recordFile + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestRun_AgyAccountsAndAuto(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	binDir := filepath.Join(t.TempDir(), "bin")
	record := filepath.Join(t.TempDir(), "ran")
	installFakeAgy(t, binDir, record)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	// Create main settings so each run refreshes merged settings.
	mainSettings := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	os.MkdirAll(filepath.Dir(mainSettings), 0755)
	os.WriteFile(mainSettings, []byte(`{"theme":"dark","trustedWorkspaces":["/main"]}`), 0644)

	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	dataDir, _ := profile.AgyAccountDataDir("agy1")
	os.WriteFile(filepath.Join(dataDir, "antigravity-oauth-token"), []byte("tok"), 0600)
	// Give agy1 a cached usage so --account auto needs no live /usage call.
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), ".ccp-usage.json"), []byte(`{"last":{"fetched_at":"2099-01-01T00:00:00Z","five_hour":{"utilization":10},"seven_day":{"utilization":20}}}`), 0644)
	// Also add a Claude account to verify pools stay separated by harness.
	if _, _, err := profile.LinkAccount(paths, "ca1", "work", false); err != nil {
		t.Fatal(err)
	}

	// 1. Named agy account with no '--' defaults to agy, inserts --app_data_dir, sets SSH_CLIENT, keeps CLAUDE_CONFIG_DIR on profile.
	if err := runCcp(t, "run", "--account", "agy1"); err != nil {
		t.Fatalf("run --account agy1: %v", err)
	}
	got, _ := os.ReadFile(record)
	for _, want := range []string{
		"ARGV=--app_data_dir=antigravity-cli-ccp-agy1\n",
		"SSH=ccp 0 0\n",
		"CCD=" + paths.ProfileDir("work") + "\n",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("run agy1 output missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.AccountDir("agy1"), ".ccp-last-used")); err != nil {
		t.Errorf(".ccp-last-used not touched: %v", err)
	}

	// 2. Main settings updates are synced before the next run while keeping account trustedWorkspaces.
	os.WriteFile(filepath.Join(dataDir, "settings.json"), []byte(`{"theme":"old","trustedWorkspaces":["/acct"]}`), 0600)
	os.Remove(record)
	if err := runCcp(t, "run", "--account", "agy1", "--", "agy", "-p", "hi"); err != nil {
		t.Fatalf("run --account agy1 -- agy -p hi: %v", err)
	}
	got, _ = os.ReadFile(record)
	if !strings.Contains(string(got), "ARGV=--app_data_dir=antigravity-cli-ccp-agy1 -p hi\n") {
		t.Errorf("flag insertion order wrong:\n%s", got)
	}
	merged, _ := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if !strings.Contains(string(merged), `"/main"`) || !strings.Contains(string(merged), `"/acct"`) || !strings.Contains(string(merged), `"dark"`) {
		t.Errorf("settings not refreshed on run:\n%s", merged)
	}

	// 3. --account auto -- agy picks agy1 (not ca1).
	os.Remove(record)
	if err := runCcp(t, "run", "work", "--account", "auto", "--", "agy", "-c"); err != nil {
		t.Fatalf("auto -- agy: %v", err)
	}
	got, _ = os.ReadFile(record)
	if !strings.Contains(string(got), "ARGV=--app_data_dir=antigravity-cli-ccp-agy1 -c\n") {
		t.Errorf("auto -- agy did not launch agy1:\n%s", got)
	}

	// 4. Harness mismatches are refused.
	if err := runCcp(t, "run", "--account", "agy1", "--", "claude"); err == nil || !strings.Contains(err.Error(), "uses harness agy") {
		t.Errorf("agy account running claude: %v", err)
	}
	if err := runCcp(t, "run", "--account", "ca1", "--", "agy"); err == nil || !strings.Contains(err.Error(), "uses harness claude") {
		t.Errorf("claude account running agy: %v", err)
	}
}

func TestRun_MissingAgyErrorsOnlyForAgyAccount(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := profile.LinkAccount(paths, "ca1", "work", false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	if err := runCcp(t, "run", "--account", "agy1"); err == nil || !strings.Contains(err.Error(), "agy not found") {
		t.Errorf("missing agy: %v", err)
	}
	// Claude account still runs even when agy is missing.
	if err := runCcp(t, "run", "--account", "ca1", "--", "sh", "-c", "true"); err != nil {
		t.Errorf("claude account failed without agy: %v", err)
	}
}

func TestEnvAndSession_RefuseAgyAccount(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		envAccount, envFormat = "", "shell"
		envCmd.Flags().Lookup("account").Changed = false
		envCmd.Flags().Lookup("format").Changed = false
		sessionAccount = ""
		sessionCmd.Flags().Lookup("account").Changed = false
	})
	if err := runCcp(t, "env", "--account", "agy1"); err == nil || !strings.Contains(err.Error(), "ccp run --account agy1 -- agy") {
		t.Errorf("env --account agy1: %v", err)
	}
	if err := runCcp(t, "session", "--account", "agy1"); err == nil || !strings.Contains(err.Error(), "ccp run --account agy1 -- agy") {
		t.Errorf("session --account agy1: %v", err)
	}

	// Broken-profile agy binding is still refused by env/session (never relinked to Claude).
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), profile.AccountFile), []byte("harness = \"agy\"\n"), 0644)
	if err := runCcp(t, "env", "work", "--account", "agy1", "--relink"); err == nil || !strings.Contains(err.Error(), "ccp run --account agy1 -- agy") {
		t.Errorf("env --relink on broken agy binding: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(paths.AccountDir("agy1"), ".claude.json")); err == nil {
		t.Error("env --relink created .claude.json inside agy account")
	}
}

func TestRun_AgyEdgeCases(t *testing.T) {
	paths := accountCmdEnv(t, "work", "other")
	binDir := filepath.Join(t.TempDir(), "bin")
	record := filepath.Join(t.TempDir(), "ran")
	installFakeAgy(t, binDir, record)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("SSH_CLIENT", "old-ssh 1 2")

	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	dataDir, _ := profile.AgyAccountDataDir("agy1")
	os.WriteFile(filepath.Join(dataDir, "antigravity-oauth-token"), []byte("tok"), 0600)
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), ".ccp-usage.json"), []byte(`{"last":{"fetched_at":"2099-01-01T00:00:00Z","five_hour":{"utilization":10},"seven_day":{"utilization":20}}}`), 0644)

	// 1. Missing agy account without profile argument still gives --harness agy hint.
	if err := runCcp(t, "run", "--account", "newagy", "--", "agy"); err == nil || !strings.Contains(err.Error(), "--harness agy") {
		t.Errorf("missing agy account without profile: %v", err)
	}

	// 2. Passing --app_data_dir after -- agy is refused.
	if err := runCcp(t, "run", "--account", "agy1", "--", "agy", "--app_data_dir=antigravity-cli"); err == nil || !strings.Contains(err.Error(), "--app_data_dir") {
		t.Errorf("override --app_data_dir: %v", err)
	}

	// 3. --relink moves agy1 to profile other without creating symlinks, and replaces existing SSH_CLIENT.
	os.WriteFile(filepath.Join(paths.ProfileDir("other"), "CLAUDE.md"), []byte("hi"), 0644)
	if err := runCcp(t, "run", "other", "--account", "agy1", "--relink"); err != nil {
		t.Fatalf("relink agy1: %v", err)
	}
	bound, harness, err := profile.AccountInfo(paths.AccountDir("agy1"))
	if err != nil || bound != "other" || harness != profile.HarnessAgy {
		t.Errorf("after relink: %q %q %v", bound, harness, err)
	}
	if _, err := os.Lstat(filepath.Join(paths.AccountDir("agy1"), "CLAUDE.md")); err == nil {
		t.Error("relink created CLAUDE.md symlink in agy account")
	}
	got, _ := os.ReadFile(record)
	if !strings.Contains(string(got), "SSH=ccp 0 0\n") || !strings.Contains(string(got), "CCD="+paths.ProfileDir("other")+"\n") {
		t.Errorf("relinked run output:\n%s", got)
	}

	// 4. Corrupt TOML on agy account with --relink -- agy restores harness = "agy" with no symlinks.
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), profile.AccountFile), []byte("profile = [\n"), 0644)
	if err := runCcp(t, "run", "work", "--account", "agy1", "--relink", "--", "agy"); err != nil {
		t.Fatalf("relink corrupt agy1: %v", err)
	}
	bound, harness, err = profile.AccountInfo(paths.AccountDir("agy1"))
	if err != nil || bound != "work" || harness != profile.HarnessAgy {
		t.Errorf("after corrupt relink: %q %q %v", bound, harness, err)
	}
	if _, err := os.Lstat(filepath.Join(paths.AccountDir("agy1"), "CLAUDE.md")); err == nil {
		t.Error("corrupt relink created symlink")
	}

	// 5. --account auto with explicit '-- claude' on an agy-only profile keeps the Claude error.
	if err := runCcp(t, "run", "work", "--account", "auto", "--", "claude"); err == nil || !strings.Contains(err.Error(), "no account is linked") {
		t.Errorf("explicit claude auto on agy-only profile: %v", err)
	}

	// 6. --account auto --or-profile -- agy ignores CLAUDE_CONFIG_DIR pointing to a Claude account.
	claudeDir, _, err := profile.LinkAccount(paths, "ca1", "work", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	os.Remove(record)
	if err := runCcp(t, "run", "work", "--account", "auto", "--or-profile", "--", "agy"); err != nil {
		t.Fatalf("auto --or-profile -- agy: %v", err)
	}
	got, _ = os.ReadFile(record)
	if !strings.Contains(string(got), "ARGV=--app_data_dir=antigravity-cli-ccp-agy1\n") {
		t.Errorf("expected agy1 launch, got:\n%s", got)
	}
}

func TestPrepareRunCmd_AutoHarness(t *testing.T) {
	prep := func(t *testing.T, paths *config.Paths, args ...string) (*exec.Cmd, string, error) {
		t.Helper()
		ra, err := parseRunArgs(args)
		if err != nil {
			t.Fatalf("parseRunArgs(%v): %v", args, err)
		}
		var cmd *exec.Cmd
		stderr, err := stderrOf(t, func() error {
			c, rel, err := prepareRunCmd(paths, ra)
			if rel != nil {
				rel()
			}
			cmd = c
			return err
		})
		return cmd, stderr, err
	}

	envVal := func(env []string, key string) string {
		prefix := key + "="
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return strings.TrimPrefix(e, prefix)
			}
		}
		return ""
	}

	t.Run("all-agy profile", func(t *testing.T) {
		paths := accountCmdEnv(t, "work")
		binDir := filepath.Join(t.TempDir(), "bin")
		installFakeAgy(t, binDir, filepath.Join(t.TempDir(), "unused"))
		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
			t.Fatal(err)
		}
		if _, err := profile.CreateAgyAccount(paths, "agy2", "work"); err != nil {
			t.Fatal(err)
		}
		installFakeUsage(t, map[string]float64{"agy1": 50, "agy2": 10}, nil)

		cmd, stderr, err := prep(t, paths, "work", "--account", "auto")
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if filepath.Base(cmd.Path) != profile.HarnessAgy {
			t.Errorf("cmd.Path = %q, want agy", cmd.Path)
		}
		if !slices.Contains(cmd.Args[1:], "--app_data_dir=antigravity-cli-ccp-agy2") {
			t.Errorf("cmd.Args = %v, want --app_data_dir=antigravity-cli-ccp-agy2", cmd.Args)
		}
		if got := envVal(cmd.Env, "SSH_CLIENT"); got != "ccp 0 0" {
			t.Errorf("SSH_CLIENT = %q, want %q", got, "ccp 0 0")
		}
		if got := envVal(cmd.Env, "CLAUDE_CONFIG_DIR"); got != paths.ProfileDir("work") {
			t.Errorf("CLAUDE_CONFIG_DIR = %q, want %q", got, paths.ProfileDir("work"))
		}
		detectLine := "ccp: every account of profile work is agy; starting agy\n"
		pickLine := "ccp: account agy2 ("
		di := strings.Index(stderr, detectLine)
		pi := strings.Index(stderr, pickLine)
		if di < 0 || pi < 0 || di >= pi {
			t.Errorf("stderr order wrong:\n%s", stderr)
		}
	})

	t.Run("all-agy pool", func(t *testing.T) {
		paths := accountCmdEnv(t, "work", "other")
		binDir := filepath.Join(t.TempDir(), "bin")
		installFakeAgy(t, binDir, filepath.Join(t.TempDir(), "unused"))
		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
			t.Fatal(err)
		}
		if _, err := profile.CreateAgyAccount(paths, "agy2", "other"); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("agy1"), "gp"); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("agy2"), "gp"); err != nil {
			t.Fatal(err)
		}
		installFakeUsage(t, map[string]float64{"agy1": 5, "agy2": 60}, nil)

		cmd, stderr, err := prep(t, paths, "other", "--account", "auto:gp")
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if filepath.Base(cmd.Path) != profile.HarnessAgy {
			t.Errorf("cmd.Path = %q, want agy", cmd.Path)
		}
		if !slices.Contains(cmd.Args[1:], "--app_data_dir=antigravity-cli-ccp-agy1") {
			t.Errorf("cmd.Args = %v, want --app_data_dir=antigravity-cli-ccp-agy1", cmd.Args)
		}
		if got := envVal(cmd.Env, "CLAUDE_CONFIG_DIR"); got != paths.ProfileDir("other") {
			t.Errorf("CLAUDE_CONFIG_DIR = %q, want %q", got, paths.ProfileDir("other"))
		}
		detectLine := "ccp: every account of pool gp is agy; starting agy\n"
		pickLine := "ccp: account agy1 (pool gp, "
		di := strings.Index(stderr, detectLine)
		pi := strings.Index(stderr, pickLine)
		if di < 0 || pi < 0 || di >= pi {
			t.Errorf("stderr order wrong:\n%s", stderr)
		}
	})

	t.Run("mixed pool stays claude", func(t *testing.T) {
		paths := accountCmdEnv(t, "work")
		if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := profile.LinkAccount(paths, "c1", "work", false); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("agy1"), "mix"); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("c1"), "mix"); err != nil {
			t.Fatal(err)
		}
		installFakeLister(t, &fakeLister{})
		installFakeUsage(t, map[string]float64{"agy1": 5, "c1": 25}, nil)

		cmd, stderr, err := prep(t, paths, "work", "--account", "auto:mix")
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if cmd.Args[0] != profile.HarnessClaude {
			t.Errorf("cmd.Args[0] = %q, want claude", cmd.Args[0])
		}
		if got := envVal(cmd.Env, "CLAUDE_CONFIG_DIR"); got != paths.AccountDir("c1") {
			t.Errorf("CLAUDE_CONFIG_DIR = %q, want %q", got, paths.AccountDir("c1"))
		}
		if strings.Contains(stderr, "starting agy") {
			t.Errorf("unexpected agy notice on mixed pool:\n%s", stderr)
		}
	})

	t.Run("all-Claude stays claude", func(t *testing.T) {
		paths := accountCmdEnv(t, "work")
		if _, _, err := profile.LinkAccount(paths, "c1", "work", false); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("c1"), "cp"); err != nil {
			t.Fatal(err)
		}
		installFakeLister(t, &fakeLister{})
		installFakeUsage(t, map[string]float64{"c1": 15}, nil)

		for _, args := range [][]string{
			{"work", "--account", "auto"},
			{"work", "--account", "auto:cp"},
		} {
			cmd, stderr, err := prep(t, paths, args...)
			if err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			if cmd.Args[0] != profile.HarnessClaude {
				t.Errorf("%v: cmd.Args[0] = %q, want claude", args, cmd.Args[0])
			}
			if strings.Contains(stderr, "starting agy") {
				t.Errorf("%v: unexpected agy notice:\n%s", args, stderr)
			}
		}
	})

	t.Run("no accounts keeps today's error", func(t *testing.T) {
		paths := accountCmdEnv(t, "empty")
		_, stderr, err := prep(t, paths, "empty", "--account", "auto")
		if !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), `no account is linked to profile "empty"`) {
			t.Errorf("empty profile err = %v", err)
		}
		if stderr != "" {
			t.Errorf("empty profile stderr = %q, want empty", stderr)
		}

		_, stderr, err = prep(t, paths, "empty", "--account", "auto:nope")
		if !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), `pool "nope" has no Claude account`) {
			t.Errorf("empty pool err = %v", err)
		}
		if stderr != "" {
			t.Errorf("empty pool stderr = %q, want empty", stderr)
		}
	})

	t.Run("explicit -- claude on all-agy pool keeps today's error", func(t *testing.T) {
		paths := accountCmdEnv(t, "work")
		if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("agy1"), "gp"); err != nil {
			t.Fatal(err)
		}

		_, stderr, err := prep(t, paths, "work", "--account", "auto:gp", "--", "claude")
		if !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), `pool "gp" has no Claude account`) {
			t.Errorf("explicit claude on all-agy pool err = %v", err)
		}
		if stderr != "" {
			t.Errorf("explicit claude stderr = %q, want empty", stderr)
		}
	})

	t.Run("unreadable account ignored", func(t *testing.T) {
		paths := accountCmdEnv(t, "work", "onlybroken")
		binDir := filepath.Join(t.TempDir(), "bin")
		installFakeAgy(t, binDir, filepath.Join(t.TempDir(), "unused"))
		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetAccountPool(paths.AccountDir("agy1"), "gp"); err != nil {
			t.Fatal(err)
		}
		brokenDir := paths.AccountDir("broken")
		if err := os.MkdirAll(brokenDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(brokenDir, profile.AccountFile), []byte("profile = [\n"), 0644); err != nil {
			t.Fatal(err)
		}
		installFakeUsage(t, map[string]float64{"agy1": 10}, nil)

		// Profile with agy1 + unreadable broken detects agy.
		cmd, stderr, err := prep(t, paths, "work", "--account", "auto")
		if err != nil {
			t.Fatalf("profile with unreadable: %v", err)
		}
		if filepath.Base(cmd.Path) != profile.HarnessAgy {
			t.Errorf("profile with unreadable: cmd.Path = %q, want agy", cmd.Path)
		}
		if !strings.Contains(stderr, "ccp: every account of profile work is agy; starting agy\n") {
			t.Errorf("profile with unreadable stderr:\n%s", stderr)
		}

		// Pool with agy1 + unreadable broken detects agy.
		cmd, stderr, err = prep(t, paths, "work", "--account", "auto:gp")
		if err != nil {
			t.Fatalf("pool with unreadable: %v", err)
		}
		if filepath.Base(cmd.Path) != profile.HarnessAgy {
			t.Errorf("pool with unreadable: cmd.Path = %q, want agy", cmd.Path)
		}
		if !strings.Contains(stderr, "ccp: every account of pool gp is agy; starting agy\n") {
			t.Errorf("pool with unreadable stderr:\n%s", stderr)
		}

		// Profile with no readable accounts stays claude and returns today's error.
		_, stderr, err = prep(t, paths, "onlybroken", "--account", "auto")
		if !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), `no account is linked to profile "onlybroken"`) {
			t.Errorf("onlybroken err = %v", err)
		}
		if stderr != "" {
			t.Errorf("onlybroken stderr = %q, want empty", stderr)
		}

		// Direct check: allAgyAccounts ignores entries with Err != nil.
		if !allAgyAccounts([]profile.Account{
			{Name: "broken", Err: errors.New("corrupt")},
			{Name: "agy1", Harness: profile.HarnessAgy},
		}) {
			t.Error("allAgyAccounts did not ignore unreadable account")
		}
		if allAgyAccounts([]profile.Account{{Name: "broken", Err: errors.New("corrupt")}}) {
			t.Error("allAgyAccounts returned true for only unreadable accounts")
		}
	})
}
