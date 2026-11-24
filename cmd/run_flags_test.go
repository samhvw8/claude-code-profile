package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// Words after "--" that start with "-" are arguments for the default command
// (the account's harness, else claude), not a command name.
func TestParseRunArgs_FlagsOnly(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"--account", "work1", "--", "--resume"}, []string{"claude", "--resume"}},
		{[]string{"dev", "--", "-p", "hi"}, []string{"claude", "-p", "hi"}},
		{[]string{"dev", "--", "--model", "x", "--", "y"}, []string{"claude", "--model", "x", "--", "y"}},
	}
	for _, tt := range tests {
		ra, err := parseRunArgs(tt.args)
		if err != nil {
			t.Errorf("parseRunArgs(%q): %v", tt.args, err)
			continue
		}
		if !slices.Equal(ra.command, tt.want) || ra.commandExplicit {
			t.Errorf("parseRunArgs(%q) = %v explicit=%v, want %v explicit=false", tt.args, ra.command, ra.commandExplicit, tt.want)
		}
	}
	for _, w := range []string{"-", "--", "-h"} {
		ra, err := parseRunArgs([]string{"dev", "--", w})
		if err != nil || ra.help || !slices.Equal(ra.command, []string{"claude", w}) {
			t.Errorf("parseRunArgs(dev -- %s) = %+v, %v", w, ra, err)
		}
	}
	// A named command stays explicit.
	ra, err := parseRunArgs([]string{"dev", "--", "npm", "-v"})
	if err != nil || !ra.commandExplicit || !slices.Equal(ra.command, []string{"npm", "-v"}) {
		t.Errorf("named command: %+v, %v", ra, err)
	}
}

func TestPrepareRunCmd_FlagsOnly(t *testing.T) {
	prep := func(t *testing.T, args ...string) ([]string, string, error) {
		t.Helper()
		ra, err := parseRunArgs(args)
		if err != nil {
			t.Fatalf("parseRunArgs(%v): %v", args, err)
		}
		var path string
		var argv []string
		_, err = stderrOf(t, func() error {
			c, rel, err := prepareRunCmd(must(config.ResolvePaths()), ra)
			if rel != nil {
				rel()
			}
			if c != nil {
				path, argv = c.Path, c.Args
			}
			return err
		})
		return argv, path, err
	}
	agyEnv := func(t *testing.T, names ...string) {
		t.Helper()
		paths := accountCmdEnv(t, "work")
		binDir := filepath.Join(t.TempDir(), "bin")
		installFakeAgy(t, binDir, filepath.Join(t.TempDir(), "unused"))
		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		usage := map[string]float64{}
		for _, n := range names {
			if _, err := profile.CreateAgyAccount(paths, n, "work"); err != nil {
				t.Fatal(err)
			}
			usage[n] = 10
		}
		installFakeUsage(t, usage, nil)
	}
	wantAgy := func(t *testing.T, argv []string, path string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if filepath.Base(path) != profile.HarnessAgy {
			t.Errorf("path = %q, want agy", path)
		}
		want := []string{"--app_data_dir=antigravity-cli-ccp-agy1", "--model", "gemini-4-neon-medium"}
		if !slices.Equal(argv[1:], want) {
			t.Errorf("args = %v, want %v", argv[1:], want)
		}
	}

	t.Run("named agy account", func(t *testing.T) {
		agyEnv(t, "agy1")
		argv, path, err := prep(t, "--account", "agy1", "--", "--model", "gemini-4-neon-medium")
		wantAgy(t, argv, path, err)
	})
	t.Run("auto, every account agy", func(t *testing.T) {
		agyEnv(t, "agy1")
		argv, path, err := prep(t, "work", "--account", "auto", "--", "--model", "gemini-4-neon-medium")
		wantAgy(t, argv, path, err)
	})
	t.Run("profile only runs claude", func(t *testing.T) {
		accountCmdEnv(t, "work")
		argv, _, err := prep(t, "work", "--", "--resume")
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if argv[0] != "claude" || argv[len(argv)-1] != "--resume" {
			t.Errorf("argv = %v, want claude ... --resume", argv)
		}
	})
	t.Run("auto with or-profile stays claude", func(t *testing.T) {
		agyEnv(t, "agy1")
		argv, _, err := prep(t, "work", "--account", "auto", "--or-profile", "--", "--resume")
		if err != nil {
			t.Fatalf("prepareRunCmd: %v", err)
		}
		if argv[0] != "claude" || argv[len(argv)-1] != "--resume" {
			t.Errorf("argv = %v, want claude ... --resume", argv)
		}
	})
	t.Run("broken binding with relink", func(t *testing.T) {
		agyEnv(t, "agy1")
		paths := must(config.ResolvePaths())
		if err := os.WriteFile(filepath.Join(paths.AccountDir("agy1"), ".ccp-account.toml"), []byte("not = [toml"), 0600); err != nil {
			t.Fatal(err)
		}
		argv, path, err := prep(t, "work", "--account", "agy1", "--relink", "--", "--model", "gemini-4-neon-medium")
		wantAgy(t, argv, path, err)
	})
	t.Run("agy flag still refused", func(t *testing.T) {
		agyEnv(t, "agy1")
		_, _, err := prep(t, "--account", "agy1", "--", "--app_data_dir=x")
		if err == nil || !strings.Contains(err.Error(), "--app_data_dir") {
			t.Errorf("err = %v, want --app_data_dir refusal", err)
		}
	})
}
