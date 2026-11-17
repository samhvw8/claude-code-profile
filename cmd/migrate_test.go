package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
)

// noLogins is a credential store with nothing in it.
type noLogins struct{}

func (noLogins) Get(string) ([]byte, string, bool, error) { return nil, "", false, nil }
func (noLogins) Set(string, string, []byte) error         { return nil }
func (noLogins) Delete(string, string) (string, error)    { return "", nil }

type noProcs struct{}

func (noProcs) ClaudeProcesses() ([]migration.ClaudeProcess, error) { return nil, nil }

func TestMigrate_AccountsOnlyWithFlag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)

	called := 0
	orig := newAccountMigrator
	newAccountMigrator = func(paths *config.Paths) (*migration.AccountMigrator, error) {
		called++
		return migration.NewAccountMigratorWith(paths, "darwin", noLogins{}, noProcs{}), nil
	}
	t.Cleanup(func() { newAccountMigrator = orig; migrateAccounts = false; migrateDryRun = false })

	migrateAccounts, migrateDryRun = false, false
	if err := runMigrate(migrateCmd, nil); err != nil {
		t.Fatal(err)
	}
	migrateDryRun = true
	if err := runMigrate(migrateCmd, nil); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatalf("plain ccp migrate reached the login migration %d time(s)", called)
	}

	if migrateCmd.SilenceUsage {
		t.Error("plain ccp migrate silenced usage")
	}

	migrateAccounts = true
	if err := runMigrate(migrateCmd, nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Errorf("ccp migrate --accounts called the login migration %d time(s), want 1", called)
	}
}

func TestMigrateAccounts_ErrorPrintsNoUsage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("CCP_DIR", filepath.Join(root, "ccp"))
	os.MkdirAll(filepath.Join(root, "ccp", "hub"), 0755)
	orig := newAccountMigrator
	newAccountMigrator = func(*config.Paths) (*migration.AccountMigrator, error) {
		return nil, errors.New("refused")
	}
	t.Cleanup(func() { newAccountMigrator = orig; migrateAccounts = false; migrateCmd.SilenceUsage = false })

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"migrate", "--accounts"})
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Errorf("usage printed after a login error:\n%s", out.String())
	}
}

func TestReportAccountMigration(t *testing.T) {
	var buf bytes.Buffer
	err := reportAccountMigration(&buf, []migration.AccountMigrationResult{
		{Profile: "default", Status: migration.AccountMoved, Reason: "a -> b", Global: true},
		{Profile: "work", Status: migration.AccountSkipped, Reason: "no login"},
		{Profile: "busy", Status: migration.AccountRefused, Reason: "Claude Code is running on this login (PID 7)"},
	}, false)
	out := buf.String()
	for _, want := range []string{
		"default: moved a -> b",
		"ccp run default --account default -- claude",
		"ccp env default --account default",
		"work: skipped (no login)",
		"busy: refused (Claude Code is running on this login (PID 7))",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if err == nil {
		t.Error("a refused profile did not fail the command")
	}
}
