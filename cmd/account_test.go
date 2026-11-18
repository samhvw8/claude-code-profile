package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
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
