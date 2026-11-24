package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

type fakeCredStore struct {
	items     map[string]fakeCredItem
	getErr    error
	deleteErr error
	deleted   []string
	warn      string
}

type fakeCredItem struct {
	secret []byte
	acct   string
}

func (f *fakeCredStore) Get(key string) ([]byte, string, bool, error) {
	if f.getErr != nil {
		return nil, "", false, f.getErr
	}
	it, ok := f.items[key]
	if !ok {
		return nil, "", false, nil
	}
	return it.secret, it.acct, true, nil
}

func (f *fakeCredStore) Set(key, acct string, secret []byte) error {
	if f.items == nil {
		f.items = map[string]fakeCredItem{}
	}
	f.items[key] = fakeCredItem{secret: secret, acct: acct}
	return nil
}

func (f *fakeCredStore) Delete(key, acct string) (string, error) {
	if f.deleteErr != nil {
		return "", f.deleteErr
	}
	f.deleted = append(f.deleted, key)
	delete(f.items, key)
	return f.warn, nil
}

type fakeAgyLister struct {
	procs []migration.AgyProcess
	err   error
}

func (f *fakeAgyLister) AgyProcesses() ([]migration.AgyProcess, error) {
	return f.procs, f.err
}

func installAccountRemoveSeams(
	t *testing.T,
	goos string,
	claudeStore migration.CredentialStore,
	agyStore migration.CredentialStore,
	claudeLister migration.ProcessLister,
	agyLister migration.AgyProcessLister,
	isTTY bool,
) {
	t.Helper()
	origGOOS := accountRemoveGOOS
	origClaudeStore := newRemoveCredentialStore
	origAgyStore := newRemoveAgyStore
	origAgyLister := newAgyProcessLister
	origTTY := stdinIsTerminal
	t.Cleanup(func() {
		accountRemoveGOOS = origGOOS
		newRemoveCredentialStore = origClaudeStore
		newRemoveAgyStore = origAgyStore
		newAgyProcessLister = origAgyLister
		stdinIsTerminal = origTTY
		accountRemoveForce = false
		accountRemoveCredsOnly = false
		accountListJSON = false
	})

	accountRemoveGOOS = func() string { return goos }
	newRemoveCredentialStore = func(string) (migration.CredentialStore, error) { return claudeStore, nil }
	if agyStore != nil {
		newRemoveAgyStore = func() migration.CredentialStore { return agyStore }
	} else {
		newRemoveAgyStore = func() migration.CredentialStore { return migration.FileStore{} }
	}
	installFakeLister(t, claudeLister)
	if agyLister == nil {
		newAgyProcessLister = func() migration.AgyProcessLister { return nil }
	} else {
		newAgyProcessLister = func() migration.AgyProcessLister { return agyLister }
	}
	stdinIsTerminal = func() bool { return isTTY }
}

func setupClaudeAccount(t *testing.T, paths *config.Paths, name, prof, pool string, store *fakeCredStore, goos string, loggedIn bool) (accountDir, loginKey string) {
	t.Helper()
	if _, _, err := profile.LinkAccount(paths, name, prof, false); err != nil {
		t.Fatal(err)
	}
	accountDir = filepath.Clean(paths.AccountDir(name))
	if pool != "" {
		if err := profile.SetAccountPool(accountDir, pool); err != nil {
			t.Fatal(err)
		}
	}
	loginKey = migration.LoginKey(goos, accountDir)
	if loggedIn {
		secret := []byte(`{"claudeAiOauth":{"accessToken":"tok-1","refreshToken":"ref-1","expiresAt":4102444800000}}`)
		if store != nil {
			_ = store.Set(loginKey, "sam", secret)
		}
		if goos == "linux" {
			if err := os.WriteFile(loginKey, secret, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return accountDir, loginKey
}

func setupAgyAccount(t *testing.T, paths *config.Paths, name, prof, pool string, loggedIn bool) (accountDir, dataDir, tokenPath string) {
	t.Helper()
	if _, err := profile.CreateAgyAccount(paths, name, prof); err != nil {
		t.Fatal(err)
	}
	accountDir = filepath.Clean(paths.AccountDir(name))
	if pool != "" {
		if err := profile.SetAccountPool(accountDir, pool); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	dataDir, err = profile.AgyAccountDataDir(name)
	if err != nil {
		t.Fatal(err)
	}
	tokenPath, err = profile.AgyTokenPath(name)
	if err != nil {
		t.Fatal(err)
	}
	// Put a conversation marker in dataDir to verify --creds-only keeps it.
	if err := os.WriteFile(filepath.Join(dataDir, "conversation.json"), []byte(`{"id":"c1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if loggedIn {
		if err := os.WriteFile(tokenPath, []byte("agy-secret-token"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return accountDir, dataDir, tokenPath
}

func TestAccountRemove_Table(t *testing.T) {
	tests := []struct {
		name             string
		goos             string
		harness          string
		loggedIn         bool
		corruptBinding   bool
		args             []string
		claudeProcs      []migration.ClaudeProcess
		claudeScanErr    error
		nilClaudeLister  bool
		agyProcs         []migration.AgyProcess
		agyScanErr       error
		nilAgyLister     bool
		isTTY            bool
		deleteErr        error
		wantErr          string
		wantFolderExists bool
		wantDataExists   bool
		wantLoginExists  bool
		wantJSONMode     string
		wantRemovedCount int
	}{
		{
			name:             "default_claude_darwin",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			wantFolderExists: false,
			wantLoginExists:  false,
		},
		{
			name:             "default_claude_linux_rm_alias_json",
			goos:             "linux",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "rm", "c1", "--force", "--json"},
			wantFolderExists: false,
			wantLoginExists:  false,
			wantJSONMode:     "all",
			wantRemovedCount: 2,
		},
		{
			name:             "default_claude_no_login_not_error",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         false,
			args:             []string{"account", "remove", "c1", "--force", "--json"},
			wantFolderExists: false,
			wantLoginExists:  false,
			wantJSONMode:     "all",
			wantRemovedCount: 1,
		},
		{
			name:             "default_agy",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force", "--json"},
			wantFolderExists: false,
			wantDataExists:   false,
			wantLoginExists:  false,
			wantJSONMode:     "all",
			wantRemovedCount: 3,
		},
		{
			name:             "creds_only_claude",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--creds-only", "--force", "--json"},
			wantFolderExists: true,
			wantLoginExists:  false,
			wantJSONMode:     "creds-only",
			wantRemovedCount: 1,
		},
		{
			name:             "creds_only_agy",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--creds-only", "--force", "--json"},
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  false,
			wantJSONMode:     "creds-only",
			wantRemovedCount: 1,
		},
		{
			name:             "busy_claude_refused_even_with_force",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			claudeProcs:      []migration.ClaudeProcess{{PID: 42, ConfigDir: "__ACCOUNT_DIR__", EnvKnown: true}},
			wantErr:          "Claude Code session is running",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "busy_claude_creds_only_refused",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--creds-only", "--force"},
			claudeProcs:      []migration.ClaudeProcess{{PID: 42, ConfigDir: "__ACCOUNT_DIR__", EnvKnown: true}},
			wantErr:          "Claude Code session is running",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "busy_agy_refused_even_with_force",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force"},
			agyProcs:         []migration.AgyProcess{{PID: 99, AppDataDir: profile.AgyAppDataDirName("a1"), ArgsKnown: true}},
			wantErr:          "agy session is running",
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_claude_error",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			claudeScanErr:    errors.New("sysctl boom"),
			wantErr:          "sysctl boom",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_claude_nil_lister",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			nilClaudeLister:  true,
			wantErr:          "no process lister",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_claude_unknown_env",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			claudeProcs:      []migration.ClaudeProcess{{PID: 7, EnvKnown: false}},
			wantErr:          "Claude Code session is running",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_agy_error",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force"},
			agyScanErr:       errors.New("proc boom"),
			wantErr:          "proc boom",
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_agy_nil_lister",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force"},
			nilAgyLister:     true,
			wantErr:          "no process lister",
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  true,
		},
		{
			name:             "scan_failure_agy_unknown_args",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force"},
			agyProcs:         []migration.AgyProcess{{PID: 8, ArgsKnown: false}},
			wantErr:          "agy session is running",
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  true,
		},
		{
			name:             "no_tty_without_force_refused",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1"},
			isTTY:            false,
			wantErr:          "without a terminal",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "json_without_force_refused",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--json"},
			isTTY:            true,
			wantErr:          "--json requires --force",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "login_delete_failure_leaves_claude_folder",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			args:             []string{"account", "remove", "c1", "--force"},
			deleteErr:        errors.New("keychain locked"),
			wantErr:          "keychain locked",
			wantFolderExists: true,
			wantLoginExists:  true,
		},
		{
			name:             "login_delete_failure_leaves_agy_folder_and_data",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			args:             []string{"account", "remove", "a1", "--force"},
			deleteErr:        errors.New("permission denied"),
			wantErr:          "permission denied",
			wantFolderExists: true,
			wantDataExists:   true,
			wantLoginExists:  true,
		},
		{
			name:             "unreadable_binding_claude_still_removable",
			goos:             "darwin",
			harness:          profile.HarnessClaude,
			loggedIn:         true,
			corruptBinding:   true,
			args:             []string{"account", "remove", "c1", "--force"},
			wantFolderExists: false,
			wantLoginExists:  false,
		},
		{
			name:             "unreadable_binding_agy_still_removable",
			goos:             "darwin",
			harness:          profile.HarnessAgy,
			loggedIn:         true,
			corruptBinding:   true,
			args:             []string{"account", "remove", "a1", "--force", "--json"},
			wantFolderExists: false,
			wantDataExists:   false,
			wantLoginExists:  false,
			wantJSONMode:     "all",
			wantRemovedCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := accountCmdEnv(t, "work")
			store := &fakeCredStore{items: map[string]fakeCredItem{}, deleteErr: tc.deleteErr}
			var agyStore migration.CredentialStore
			if tc.harness == profile.HarnessAgy && tc.deleteErr != nil {
				agyStore = &fakeCredStore{deleteErr: tc.deleteErr}
			}

			var accountDir, loginKey, dataDir, tokenPath string
			if tc.harness == profile.HarnessAgy {
				accountDir, dataDir, tokenPath = setupAgyAccount(t, paths, "a1", "work", "team", tc.loggedIn)
			} else {
				accountDir, loginKey = setupClaudeAccount(t, paths, "c1", "work", "team", store, tc.goos, tc.loggedIn)
			}
			if tc.corruptBinding {
				if err := os.WriteFile(filepath.Join(accountDir, profile.AccountFile), []byte("not = [valid toml"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			claudeProcs := make([]migration.ClaudeProcess, len(tc.claudeProcs))
			copy(claudeProcs, tc.claudeProcs)
			for i := range claudeProcs {
				if claudeProcs[i].ConfigDir == "__ACCOUNT_DIR__" {
					claudeProcs[i].ConfigDir = accountDir
				}
			}

			var claudeLister migration.ProcessLister
			if !tc.nilClaudeLister {
				claudeLister = &fakeLister{procs: claudeProcs, err: tc.claudeScanErr}
			}
			var agyLister migration.AgyProcessLister
			if !tc.nilAgyLister {
				agyLister = &fakeAgyLister{procs: tc.agyProcs, err: tc.agyScanErr}
			}

			installAccountRemoveSeams(t, tc.goos, store, agyStore, claudeLister, agyLister, tc.isTTY)

			out, err := captureStdout(t, func() error { return runCcp(t, tc.args...) })
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			_, folderErr := os.Stat(accountDir)
			if got := folderErr == nil; got != tc.wantFolderExists {
				t.Errorf("accountDir exists = %v, want %v", got, tc.wantFolderExists)
			}

			if tc.harness == profile.HarnessAgy {
				_, dataErr := os.Stat(dataDir)
				if got := dataErr == nil; got != tc.wantDataExists {
					t.Errorf("dataDir exists = %v, want %v", got, tc.wantDataExists)
				}
				_, tokErr := os.Stat(tokenPath)
				if got := tokErr == nil; got != tc.wantLoginExists {
					t.Errorf("tokenPath exists = %v, want %v", got, tc.wantLoginExists)
				}
				if tc.wantDataExists {
					if _, convErr := os.Stat(filepath.Join(dataDir, "conversation.json")); convErr != nil {
						t.Errorf("conversation.json missing after --creds-only: %v", convErr)
					}
				}
			} else {
				_, exists := store.items[loginKey]
				if exists != tc.wantLoginExists {
					t.Errorf("claude login exists = %v, want %v", exists, tc.wantLoginExists)
				}
			}

			if tc.wantJSONMode != "" {
				var got accountRemoveJSON
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("invalid JSON %q: %v", out, err)
				}
				if got.Harness != tc.harness {
					t.Errorf("json harness = %q, want %q", got.Harness, tc.harness)
				}
				if got.Mode != tc.wantJSONMode {
					t.Errorf("json mode = %q, want %q", got.Mode, tc.wantJSONMode)
				}
				if len(got.Removed) != tc.wantRemovedCount {
					t.Errorf("json removed = %v (len %d), want len %d", got.Removed, len(got.Removed), tc.wantRemovedCount)
				}
			}
		})
	}
}

func TestAccountRemove_UnknownAndInvalidName(t *testing.T) {
	accountCmdEnv(t, "work")
	installAccountRemoveSeams(t, "darwin", &fakeCredStore{}, nil, &fakeLister{}, &fakeAgyLister{}, true)

	_, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "ghost", "--force") })
	if err == nil || !strings.Contains(err.Error(), "ccp account") {
		t.Fatalf("unknown account err = %v, want mention of 'ccp account'", err)
	}

	_, err = captureStdout(t, func() error { return runCcp(t, "account", "remove", "../escape", "--force") })
	if err == nil {
		t.Fatalf("expected validation error for invalid account name")
	}
}

func TestAccountRemove_CredsOnlyShowsNotLoggedIn(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	store := &fakeCredStore{items: map[string]fakeCredItem{}}

	setupClaudeAccount(t, paths, "c1", "work", "team", store, "darwin", true)
	setupAgyAccount(t, paths, "a1", "work", "team", true)
	installAccountRemoveSeams(t, "darwin", store, nil, &fakeLister{}, &fakeAgyLister{}, true)

	// Confirm both start logged in via accountusage.Reader.
	reader := &accountusage.Reader{Store: store, GOOS: "darwin", HomeDir: home}
	accounts, err := profile.ListAccounts(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(accounts))
	}

	// Remove creds only for both.
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "c1", "--creds-only", "--force") }); err != nil {
		t.Fatal(err)
	}
	accountRemoveCredsOnly = false
	accountRemoveForce = false
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "remove", "a1", "--creds-only", "--force") }); err != nil {
		t.Fatal(err)
	}

	// Both accounts still exist with their pool binding intact, and accountusage reports not logged in.
	accounts, err = profile.ListAccounts(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts after --creds-only = %d, want 2", len(accounts))
	}
	for _, a := range accounts {
		if a.Pool != "team" {
			t.Errorf("account %s pool = %q, want team", a.Name, a.Pool)
		}
	}
	statuses := reader.Read(accounts)
	for _, s := range statuses {
		if s.LoggedIn {
			t.Errorf("account %s still reported LoggedIn=true after --creds-only", s.Name)
		}
		if s.Note != "not logged in" {
			t.Errorf("account %s Note = %q, want 'not logged in'", s.Name, s.Note)
		}
	}
}

func TestAccountRemove_InteractivePrompt(t *testing.T) {
	paths := accountCmdEnv(t, "work")
	store := &fakeCredStore{items: map[string]fakeCredItem{}}
	accountDir, loginKey := setupClaudeAccount(t, paths, "c1", "work", "", store, "darwin", true)
	installAccountRemoveSeams(t, "darwin", store, nil, &fakeLister{}, &fakeAgyLister{}, true)

	withStdin := func(input string, fn func() error) (string, error) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		origStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = origStdin })
		_, _ = w.WriteString(input)
		_ = w.Close()
		out, runErr := captureStdout(t, fn)
		os.Stdin = origStdin
		_ = r.Close()
		return out, runErr
	}

	// Answering "n" prints exact targets (Keychain service name + folder) and deletes nothing.
	out, err := withStdin("n\n", func() error { return runCcp(t, "account", "remove", "c1") })
	if err != nil {
		t.Fatalf("cancel err: %v", err)
	}
	if !strings.Contains(out, loginKey) || !strings.Contains(out, accountDir) {
		t.Errorf("prompt did not list exact targets (key %q, dir %q):\n%s", loginKey, accountDir, out)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("expected Cancelled in output:\n%s", out)
	}
	if _, err := os.Stat(accountDir); err != nil {
		t.Fatalf("accountDir deleted despite cancel: %v", err)
	}
	if _, ok := store.items[loginKey]; !ok {
		t.Fatalf("login deleted despite cancel")
	}

	// Answering "y" proceeds and deletes both.
	out, err = withStdin("y\n", func() error { return runCcp(t, "account", "remove", "c1") })
	if err != nil {
		t.Fatalf("confirm err: %v", err)
	}
	if !strings.Contains(out, "Removed account c1") {
		t.Errorf("unexpected confirm output:\n%s", out)
	}
	if _, err := os.Stat(accountDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accountDir still exists after confirm: %v", err)
	}
	if _, ok := store.items[loginKey]; ok {
		t.Fatalf("login still exists after confirm")
	}
}
