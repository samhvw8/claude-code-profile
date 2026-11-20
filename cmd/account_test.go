package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"context"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
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

type fakeAccountStore struct {
	mu       sync.Mutex
	secrets  map[string][]byte
	services map[[2]string]bool
}

func (f *fakeAccountStore) Get(key string) ([]byte, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.secrets[key]
	return v, "", ok, nil
}
func (f *fakeAccountStore) Set(string, string, []byte) error       { return nil }
func (f *fakeAccountStore) Delete(string, string) (string, error) { return "", nil }
func (f *fakeAccountStore) Exists(service, acct string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.services[[2]string{service, acct}], nil
}
func (f *fakeAccountStore) ListServices(string) ([]string, error) { return nil, nil }

func installFakeAccountReader(t *testing.T, paths *config.Paths, store *fakeAccountStore, agyCalls *int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"five_hour":{"utilization":12},"seven_day":{"utilization":34}}`))
	}))
	t.Cleanup(srv.Close)
	home := filepath.Dir(paths.GlobalClaudeDir)
	orig := newAccountUsageReader
	newAccountUsageReader = func() (*accountusage.Reader, error) {
		return &accountusage.Reader{
			Store:      store,
			Keychain:   store,
			GOOS:       "darwin",
			URL:        srv.URL,
			HTTP:       srv.Client(),
			UserAgent:  "ccp/test",
			Now:        time.Now,
			FreshFor:   accountusage.FreshFor,
			HomeDir:    home,
			AgyTimeout: time.Second,
			Env:        []string{"PATH=/usr/bin"},
			RunAgy: func(context.Context, string, []string, []string) ([]byte, error) {
				if agyCalls != nil {
					atomic.AddInt32(agyCalls, 1)
				}
				return []byte(`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"window":"5h","remaining_fraction":0.8},{"window":"weekly","remaining_fraction":0.6}]}]}}}`), nil
			},
		}, nil
	}
	t.Cleanup(func() { newAccountUsageReader = orig })
}

func testCredBytes(expires time.Time) []byte {
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "tok", "refreshToken": "ref", "expiresAt": expires.UnixMilli(),
	}})
	return b
}

func TestAccountList_OtherLoginsAndBoundaries(t *testing.T) {
	paths := accountCmdEnv(t, "default", "work")
	if _, _, err := profile.LinkAccount(paths, "gt1", "work", false); err != nil {
		t.Fatal(err)
	}
	home := filepath.Dir(paths.GlobalClaudeDir)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"personal@example.invalid"}}`), 0600)
	os.WriteFile(filepath.Join(paths.ProfileDir("default"), ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"def@example.invalid"}}`), 0600)

	defAbs, _ := filepath.Abs(paths.ProfileDir("default"))
	store := &fakeAccountStore{
		secrets: map[string][]byte{
			"Claude Code-credentials":                                     testCredBytes(time.Now().Add(time.Hour)),
			"Claude Code-credentials-" + migration.ConfigDirHash(filepath.Clean(defAbs)): testCredBytes(time.Now().Add(time.Hour)),
		},
		services: map[[2]string]bool{
			{"gemini", "antigravity"}: true,
		},
	}
	var agyCalls int32
	installFakeAccountReader(t, paths, store, &agyCalls)

	out, err := captureStdout(t, func() error { return runCcp(t, "account") })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--- Accounts ---",
		"--- Other logins (not accounts) ---",
		"KIND", "WHERE", "EMAIL", "5H", "7D", "AS OF",
		"claude global", "~/.claude", "personal@example.invalid",
		"claude profile", "profile default", "def@example.invalid",
		"agy plain", "~/.gemini/antigravity-cli",
		"12%", "34%", "20%", "40%",
		"ccp migrate --accounts --dry-run previews moving profile logins into accounts (it moves every profile, including the global login)",
		"add more agy logins with: ccp account add <name> --harness agy",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ccp account missing %q:\n%s", want, out)
		}
	}

	// ccp account --json keeps managed-only []accountStatusJSON array.
	accountListJSON = true
	t.Cleanup(func() { accountListJSON = false })
	jsonOut, err := captureStdout(t, func() error { return runCcp(t, "account", "--json") })
	if err != nil {
		t.Fatal(err)
	}
	var list []accountStatusJSON
	if err := json.Unmarshal([]byte(jsonOut), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	if len(list) != 1 || list[0].Name != "gt1" {
		t.Errorf("json list = %+v, want only gt1", list)
	}
	accountListJSON = false

	// ccp status never prints Other logins and never triggers agy plain probe.
	callsBeforeStatus := atomic.LoadInt32(&agyCalls)
	statusOut, err := captureStdout(t, func() error { return runCcp(t, "status") })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(statusOut, "Other logins") || atomic.LoadInt32(&agyCalls) != callsBeforeStatus {
		t.Errorf("ccp status leaked other logins (agyCalls %d -> %d):\n%s", callsBeforeStatus, agyCalls, statusOut)
	}

	// --account auto pools never include unmanaged logins.
	if _, err := chooseAccount(paths, "default", profile.AutoAccount, false, profile.HarnessClaude); err == nil || !strings.Contains(err.Error(), "no account is linked") {
		t.Errorf("chooseAccount(default) = %v, want no account linked", err)
	}
}

func TestAccountList_OtherLoginsFooterHintsByKind(t *testing.T) {
	t.Run("global and agy without profile logins", func(t *testing.T) {
		paths := accountCmdEnv(t, "default")
		store := &fakeAccountStore{
			secrets: map[string][]byte{
				"Claude Code-credentials": testCredBytes(time.Now().Add(time.Hour)),
			},
			services: map[[2]string]bool{
				{"gemini", "antigravity"}: true,
			},
		}
		installFakeAccountReader(t, paths, store, nil)
		out, err := captureStdout(t, func() error { return runCcp(t, "account") })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "No accounts yet") || !strings.Contains(out, "Other logins (not accounts)") {
			t.Errorf("missing empty-accounts + other logins:\n%s", out)
		}
		if strings.Contains(out, "ccp migrate --accounts") {
			t.Errorf("unexpected migrate hint when no profile login exists:\n%s", out)
		}
		if !strings.Contains(out, "add more agy logins with: ccp account add <name> --harness agy") {
			t.Errorf("missing agy hint:\n%s", out)
		}
	})

	t.Run("profile login only without agy", func(t *testing.T) {
		paths := accountCmdEnv(t, "work")
		workAbs, _ := filepath.Abs(paths.ProfileDir("work"))
		store := &fakeAccountStore{
			secrets: map[string][]byte{
				"Claude Code-credentials-" + migration.ConfigDirHash(filepath.Clean(workAbs)): testCredBytes(time.Now().Add(time.Hour)),
			},
			services: map[[2]string]bool{},
		}
		installFakeAccountReader(t, paths, store, nil)
		out, err := captureStdout(t, func() error { return runCcp(t, "account") })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "ccp migrate --accounts --dry-run") {
			t.Errorf("missing migrate --dry-run hint:\n%s", out)
		}
		if strings.Contains(out, "--harness agy") {
			t.Errorf("unexpected agy hint:\n%s", out)
		}
	})
}
