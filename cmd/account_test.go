package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// captureStdout runs fn and returns what it printed to os.Stdout.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = stdout })
	runErr := fn()
	os.Stdout = stdout
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String(), runErr
}

func TestAccountList_EmptyShowsHowToAdd(t *testing.T) {
	accountCmdEnv(t, "dev")
	for _, args := range [][]string{{"account"}, {"account", "list"}, {"accounts"}} {
		out, err := captureStdout(t, func() error { return runCcp(t, args...) })
		if err != nil {
			t.Fatalf("ccp %v: %v", args, err)
		}
		if !strings.Contains(out, "No accounts yet") || !strings.Contains(out, "ccp account add") {
			t.Errorf("ccp %v printed:\n%s", args, out)
		}
	}
}

func TestAccountAdd_CreatesAndLists(t *testing.T) {
	paths := accountCmdEnv(t, "dev", "work")
	out, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "gt1", "work") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ccp run --account gt1") {
		t.Errorf("add did not say how to log in:\n%s", out)
	}
	if got, err := profile.AccountProfile(paths.AccountDir("gt1")); err != nil || got != "work" {
		t.Fatalf("gt1 bound to %q, %v; want work", got, err)
	}

	accountListJSON = true
	t.Cleanup(func() { accountListJSON = false })
	out, err = captureStdout(t, func() error { return runCcp(t, "account", "list", "--json") })
	if err != nil {
		t.Fatal(err)
	}
	var list []accountStatusJSON
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(list) != 1 || list[0].Name != "gt1" || list[0].Profile != "work" || list[0].LoggedIn {
		t.Errorf("list = %+v; want gt1 on work, not logged in", list)
	}
}

func TestAccountAdd_Refusals(t *testing.T) {
	accountCmdEnv(t, "dev")
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "gt1", "dev") }); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"account", "add", "gt1", "dev"}, "already exists"},
		{[]string{"account", "add", "auto", "dev"}, "reserved"},
		{[]string{"account", "add", "gt2", "nope"}, "profile not found"},
		{[]string{"account", "add", "gt2"}, "no current profile"},
	}
	for _, c := range cases {
		_, err := captureStdout(t, func() error { return runCcp(t, c.args...) })
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ccp %v: err %v, want %q", c.args, err, c.want)
		}
	}
}

func TestAccountAdd_AgyCreatesFoldersMergedSettingsAndListsHarness(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	mainSettings := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	os.MkdirAll(filepath.Dir(mainSettings), 0755)
	os.WriteFile(mainSettings, []byte(`{"theme":"dark","trustedWorkspaces":["/ws/a"]}`), 0644)

	t.Cleanup(func() { accountAddHarness = profile.HarnessClaude })
	out, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "agy1", "work", "--harness", "agy") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ccp run --account agy1") {
		t.Errorf("add output did not mention sign-in command:\n%s", out)
	}
	bound, harness, err := profile.AccountInfo(paths.AccountDir("agy1"))
	if err != nil || bound != "work" || harness != profile.HarnessAgy {
		t.Fatalf("AccountInfo = %q, %q, %v", bound, harness, err)
	}
	dataDir, _ := profile.AgyAccountDataDir("agy1")
	if _, err := os.Stat(filepath.Join(dataDir, "settings.json")); err != nil {
		t.Errorf("merged settings.json missing: %v", err)
	}

	// Logged-out agy account table and JSON output.
	tableOut, err := captureStdout(t, func() error { return runCcp(t, "account", "list") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut, "HARNESS") || !strings.Contains(tableOut, "agy") {
		t.Errorf("account list missing HARNESS/agy:\n%s", tableOut)
	}

	// Mark agy1 logged in (non-empty token) and provide cached usage so status/list show "-" email without running agy.
	os.WriteFile(filepath.Join(dataDir, "antigravity-oauth-token"), []byte("tok"), 0600)
	os.WriteFile(filepath.Join(paths.AccountDir("agy1"), ".ccp-usage.json"), []byte(`{"last":{"fetched_at":"2099-01-01T00:00:00Z","five_hour":{"utilization":10},"seven_day":{"utilization":38.3},"scoped":[{"name":"Claude and GPT models","utilization":97}]}}`), 0644)
	if _, _, err := profile.LinkAccount(paths, "gt1", "work", false); err != nil {
		t.Fatal(err)
	}
	tableOut, err = captureStdout(t, func() error { return runCcp(t, "account", "list") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut, "38%") || !strings.Contains(tableOut, "Claude and GPT models 97%") || !strings.Contains(tableOut, "agy1") || !strings.Contains(tableOut, " - ") {
		t.Errorf("logged-in agy account list:\n%s", tableOut)
	}

	accountListJSON = true
	t.Cleanup(func() { accountListJSON = false })
	jsonOut, err := captureStdout(t, func() error { return runCcp(t, "account", "list", "--json") })
	if err != nil {
		t.Fatal(err)
	}
	var list []accountStatusJSON
	if err := json.Unmarshal([]byte(jsonOut), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	if len(list) != 2 || list[0].Harness != "agy" || list[0].Email != "" || !list[0].LoggedIn || list[1].Harness != "claude" {
		t.Errorf("list = %+v", list)
	}

	statusOut, err := captureStdout(t, func() error { return runCcp(t, "status") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusOut, "HARNESS") || !strings.Contains(statusOut, "agy") {
		t.Errorf("status output missing HARNESS/agy:\n%s", statusOut)
	}
}

func TestAccountAdd_RejectsInvalidHarness(t *testing.T) {
	accountCmdEnv(t, "dev")
	t.Cleanup(func() { accountAddHarness = profile.HarnessClaude })
	_, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "bad1", "dev", "--harness", "bogus") })
	if err == nil || !strings.Contains(err.Error(), "unknown harness") {
		t.Errorf("err = %v, want unknown harness", err)
	}
}
