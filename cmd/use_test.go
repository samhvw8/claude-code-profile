package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// ccp use must write only CLAUDE_CONFIG_DIR. Claude Code loads the profile's
// CLAUDE.md and rules from it; the old --add-dir alias and its env var made
// Claude Code 2.1.326+ ask to trust the profile folder on every start.
func TestUpdateProjectEnvWritesOnlyConfigDir(t *testing.T) {
	for _, file := range []string{"mise.toml", ".envrc"} {
		t.Run(file, func(t *testing.T) {
			t.Chdir(t.TempDir())
			seed := "[env]\n"
			if file == ".envrc" {
				seed = ""
			}
			if err := os.WriteFile(file, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := updateProjectEnv("/p/dev", "dev"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if !strings.Contains(got, "CLAUDE_CONFIG_DIR") || !strings.Contains(got, "/p/dev") {
				t.Errorf("%s lacks CLAUDE_CONFIG_DIR=/p/dev:\n%s", file, got)
			}
			if strings.Contains(got, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD") {
				t.Errorf("%s still sets CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD:\n%s", file, got)
			}
		})
	}
}

// ccp use removes the CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD line that
// ccp versions before 0.53.0 wrote, and leaves every other line alone.
func TestUpdateProjectEnvRemovesStaleAddDirEnvVar(t *testing.T) {
	cases := map[string]string{
		"mise.toml": "[env]\nCLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD = \"1\"\nCLAUDE_CONFIG_DIR = \"/p/old\"\nOTHER = \"x\"\n",
		".envrc":    "export CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=\"1\"\nexport CLAUDE_CONFIG_DIR=\"/p/old\"\nexport OTHER=x\n",
	}
	for file, seed := range cases {
		t.Run(file, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile(file, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := updateProjectEnv("/p/dev", "dev"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if strings.Contains(got, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD") {
				t.Errorf("%s still has the stale env var:\n%s", file, got)
			}
			if !strings.Contains(got, "OTHER") || !strings.Contains(got, "/p/dev") || strings.Contains(got, "/p/old") {
				t.Errorf("%s lost or kept the wrong lines:\n%s", file, got)
			}
		})
	}
}

func TestConfigShellPrintsNoAddDirAlias(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = stdout })
	runErr := runConfigShell(nil, nil)
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	for _, bad := range []string{"--add-dir", "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD", "alias claude="} {
		if strings.Contains(string(out), bad) {
			t.Errorf("config shell output contains %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(string(out), "ccp-use()") {
		t.Errorf("config shell output lost the ccp-use function:\n%s", out)
	}
}
