package cmd

import (
	"bytes"
	"os"
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
	if _, _, err := profile.LinkAccount(paths, "gt1", "companyA", false); err != nil {
		t.Fatal(err)
	}

	if got, err := activationProfile(paths, "", "gt1"); err != nil || got != "companyA" {
		t.Errorf("named account: %q, %v; want its own profile", got, err)
	}
	if _, err := activationProfile(paths, "", "gt9"); err == nil || !strings.Contains(err.Error(), "does not exist yet") {
		t.Errorf("unknown account: %v", err)
	}
	if got, err := activationProfile(paths, "default", "gt9"); err != nil || got != "default" {
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
	t.Setenv("CLAUDE_CONFIG_DIR", paths.AccountDir("gt1"))
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
	if _, err := chooseAccount(paths, "dev", "auto", false); err == nil || !strings.Contains(err.Error(), "no account is linked") {
		t.Errorf("empty pool: %v", err)
	}
	if _, err := chooseAccount(paths, "dev", "auto", true); err == nil || !strings.Contains(err.Error(), "--relink") {
		t.Errorf("auto with --relink: %v", err)
	}
	if got, err := chooseAccount(paths, "dev", "w1", false); err != nil || got != "w1" {
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
