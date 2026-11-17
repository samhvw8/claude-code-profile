package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

type fakeCred struct {
	secret []byte
	acct   string
}

// fakeStore is an in-memory credential store that records every call.
type fakeStore struct {
	items      map[string]fakeCred
	ops        []string
	corruptSet bool   // Set stores different bytes (verify must fail)
	failDelete string // Delete of this key fails
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string]fakeCred{}} }

func (s *fakeStore) Get(key string) ([]byte, string, bool, error) {
	s.ops = append(s.ops, "get "+key)
	c, ok := s.items[key]
	return c.secret, c.acct, ok, nil
}

func (s *fakeStore) Set(key, acct string, secret []byte) error {
	s.ops = append(s.ops, "set "+key)
	if s.corruptSet {
		secret = append(slices.Clone(secret), 'X')
	}
	s.items[key] = fakeCred{secret: slices.Clone(secret), acct: acct}
	return nil
}

func (s *fakeStore) Delete(key string) error {
	s.ops = append(s.ops, "delete "+key)
	if key == s.failDelete {
		return errors.New("boom")
	}
	delete(s.items, key)
	return nil
}

// mutations returns the calls that change the store, in order.
func (s *fakeStore) mutations() []string {
	var out []string
	for _, op := range s.ops {
		if !strings.HasPrefix(op, "get ") {
			out = append(out, op)
		}
	}
	return out
}

type fakeProcs []ClaudeProcess

func (f fakeProcs) ClaudeProcesses() ([]ClaudeProcess, error) { return f, nil }

type migrateEnv struct {
	paths *config.Paths
	store *fakeStore
	home  string
}

// newMigrateEnv builds a ccp tree with the given profiles and a fake home.
func newMigrateEnv(t *testing.T, profiles ...string) *migrateEnv {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	ccp := filepath.Join(root, "ccp")
	paths := &config.Paths{
		CcpDir:          ccp,
		ClaudeDir:       filepath.Join(home, ".claude"),
		GlobalClaudeDir: filepath.Join(home, ".claude"),
		HubDir:          filepath.Join(ccp, "hub"),
		ProfilesDir:     filepath.Join(ccp, "profiles"),
		SharedDir:       filepath.Join(ccp, "profiles", "shared"),
		StoreDir:        filepath.Join(ccp, "store"),
	}
	os.MkdirAll(home, 0755)
	for _, it := range config.AllHubItemTypes() {
		os.MkdirAll(paths.HubItemDir(it), 0755)
	}
	mgr := profile.NewManager(paths)
	for _, p := range profiles {
		if _, err := mgr.Create(p, profile.NewManifest(p, "")); err != nil {
			t.Fatalf("Create %s: %v", p, err)
		}
	}
	return &migrateEnv{paths: paths, store: newFakeStore(), home: home}
}

func (e *migrateEnv) migrator(procs ...ClaudeProcess) *AccountMigrator {
	return NewAccountMigratorWith(e.paths, "darwin", e.store, fakeProcs(procs))
}

func (e *migrateEnv) makeGlobal(t *testing.T, name string) {
	t.Helper()
	if err := os.Symlink(e.paths.ProfileDir(name), e.paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
}

func (e *migrateEnv) globalState() string { return filepath.Join(e.home, ".claude.json") }

func (e *migrateEnv) envService(name string) string {
	return KeychainServiceFor(e.paths.ProfileDir(name), false)
}

func (e *migrateEnv) accountService(name string) string {
	return KeychainServiceFor(e.paths.AccountDir(name), false)
}

func writeJSONFixture(t *testing.T, path string, v any, mode os.FileMode) {
	t.Helper()
	data, _ := json.Marshal(v)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, mode)
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func only(t *testing.T, rs []AccountMigrationResult) AccountMigrationResult {
	t.Helper()
	if len(rs) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(rs), rs)
	}
	return rs[0]
}

func TestConfigDirHash(t *testing.T) {
	if got := ConfigDirHash("/Users/samhv/.ccp/profiles/companyA"); got != "c08e605e" {
		t.Errorf("ConfigDirHash = %q, want c08e605e", got)
	}
	if got := KeychainServiceFor("/Users/samhv/.ccp/profiles/companyA", false); got != "Claude Code-credentials-c08e605e" {
		t.Errorf("env service = %q", got)
	}
	if got := KeychainServiceFor("/anything", true); got != "Claude Code-credentials" {
		t.Errorf("global service = %q", got)
	}
}

func TestAccountMigrator_SourceChoice(t *testing.T) {
	tests := []struct {
		name       string
		global     bool // ~/.claude -> profile
		globalCred bool
		envCred    bool
		wantStatus string
		wantGlobal bool
		wantWarn   bool
	}{
		{"global only", true, true, false, AccountWouldMove, true, false},
		{"env only", false, false, true, AccountWouldMove, false, false},
		{"env only while ~/.claude is the profile", true, false, true, AccountWouldMove, false, false},
		{"both: global wins, env left", true, true, true, AccountWouldMove, true, true},
		{"global login of another profile is not ours", false, true, false, AccountSkipped, false, false},
		{"none", false, false, false, AccountSkipped, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newMigrateEnv(t, "p")
			if tt.global {
				e.makeGlobal(t, "p")
			}
			if tt.globalCred {
				e.store.items[keychainService] = fakeCred{secret: []byte("g")}
			}
			if tt.envCred {
				e.store.items[e.envService("p")] = fakeCred{secret: []byte("e")}
			}
			r := only(t, must(e.migrator().Migrate(true)))
			if r.Status != tt.wantStatus || r.Global != tt.wantGlobal || (len(r.Warnings) > 0) != tt.wantWarn {
				t.Errorf("got %+v; want status %s global %v warn %v", r, tt.wantStatus, tt.wantGlobal, tt.wantWarn)
			}
			if tt.wantStatus == AccountSkipped && r.Reason != "no login" {
				t.Errorf("reason = %q, want no login", r.Reason)
			}
		})
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestAccountMigrator_MovesGlobalLogin(t *testing.T) {
	e := newMigrateEnv(t, "default", "other")
	e.makeGlobal(t, "default")
	e.store.items[keychainService] = fakeCred{secret: []byte(`{"claudeAiOauth":{"accessToken":"t"}}`), acct: "sam"}
	writeJSONFixture(t, e.globalState(), map[string]any{"oauthAccount": map[string]any{"emailAddress": "me@corp"}, "numStartups": 4}, 0644)

	rs := must(e.migrator(ClaudeProcess{PID: 7, ConfigDir: "/elsewhere", EnvKnown: true}).Migrate(false))
	var r AccountMigrationResult
	for _, x := range rs {
		if x.Profile == "default" {
			r = x
		} else if x.Status != AccountSkipped {
			t.Errorf("other profile: %+v", x)
		}
	}
	if r.Status != AccountMoved || !r.Global {
		t.Fatalf("result %+v", r)
	}

	target := e.accountService("default")
	wantOps := []string{"set " + target, "delete " + keychainService}
	if got := e.store.mutations(); !slices.Equal(got, wantOps) {
		t.Errorf("mutations = %v, want %v", got, wantOps)
	}
	// The read-back comes between the write and the delete.
	setAt := slices.Index(e.store.ops, "set "+target)
	if setAt < 0 || e.store.ops[setAt+1] != "get "+target {
		t.Errorf("no verify read after the write: %v", e.store.ops)
	}
	if c := e.store.items[target]; string(c.secret) != `{"claudeAiOauth":{"accessToken":"t"}}` || c.acct != "sam" {
		t.Errorf("target = %+v", c)
	}
	if _, ok := e.store.items[keychainService]; ok {
		t.Error("source credential kept")
	}

	acctState := filepath.Join(e.paths.AccountDir("default"), ".claude.json")
	got := readJSONMap(t, acctState)
	if got["oauthAccount"].(map[string]any)["emailAddress"] != "me@corp" || got["numStartups"] != float64(4) {
		t.Errorf("account state = %v", got)
	}
	if info, _ := os.Stat(acctState); info.Mode().Perm() != 0600 {
		t.Errorf("account state mode %v", info.Mode().Perm())
	}
	src := readJSONMap(t, e.globalState())
	if _, ok := src["oauthAccount"]; ok || src["numStartups"] != float64(4) {
		t.Errorf("source state = %v", src)
	}
	if info, _ := os.Stat(e.globalState()); info.Mode().Perm() != 0644 {
		t.Errorf("source state mode changed to %v", info.Mode().Perm())
	}
	if bound, _ := profile.AccountProfile(e.paths.AccountDir("default")); bound != "default" {
		t.Errorf("account bound to %q", bound)
	}

	// Idempotent: the second run skips.
	again := must(e.migrator().Migrate(false))
	for _, x := range again {
		if x.Profile == "default" && (x.Status != AccountSkipped || x.Reason != "no login") {
			t.Errorf("second run: %+v", x)
		}
	}
}

func TestAccountMigrator_MovesEnvLogin(t *testing.T) {
	e := newMigrateEnv(t, "work")
	src := e.envService("work")
	e.store.items[src] = fakeCred{secret: []byte("s"), acct: "a"}
	profState := filepath.Join(e.paths.ProfileDir("work"), ".claude.json")
	writeJSONFixture(t, profState, map[string]any{"oauthAccount": map[string]any{"emailAddress": "w@corp"}, "k": 1}, 0600)

	r := only(t, must(e.migrator().Migrate(false)))
	if r.Status != AccountMoved || r.Global {
		t.Fatalf("result %+v", r)
	}
	if _, ok := e.store.items[e.accountService("work")]; !ok {
		t.Error("target missing")
	}
	if _, ok := e.store.items[src]; ok {
		t.Error("source kept")
	}
	if _, ok := readJSONMap(t, profState)["oauthAccount"]; ok {
		t.Error("oauthAccount left in the profile state")
	}
	if readJSONMap(t, filepath.Join(e.paths.AccountDir("work"), ".claude.json"))["oauthAccount"] == nil {
		t.Error("oauthAccount not copied")
	}
}

func TestAccountMigrator_AlreadyMigrated(t *testing.T) {
	e := newMigrateEnv(t, "p")
	e.store.items[e.envService("p")] = fakeCred{secret: []byte("old")}
	e.store.items[e.accountService("p")] = fakeCred{secret: []byte("new")}
	r := only(t, must(e.migrator().Migrate(false)))
	if r.Status != AccountSkipped || r.Reason != "already migrated" {
		t.Errorf("result %+v", r)
	}
	if len(e.store.mutations()) != 0 {
		t.Errorf("store changed: %v", e.store.mutations())
	}
}

func TestAccountMigrator_AccountBoundElsewhere(t *testing.T) {
	e := newMigrateEnv(t, "p", "q")
	e.store.items[e.envService("p")] = fakeCred{secret: []byte("s")}
	if _, _, err := profile.LinkAccount(e.paths, "p", "q", false); err != nil {
		t.Fatal(err)
	}
	for _, r := range must(e.migrator().Migrate(false)) {
		if r.Profile == "p" && (r.Status != AccountRefused || !strings.Contains(r.Reason, `linked to profile "q"`)) {
			t.Errorf("result %+v", r)
		}
	}
	if len(e.store.mutations()) != 0 {
		t.Errorf("store changed: %v", e.store.mutations())
	}
}

func TestAccountMigrator_RefusesWhileClaudeRuns(t *testing.T) {
	tests := []struct {
		name    string
		global  bool
		procs   []ClaudeProcess
		refused bool
	}{
		{"global: claude without CLAUDE_CONFIG_DIR", true, []ClaudeProcess{{PID: 11, EnvKnown: true}}, true},
		{"global: CLAUDE_CONFIG_DIR=~/.claude", true, []ClaudeProcess{{PID: 12, ConfigDir: "HOME/.claude", EnvKnown: true}}, true},
		{"global: CLAUDE_CONFIG_DIR=profile", true, []ClaudeProcess{{PID: 13, ConfigDir: "PROFILE", EnvKnown: true}}, true},
		{"global: unreadable environment", true, []ClaudeProcess{{PID: 14}}, true},
		{"global: only other folders", true, []ClaudeProcess{{PID: 15, ConfigDir: "/x/other", EnvKnown: true}}, false},
		{"env: CLAUDE_CONFIG_DIR=profile", false, []ClaudeProcess{{PID: 21, ConfigDir: "PROFILE", EnvKnown: true}}, true},
		{"env: CLAUDE_CONFIG_DIR=profile/", false, []ClaudeProcess{{PID: 22, ConfigDir: "PROFILE/", EnvKnown: true}}, true},
		{"env: plain claude is unrelated", false, []ClaudeProcess{{PID: 23, EnvKnown: true}, {PID: 24, ConfigDir: "/x/other", EnvKnown: true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newMigrateEnv(t, "p")
			if tt.global {
				e.makeGlobal(t, "p")
				e.store.items[keychainService] = fakeCred{secret: []byte("s")}
			} else {
				e.store.items[e.envService("p")] = fakeCred{secret: []byte("s")}
			}
			var procs []ClaudeProcess
			for _, p := range tt.procs {
				p.ConfigDir = strings.NewReplacer("HOME", e.home, "PROFILE", e.paths.ProfileDir("p")).Replace(p.ConfigDir)
				procs = append(procs, p)
			}
			r := only(t, must(e.migrator(procs...).Migrate(false)))
			if tt.refused {
				if r.Status != AccountRefused {
					t.Fatalf("result %+v, want refused", r)
				}
				for _, p := range tt.procs {
					if !strings.Contains(r.Reason, fmt.Sprint(p.PID)) {
						t.Errorf("reason %q does not name PID %d", r.Reason, p.PID)
					}
				}
				if len(e.store.mutations()) != 0 {
					t.Errorf("refused profile changed the store: %v", e.store.mutations())
				}
				if _, err := os.Stat(e.paths.AccountDir("p")); err == nil {
					t.Error("refused profile created its account folder")
				}
			} else if r.Status != AccountMoved {
				t.Errorf("result %+v, want moved", r)
			}
		})
	}
}

func TestAccountMigrator_RollbackOnVerifyFailure(t *testing.T) {
	e := newMigrateEnv(t, "p")
	src := e.envService("p")
	e.store.items[src] = fakeCred{secret: []byte("s")}
	e.store.corruptSet = true
	profState := filepath.Join(e.paths.ProfileDir("p"), ".claude.json")
	writeJSONFixture(t, profState, map[string]any{"oauthAccount": map[string]any{"emailAddress": "x"}}, 0600)

	r := only(t, must(e.migrator().Migrate(false)))
	if r.Status != AccountFailed || !strings.Contains(r.Reason, "rolled back") {
		t.Fatalf("result %+v", r)
	}
	target := e.accountService("p")
	if want := []string{"set " + target, "delete " + target}; !slices.Equal(e.store.mutations(), want) {
		t.Errorf("mutations = %v, want %v", e.store.mutations(), want)
	}
	if _, ok := e.store.items[src]; !ok {
		t.Error("source removed despite failure")
	}
	if _, ok := readJSONMap(t, profState)["oauthAccount"]; !ok {
		t.Error("source oauthAccount removed despite failure")
	}
	if _, err := os.Stat(filepath.Join(e.paths.AccountDir("p"), ".claude.json")); err == nil {
		if _, ok := readJSONMap(t, filepath.Join(e.paths.AccountDir("p"), ".claude.json"))["oauthAccount"]; ok {
			t.Error("oauthAccount written to the account despite failure")
		}
	}
}

func TestAccountMigrator_RollbackOnSourceDeleteFailure(t *testing.T) {
	e := newMigrateEnv(t, "p")
	src := e.envService("p")
	e.store.items[src] = fakeCred{secret: []byte("s")}
	e.store.failDelete = src
	profState := filepath.Join(e.paths.ProfileDir("p"), ".claude.json")
	writeJSONFixture(t, profState, map[string]any{"oauthAccount": map[string]any{"emailAddress": "x"}, "k": 1}, 0600)

	r := only(t, must(e.migrator().Migrate(false)))
	if r.Status != AccountFailed {
		t.Fatalf("result %+v", r)
	}
	if _, ok := e.store.items[e.accountService("p")]; ok {
		t.Error("target credential kept after failed source delete")
	}
	acctState := readJSONMap(t, filepath.Join(e.paths.AccountDir("p"), ".claude.json"))
	if _, ok := acctState["oauthAccount"]; ok {
		t.Errorf("account state not restored: %v", acctState)
	}
	if acctState["k"] != float64(1) {
		t.Errorf("seeded state lost: %v", acctState)
	}
}

func TestAccountMigrator_DryRunChangesNothing(t *testing.T) {
	e := newMigrateEnv(t, "default")
	e.makeGlobal(t, "default")
	e.store.items[keychainService] = fakeCred{secret: []byte("s")}
	writeJSONFixture(t, e.globalState(), map[string]any{"oauthAccount": map[string]any{"emailAddress": "x"}}, 0600)
	before, _ := os.ReadFile(e.globalState())

	r := only(t, must(e.migrator().Migrate(true)))
	if r.Status != AccountWouldMove || !strings.Contains(r.Reason, e.accountService("default")) {
		t.Errorf("result %+v", r)
	}
	if len(e.store.mutations()) != 0 {
		t.Errorf("store changed: %v", e.store.mutations())
	}
	if _, err := os.Stat(e.paths.AccountsDir()); err == nil {
		t.Error("dry run created the accounts folder")
	}
	if after, _ := os.ReadFile(e.globalState()); string(after) != string(before) {
		t.Error("dry run changed ~/.claude.json")
	}
}

func TestAccountMigrator_LinuxFiles(t *testing.T) {
	e := newMigrateEnv(t, "default")
	e.makeGlobal(t, "default")
	srcFile := filepath.Join(e.paths.ProfileDir("default"), ".credentials.json")
	if err := os.WriteFile(srcFile, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewAccountMigratorWith(e.paths, "linux", FileStore{}, fakeProcs(nil))
	r := only(t, must(m.Migrate(false)))
	if r.Status != AccountMoved || !r.Global || len(r.Warnings) != 1 { // warning: no oauthAccount
		t.Fatalf("result %+v", r)
	}
	dst := filepath.Join(e.paths.AccountDir("default"), ".credentials.json")
	info, err := os.Lstat(dst)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
		t.Fatalf("target %v %v", info, err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "secret" {
		t.Errorf("target = %q", data)
	}
	if _, err := os.Stat(srcFile); err == nil {
		t.Error("source file kept")
	}
	if r := only(t, must(m.Migrate(false))); r.Status != AccountSkipped {
		t.Errorf("second run %+v", r)
	}
}

func TestKeychainStore(t *testing.T) {
	var calls [][]string
	var stdins []string
	attrs := "keychain: \"/k\"\nattributes:\n    \"acct\"<blob>=\"sam\"\n    \"svce\"<blob>=\"Claude Code-credentials\"\n"
	k := &KeychainStore{run: func(stdin, name string, args ...string) (string, string, int, error) {
		calls = append(calls, append([]string{name}, args...))
		stdins = append(stdins, stdin)
		switch {
		case slices.Contains(args, "missing"):
			return "", "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n", 44, nil
		case slices.Contains(args, "quoted"):
			return attrs, "password: \"{\"a\":\"b c\"}\"\n", 0, nil
		case slices.Contains(args, "hexed"):
			return strings.Replace(attrs, `"sam"`, `0x73616D  "sam"`, 1), "password: 0x00FF41  \"\\000\\377A\"\n", 0, nil
		case len(args) == 1 && args[0] == "-i" && strings.Contains(stdin, "bad"):
			return "", "add-generic-password: returned 45\n", 0, nil
		}
		return "", "", 0, nil
	}}

	if _, _, found, err := k.Get("missing"); found || err != nil {
		t.Errorf("missing: found %v err %v", found, err)
	}
	secret, acct, found, err := k.Get("quoted")
	if err != nil || !found || string(secret) != `{"a":"b c"}` || acct != "sam" {
		t.Errorf("quoted: %q %q %v %v", secret, acct, found, err)
	}
	secret, acct, _, err = k.Get("hexed")
	if err != nil || string(secret) != "\x00\xffA" || acct != "sam" {
		t.Errorf("hexed: %q %q %v", secret, acct, err)
	}

	calls, stdins = nil, nil
	if err := k.Set("Claude Code-credentials-c08e605e", "sam", []byte("top-secret")); err != nil {
		t.Fatal(err)
	}
	for _, a := range calls[0] {
		if strings.Contains(a, "top-secret") || strings.Contains(a, "746f702d736563726574") {
			t.Errorf("secret in argv: %v", calls[0])
		}
	}
	want := "add-generic-password -U -a \"sam\" -s \"Claude Code-credentials-c08e605e\" -X 746f702d736563726574\n"
	if stdins[0] != want || !slices.Equal(calls[0], []string{"security", "-i"}) {
		t.Errorf("stdin %q argv %v", stdins[0], calls[0])
	}
	if err := k.Set("bad", "sam", []byte("x")); err == nil {
		t.Error("a failed security -i command was not reported")
	}
	if err := k.Set("svc", "a\"b", []byte("x")); err == nil {
		t.Error("quote in account accepted")
	}
	if err := k.Delete("missing"); err != nil {
		t.Errorf("delete of missing item: %v", err)
	}
}

func TestPsLister(t *testing.T) {
	list := "  101 /Users/u/.local/share/mise/installs/claude/2.1.326/claude --add-dir x\n" +
		"  102 claude --resume abc\n" +
		"  103 /bin/zsh -lic claude\n" +
		"  104 claude-helper --x\n" +
		"  105 claude\n"
	env := map[string]string{
		"101": "/Users/u/.local/share/mise/installs/claude/2.1.326/claude --add-dir x HOME=/Users/u CLAUDE_CONFIG_DIR=/Users/u/My Profiles/p TERM=xterm",
		"102": "claude --resume abc HOME=/Users/u",
		"105": "claude",
	}
	l := psLister{run: func(_, name string, args ...string) (string, string, int, error) {
		if args[0] == "-Axww" {
			return list, "", 0, nil
		}
		return env[args[len(args)-1]] + "\n", "", 0, nil
	}}
	got, err := l.ClaudeProcesses()
	if err != nil {
		t.Fatal(err)
	}
	want := []ClaudeProcess{
		{PID: 101, ConfigDir: "/Users/u/My Profiles/p", EnvKnown: true},
		{PID: 102, EnvKnown: true},
		{PID: 105, EnvKnown: false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestProcLister(t *testing.T) {
	root := t.TempDir()
	add := func(pid, cmdline, environ string) {
		dir := filepath.Join(root, pid)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0644)
		if environ != "" {
			os.WriteFile(filepath.Join(dir, "environ"), []byte(environ), 0644)
		}
	}
	add("10", "/home/u/.local/share/mise/installs/claude/2.1.0/claude\x00--x\x00", "HOME=/home/u\x00CLAUDE_CONFIG_DIR=/home/u/.ccp/profiles/p\x00")
	add("11", "bash\x00-c\x00claude\x00", "HOME=/home/u\x00")
	add("12", "claude\x00", "")
	os.MkdirAll(filepath.Join(root, "self"), 0755)

	got, err := procLister{root: root}.ClaudeProcesses()
	if err != nil {
		t.Fatal(err)
	}
	want := []ClaudeProcess{{PID: 10, ConfigDir: "/home/u/.ccp/profiles/p", EnvKnown: true}, {PID: 12}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
