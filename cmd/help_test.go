package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// run turns off cobra's flag parsing, so it must handle -h and --help itself.
// Before 0.53.2 "ccp run --help" failed with "unknown flag before --".
func TestRunCmd_Help(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--help"},
		{"run", "-h"},
		{"run", "dev", "--account", "auto", "--help"},
	} {
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs(args)
		err := rootCmd.Execute()
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		if err != nil {
			t.Errorf("ccp %s: err %v", strings.Join(args, " "), err)
		}
		if !strings.Contains(out.String(), "--account auto picks") {
			t.Errorf("ccp %s printed no help:\n%s", strings.Join(args, " "), out.String())
		}
	}
}

// A help flag after "--" belongs to the command ccp runs.
func TestParseRunArgs_HelpAfterSeparator(t *testing.T) {
	ra, err := parseRunArgs([]string{"dev", "--", "claude", "--help"})
	if err != nil || ra.help || strings.Join(ra.command, " ") != "claude --help" {
		t.Errorf("got %+v, %v; want command claude --help and no ccp help", ra, err)
	}
}

// hub list and hub add accept every type in config.AllHubItemTypes; their help
// and errors must name them all.
func TestHubHelpNamesEveryType(t *testing.T) {
	for _, typ := range config.AllHubItemTypes() {
		for name, text := range map[string]string{"hub list": hubListCmd.Long, "hub add": hubAddCmd.Long} {
			if !strings.Contains(text, string(typ)) {
				t.Errorf("%s help does not name %s", name, typ)
			}
		}
		if !strings.Contains(hubTypeNames(), string(typ)) {
			t.Errorf("hub type error message does not name %s", typ)
		}
	}
}

func TestStatusHelpNamesAccounts(t *testing.T) {
	if !strings.Contains(statusCmd.Long, "Accounts") {
		t.Errorf("status help does not mention the accounts table:\n%s", statusCmd.Long)
	}
}
