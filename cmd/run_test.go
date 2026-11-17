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
