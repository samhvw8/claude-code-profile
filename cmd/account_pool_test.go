package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// fakeLister returns procs; with later set, every call after the first
// returns later instead (a session that started between scan and lock).
type fakeLister struct {
	procs, later []migration.ClaudeProcess
	err          error
	calls        atomic.Int32
	agy          []migration.AgyProcess
	agyErr       error
}

func (f *fakeLister) AgyProcesses() ([]migration.AgyProcess, error) { return f.agy, f.agyErr }

func (f *fakeLister) ClaudeProcesses() ([]migration.ClaudeProcess, error) {
	if f.calls.Add(1) > 1 && f.later != nil {
		return f.later, f.err
	}
	return f.procs, f.err
}

// installFakeLister replaces the process lister; nil stands for a system
// without one.
func installFakeLister(t *testing.T, l migration.ProcessLister) {
	t.Helper()
	orig := newProcessLister
	if l == nil {
		newProcessLister = func() migration.ProcessLister { return nil }
	} else {
		newProcessLister = func() migration.ProcessLister { return l }
	}
	t.Cleanup(func() { newProcessLister = orig })
}

// installFakeUsage makes every account logged in, with the given 5-hour use
// (accounts not named have no usage data); read records which were asked.
func installFakeUsage(t *testing.T, five map[string]float64, read *[]string) {
	t.Helper()
	orig := readAccountUsage
	readAccountUsage = func(accts []profile.Account) ([]accountusage.Status, error) {
		out := make([]accountusage.Status, len(accts))
		for i, a := range accts {
			if read != nil {
				*read = append(*read, a.Name)
			}
			s := accountusage.Status{Name: a.Name, Profile: a.Profile, Harness: a.Harness, Dir: a.Dir, Pool: a.Pool, LoggedIn: true, Live: true}
			if v, ok := five[a.Name]; ok {
				s.Usage = &accountusage.Snapshot{FetchedAt: time.Now(), FiveHour: &accountusage.Window{Utilization: v}}
			}
			out[i] = s
		}
		return out, nil
	}
	t.Cleanup(func() { readAccountUsage = orig })
}

// stderrOf runs fn and returns what it printed to os.Stderr.
func stderrOf(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = stderr })
	done := make(chan string)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	runErr := fn()
	os.Stderr = stderr
	w.Close()
	return <-done, runErr
}

// poolEnv: profiles a and b; c1 in pool p on a, c2 in pool p on b, c3 on b
// with no pool.
func poolEnv(t *testing.T) (paths *config.Paths, out string) {
	t.Helper()
	p := accountCmdEnv(t, "a", "b")
	for _, acc := range [][3]string{{"c1", "a", "p"}, {"c2", "b", "p"}, {"c3", "b", ""}} {
		if _, _, err := profile.LinkAccount(p, acc[0], acc[1], false); err != nil {
			t.Fatal(err)
		}
		if acc[2] != "" {
			if err := profile.SetAccountPool(p.AccountDir(acc[0]), acc[2]); err != nil {
				t.Fatal(err)
			}
		}
	}
	return p, filepath.Join(t.TempDir(), "dir")
}

// runPooled runs 'ccp run <args> -- sh -c' recording CLAUDE_CONFIG_DIR, and
// returns that folder and stderr.
func runPooled(t *testing.T, out string, args ...string) (dir, stderr string, err error) {
	t.Helper()
	os.Remove(out)
	full := append(append([]string{"run"}, args...), "--", "sh", "-c", "printf %s \"$CLAUDE_CONFIG_DIR\" > "+out)
	stderr, err = stderrOf(t, func() error { return runCcp(t, full...) })
	data, _ := os.ReadFile(out)
	return string(data), stderr, err
}

func boundTo(t *testing.T, paths *config.Paths, name string) (string, string) {
	t.Helper()
	a := profile.ReadAccount(paths, name)
	if a.Err != nil {
		t.Fatalf("%s: %v", name, a.Err)
	}
	return a.Profile, a.Pool
}

func TestParseRunArgs_AutoPool(t *testing.T) {
	ra, err := parseRunArgs([]string{"--account", "auto:company", "--or-profile"})
	if err != nil || ra.account != "auto:company" || !ra.orProfile {
		t.Errorf("auto:company --or-profile: %+v %v", ra, err)
	}
	if _, err := parseRunArgs([]string{"--account", "auto:", "--", "claude"}); err == nil {
		t.Error("auto: with no pool accepted")
	}
	if _, err := parseRunArgs([]string{"--account", "auto:a/b"}); err == nil {
		t.Error("invalid pool accepted")
	}
}

// A pooled run picks across profiles and relinks a free account from
// another profile, keeping its pool, and says so.
func TestRunPool_PicksAcrossProfilesAndRelinks(t *testing.T) {
	paths, out := poolEnv(t)
	var read []string
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10, "c3": 0}, &read)
	dir, stderr, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil {
		t.Fatal(err)
	}
	if dir != paths.AccountDir("c2") {
		t.Errorf("ran in %q, want c2", dir)
	}
	if prof, pool := boundTo(t, paths, "c2"); prof != "a" || pool != "p" {
		t.Errorf("c2 after pick: %q %q", prof, pool)
	}
	if !strings.Contains(stderr, "ccp: account c2 (pool p, relinked from b, 5h 10%)") {
		t.Errorf("pick line: %q", stderr)
	}
	if strings.Contains(strings.Join(read, ","), "c3") {
		t.Errorf("an account outside the pool was read: %v", read)
	}
	// Same profile now: no relink in the message.
	_, stderr, err = runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || !strings.Contains(stderr, "ccp: account c2 (pool p, 5h 10%)") {
		t.Errorf("second pick: %v %q", err, stderr)
	}
}

// A cross-profile member with a Claude Code process on its folder is
// skipped before the usage read and keeps its binding.
func TestRunPool_SkipsBusyCrossProfile(t *testing.T) {
	paths, out := poolEnv(t)
	var read []string
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, &read)
	installFakeLister(t, &fakeLister{procs: []migration.ClaudeProcess{{PID: 7, ConfigDir: paths.AccountDir("c2") + "/", EnvKnown: true}}})
	dir, _, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || dir != paths.AccountDir("c1") {
		t.Fatalf("ran in %q (%v), want c1", dir, err)
	}
	if prof, _ := boundTo(t, paths, "c2"); prof != "b" {
		t.Errorf("busy c2 was relinked to %q", prof)
	}
	if strings.Join(read, ",") != "c1" {
		t.Errorf("read %v, want only c1", read)
	}
}

// Fail closed: an unreadable process environment, a failed scan, or no
// lister at all make every cross-profile member busy.
func TestRunPool_FailsClosed(t *testing.T) {
	for name, lister := range map[string]migration.ProcessLister{
		"env unknown": &fakeLister{procs: []migration.ClaudeProcess{{PID: 9}}},
		"scan error":  &fakeLister{err: errors.New("boom")},
		"no lister":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			paths, out := poolEnv(t)
			installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, nil)
			installFakeLister(t, lister)
			dir, _, err := runPooled(t, out, "a", "--account", "auto:p")
			if err != nil || dir != paths.AccountDir("c1") {
				t.Fatalf("ran in %q (%v), want c1", dir, err)
			}
			if prof, _ := boundTo(t, paths, "c2"); prof != "b" {
				t.Errorf("c2 relinked to %q", prof)
			}
			// Asked from b, c1 (on a) is the only other member: busy, so
			// only c2 remains, which is on b already.
			dir, _, err = runPooled(t, out, "b", "--account", "auto:p")
			if err != nil || dir != paths.AccountDir("c2") {
				t.Errorf("from b: ran in %q (%v), want c2", dir, err)
			}
		})
	}
}

// A member linked to the requested profile stays usable while a session
// runs on it (parallel sessions on one account and profile).
func TestRunPool_SameProfileUsableWhileRunning(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, nil)
	installFakeLister(t, &fakeLister{procs: []migration.ClaudeProcess{
		{PID: 1, ConfigDir: paths.AccountDir("c1"), EnvKnown: true},
		{PID: 2, ConfigDir: paths.AccountDir("c2"), EnvKnown: true},
	}})
	dir, _, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || dir != paths.AccountDir("c1") {
		t.Fatalf("ran in %q (%v), want c1", dir, err)
	}
}

// A session that starts on the pick between the scan and the lock makes the
// locked re-check fail; the next candidate is taken.
func TestRunPool_RecheckUnderLockFallsThrough(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, nil)
	installFakeLister(t, &fakeLister{later: []migration.ClaudeProcess{{PID: 3, ConfigDir: paths.AccountDir("c2"), EnvKnown: true}}})
	dir, _, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || dir != paths.AccountDir("c1") {
		t.Fatalf("ran in %q (%v), want c1", dir, err)
	}
	if prof, _ := boundTo(t, paths, "c2"); prof != "b" {
		t.Errorf("c2 relinked to %q", prof)
	}
}

// Every member busy on another profile: an error that --or-profile turns
// into a run on the profile.
func TestRunPool_AllBusyOrProfile(t *testing.T) {
	paths, out := poolEnv(t)
	if err := profile.SetAccountPool(paths.AccountDir("c1"), ""); err != nil {
		t.Fatal(err)
	}
	installFakeUsage(t, map[string]float64{"c2": 10}, nil)
	installFakeLister(t, &fakeLister{procs: []migration.ClaudeProcess{{PID: 2, ConfigDir: paths.AccountDir("c2"), EnvKnown: true}}})
	if _, _, err := runPooled(t, out, "a", "--account", "auto:p"); err == nil || !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), "linked to another profile") {
		t.Errorf("all busy: %v", err)
	}
	dir, stderr, err := runPooled(t, out, "a", "--account", "auto:p", "--or-profile")
	if err != nil || dir != paths.ProfileDir("a") || !strings.Contains(stderr, "no account of pool p is free") {
		t.Errorf("--or-profile: %q %v %q", dir, err, stderr)
	}
	if _, _, err := runPooled(t, out, "a", "--account", "auto:empty", "--or-profile"); err != nil {
		t.Errorf("--or-profile on an empty pool: %v", err)
	}
}

// env and session never relink: only pool members already on the profile.
func TestEnvAndSession_PoolNeverRelinks(t *testing.T) {
	paths, _ := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, nil)
	t.Cleanup(func() {
		envAccount, envFormat = "", "shell"
		envCmd.Flags().Lookup("account").Changed = false
		sessionAccount = ""
		sessionCmd.Flags().Lookup("account").Changed = false
	})
	out, err := captureStdout(t, func() error { return runCcp(t, "env", "a", "--account", "auto:p") })
	if err != nil || !strings.Contains(out, paths.AccountDir("c1")) {
		t.Errorf("env auto:p from a: %v %q", err, out)
	}
	if prof, _ := boundTo(t, paths, "c2"); prof != "b" {
		t.Errorf("env relinked c2 to %q", prof)
	}
	if err := profile.SetAccountPool(paths.AccountDir("c1"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return runCcp(t, "env", "a", "--account", "auto:p") }); err == nil || !strings.Contains(err.Error(), "never relink") {
		t.Errorf("env with no member on a: %v", err)
	}
	if err := runCcp(t, "env", "a", "--account", "auto:p", "--format", "mise"); err == nil || !strings.Contains(err.Error(), "picks once") {
		t.Errorf("env auto:p into mise: %v", err)
	}
	t.Setenv("SHELL", "/usr/bin/true")
	if _, err := captureStdout(t, func() error { return runCcp(t, "session", "a", "--account", "auto:p") }); err == nil || !strings.Contains(err.Error(), "never relink") {
		t.Errorf("session with no member on a: %v", err)
	}
	if prof, _ := boundTo(t, paths, "c2"); prof != "b" {
		t.Errorf("session relinked c2 to %q", prof)
	}
}

// An agy account runs on any profile without rewriting its binding, named
// or picked from a pool; --relink still rebinds.
func TestRun_AgyAnyProfileKeepsBinding(t *testing.T) {
	paths := accountCmdEnv(t, "work", "other")
	binDir := filepath.Join(t.TempDir(), "bin")
	record := filepath.Join(t.TempDir(), "ran")
	installFakeAgy(t, binDir, record)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	if _, err := profile.CreateAgyAccount(paths, "agy1", "work"); err != nil {
		t.Fatal(err)
	}
	if err := profile.SetAccountPool(paths.AccountDir("agy1"), "gp"); err != nil {
		t.Fatal(err)
	}
	installFakeUsage(t, map[string]float64{"agy1": 5}, nil)
	for _, args := range [][]string{
		{"run", "other", "--account", "agy1"},
		{"run", "other", "--account", "auto:gp", "--", "agy"},
	} {
		os.Remove(record)
		if _, err := stderrOf(t, func() error { return runCcp(t, args...) }); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		got, _ := os.ReadFile(record)
		if !strings.Contains(string(got), "CCD="+paths.ProfileDir("other")+"\n") {
			t.Errorf("%v ran:\n%s", args, got)
		}
		if prof, pool := boundTo(t, paths, "agy1"); prof != "work" || pool != "gp" {
			t.Errorf("%v rewrote the binding: %q %q", args, prof, pool)
		}
	}
	if err := runCcp(t, "run", "other", "--account", "agy1", "--relink"); err != nil {
		t.Fatal(err)
	}
	if prof, pool := boundTo(t, paths, "agy1"); prof != "other" || pool != "gp" {
		t.Errorf("--relink: %q %q", prof, pool)
	}
	// Its default profile gone, a named agy account runs on the current one.
	os.RemoveAll(paths.ProfileDir("other"))
	if _, err := activationProfile(paths, "", "agy1"); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("no current profile: %v", err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", paths.ProfileDir("work"))
	if got, err := activationProfile(must(config.ResolvePaths()), "", "agy1"); err != nil || got != "work" {
		t.Errorf("missing default profile: %q %v", got, err)
	}
}

func TestAccountPoolCommand(t *testing.T) {
	paths := accountCmdEnv(t, "dev")
	t.Cleanup(func() {
		accountPoolClear, accountAddPool, accountListJSON = false, "", false
		accountPoolCmd.Flags().Lookup("clear").Changed = false
		accountAddCmd.Flags().Lookup("pool").Changed = false
	})
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "w1", "dev", "--pool", "company") }); err != nil {
		t.Fatal(err)
	}
	accountAddPool = ""
	if _, pool := boundTo(t, paths, "w1"); pool != "company" {
		t.Errorf("add --pool: %q", pool)
	}
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "w2", "dev", "--pool", "auto") }); err == nil {
		t.Error("add --pool auto accepted")
	}
	accountAddPool = ""
	if _, err := os.Stat(paths.AccountDir("w2")); err == nil {
		t.Error("a refused --pool left a folder")
	}
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "pool", "w1", "other") }); err != nil {
		t.Fatal(err)
	}
	if prof, pool := boundTo(t, paths, "w1"); pool != "other" || prof != "dev" {
		t.Errorf("pool set: %q %q", prof, pool)
	}
	installFakeAccountReader(t, paths, &fakeAccountStore{}, nil)
	out, err := captureStdout(t, func() error { return runCcp(t, "account", "--json") })
	if err != nil || !strings.Contains(out, `"pool": "other"`) {
		t.Errorf("json: %v\n%s", err, out)
	}
	accountListJSON = false
	out, err = captureStdout(t, func() error { return runCcp(t, "account") })
	if err != nil || !strings.Contains(out, "POOL") || !strings.Contains(out, " other ") {
		t.Errorf("table: %v\n%s", err, out)
	}
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "pool", "w1", "--clear") }); err != nil {
		t.Fatal(err)
	}
	accountPoolClear = false
	if _, pool := boundTo(t, paths, "w1"); pool != "" {
		t.Errorf("clear: %q", pool)
	}
	out, _ = captureStdout(t, func() error { return runCcp(t, "account", "--json") })
	accountListJSON = false
	if strings.Contains(out, `"pool"`) {
		t.Errorf("json has an empty pool:\n%s", out)
	}
	if err := runCcp(t, "account", "pool", "w1"); err == nil {
		t.Error("pool with neither a name nor --clear accepted")
	}
	if err := runCcp(t, "account", "pool", "nope", "x"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unknown account: %v", err)
	}
}

// A lock failure on one member is a lost candidate: the next is tried, and
// with every member lost the error is errNoUsableAccount (so --or-profile
// falls back).
func TestRunPool_LockFailureTriesNext(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 50, "c2": 10}, nil)
	orig := lockAccount
	failFor := map[string]bool{paths.AccountDir("c2"): true}
	lockAccount = func(dir string) (func(), error) {
		if failFor[filepath.Clean(dir)] {
			return nil, errors.New("injected lock failure")
		}
		return orig(dir)
	}
	t.Cleanup(func() { lockAccount = orig })
	dir, _, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || dir != paths.AccountDir("c1") {
		t.Fatalf("ran in %q (%v), want c1", dir, err)
	}
	failFor[paths.AccountDir("c1")] = true
	if _, _, err := runPooled(t, out, "a", "--account", "auto:p"); !errors.Is(err, errNoUsableAccount) || !strings.Contains(err.Error(), "injected lock failure") {
		t.Errorf("every lock failing: %v", err)
	}
	dir, _, err = runPooled(t, out, "a", "--account", "auto:p", "--or-profile")
	if err != nil || dir != paths.ProfileDir("a") {
		t.Errorf("--or-profile: %q %v", dir, err)
	}
}

// Pool errors name accounts whose binding cannot be read (their pool is
// unknown), instead of only saying the pool is empty.
func TestRunPool_ErrorNamesUnreadable(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, nil, nil)
	for _, n := range []string{"c1", "c2"} {
		os.WriteFile(filepath.Join(paths.AccountDir(n), profile.AccountFile), []byte("profile = \"a\"\npool = \"bad pool\"\n"), 0o644)
	}
	_, _, err := runPooled(t, out, "a", "--account", "auto:p")
	if err == nil || !strings.Contains(err.Error(), "c1") || !strings.Contains(err.Error(), "c2") || !errors.Is(err, errNoUsableAccount) {
		t.Errorf("err = %v", err)
	}
	t.Cleanup(func() { envAccount = ""; envCmd.Flags().Lookup("account").Changed = false })
	if _, err := captureStdout(t, func() error { return runCcp(t, "env", "a", "--account", "auto:p") }); err == nil || !strings.Contains(err.Error(), "c1") {
		t.Errorf("env err = %v", err)
	}
	// 'ccp account pool --clear' repairs it.
	t.Cleanup(func() { accountPoolClear = false; accountPoolCmd.Flags().Lookup("clear").Changed = false })
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "pool", "c1", "--clear") }); err != nil {
		t.Fatalf("pool --clear on an invalid pool: %v", err)
	}
	if a := profile.ReadAccount(paths, "c1"); a.Err != nil {
		t.Errorf("not repaired: %v", a.Err)
	}
}

// A relative CLAUDE_CONFIG_DIR in another process cannot be compared with
// ccp's own working directory: it counts as busy.
func TestAccountBusy_RelativeConfigDir(t *testing.T) {
	dir := t.TempDir()
	if !accountBusy([]migration.ClaudeProcess{{PID: 1, ConfigDir: "accounts/x", EnvKnown: true}}, nil, dir) {
		t.Error("relative CLAUDE_CONFIG_DIR not busy")
	}
	if accountBusy([]migration.ClaudeProcess{{PID: 1, ConfigDir: "/elsewhere", EnvKnown: true}, {PID: 2, EnvKnown: true}}, nil, dir) {
		t.Error("an absolute other folder or an unset one made it busy")
	}
}

// After its default profile is deleted, an agy account is not a doctor
// problem (it runs on the current profile); doctor notes it instead.
func TestCheckAccounts_AgyMissingDefaultIsANote(t *testing.T) {
	paths := accountCmdEnv(t, "a", "gone")
	if _, err := profile.CreateAgyAccount(paths, "agy1", "gone"); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(paths.ProfileDir("gone"))
	if problems := checkAccounts(paths); len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	if notes := strings.Join(accountNotes(paths), "\n"); !strings.Contains(notes, "agy1") || !strings.Contains(notes, "current profile") {
		t.Errorf("notes: %q", notes)
	}
}

// 'ccp account pool <name> ""' is an empty pool name, not a hidden --clear.
func TestAccountPoolCommand_EmptyNameRefused(t *testing.T) {
	paths := accountCmdEnv(t, "dev")
	t.Cleanup(func() { accountAddPool = ""; accountAddCmd.Flags().Lookup("pool").Changed = false })
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "w1", "dev", "--pool", "company") }); err != nil {
		t.Fatal(err)
	}
	accountAddPool = ""
	if _, err := captureStdout(t, func() error { return runCcp(t, "account", "pool", "w1", "") }); err == nil {
		t.Error("empty pool name accepted")
	}
	if _, pool := boundTo(t, paths, "w1"); pool != "company" {
		t.Errorf("pool after a refused empty name: %q", pool)
	}
}

// A cross-profile pick links the account once: each warning (a real file
// where the profile has an entry) is printed once, not by the claim and again
// by the activation.
func TestRunPool_RelinkWarnsOnce(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 90, "c2": 10}, nil)
	if err := os.WriteFile(filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(paths.AccountDir("c2"), "CLAUDE.md")
	os.Remove(real)
	if err := os.WriteFile(real, []byte("own"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, stderr, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil || dir != paths.AccountDir("c2") {
		t.Fatalf("run: %v %q\n%s", err, dir, stderr)
	}
	if n := strings.Count(stderr, "CLAUDE.md is a real file"); n != 1 {
		t.Errorf("real-file warning printed %d times:\n%s", n, stderr)
	}
}

// --relink with auto:<pool> is a usage error; it does not list unreadable
// bindings, which have nothing to do with it.
func TestRunPool_RelinkErrorIsPlain(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, nil, nil)
	os.WriteFile(filepath.Join(paths.AccountDir("c3"), profile.AccountFile), []byte("profile = \"a\"\npool = \"bad pool\"\n"), 0o644)
	_, _, err := runPooled(t, out, "a", "--account", "auto:p", "--relink")
	if err == nil || !strings.Contains(err.Error(), "--relink needs a named account") || strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("err = %v", err)
	}
	_, _, err = runPooled(t, out, "a", "--account", "auto:p", "--relink", "--", "agy")
	if err == nil || !strings.Contains(err.Error(), "--relink needs a named account") || strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("agy err = %v", err)
	}
}

// A binding whose only fault is its pool gets the pool --clear repair in
// doctor, not --relink.
func TestCheckAccounts_InvalidPoolHint(t *testing.T) {
	paths, _ := poolEnv(t)
	os.WriteFile(filepath.Join(paths.AccountDir("c1"), profile.AccountFile), []byte("profile = \"a\"\npool = \"bad pool\"\n"), 0o644)
	got := strings.Join(checkAccounts(paths), "\n")
	if !strings.Contains(got, "ccp account pool c1 --clear") || strings.Contains(got, "c1 --relink") {
		t.Errorf("doctor:\n%s", got)
	}
}

// 'account add --pool' says the account was created before naming its pool.
func TestAccountAdd_PoolLineAfterCreated(t *testing.T) {
	accountCmdEnv(t, "dev")
	t.Cleanup(func() { accountAddPool = ""; accountAddCmd.Flags().Lookup("pool").Changed = false })
	out, err := captureStdout(t, func() error { return runCcp(t, "account", "add", "w1", "dev", "--pool", "company") })
	if err != nil {
		t.Fatal(err)
	}
	c, p := strings.Index(out, "created"), strings.Index(out, "In pool company")
	if c < 0 || p < 0 || p < c {
		t.Errorf("order:\n%s", out)
	}
}

// A one-shot warning from the relink (a hand-made link in place of the
// account's own .claude.json, removed by the link) is printed, once.
func TestRunPool_RelinkOneShotWarning(t *testing.T) {
	paths, out := poolEnv(t)
	installFakeUsage(t, map[string]float64{"c1": 90, "c2": 10}, nil)
	state := filepath.Join(paths.AccountDir("c2"), ".claude.json")
	os.Remove(state)
	if err := os.Symlink(filepath.Join(t.TempDir(), "elsewhere.json"), state); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runPooled(t, out, "a", "--account", "auto:p")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, stderr)
	}
	if n := strings.Count(stderr, "removed link .claude.json"); n != 1 {
		t.Errorf("removed-link warning printed %d times:\n%s", n, stderr)
	}
}
