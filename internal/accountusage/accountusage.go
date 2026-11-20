// Package accountusage reads how much of its plan limits each Claude account
// has used, and picks the account with the most left (ccp run --account auto).
//
// Usage comes from GET /api/oauth/usage, the endpoint Claude Code's own /usage
// screen reads. It is undocumented, so every failure (429, network, a changed
// shape, an expired token) falls back: to the usage saved at the last good
// read, then to the account used least recently. ccp only reads logins; it
// never refreshes or writes one.
package accountusage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

const (
	// UsageFile in an account folder holds the usage of the last good read.
	UsageFile = ".ccp-usage.json"
	// LastUsedFile in an account folder is touched each time ccp starts it.
	LastUsedFile = ".ccp-last-used"
	// DefaultURL is the usage endpoint of Claude subscription logins.
	DefaultURL = "https://api.anthropic.com/api/oauth/usage"
	// FreshFor is how long a saved read is used without asking again.
	FreshFor = 5 * time.Minute

	defaultAgyTimeout = 30 * time.Second

	// An account at or above these is picked only when every other one is too.
	busyFiveHour = 80
	busySevenDay = 95
	maxBody      = 256 << 10
)

// Window is one usage limit: the percent used and when it resets.
type Window struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at,omitzero"`
}

// At returns the percent used at now: 0 once the window has reset.
func (w *Window) At(now time.Time) (float64, bool) {
	if w == nil {
		return 0, false
	}
	if !w.ResetsAt.IsZero() && !now.Before(w.ResetsAt) {
		return 0, true
	}
	return w.Utilization, true
}

// Snapshot is one read of an account's usage. A plan without a 5-hour
// window has FiveHour nil.
type Snapshot struct {
	FetchedAt time.Time `json:"fetched_at"`
	FiveHour  *Window   `json:"five_hour,omitempty"`
	SevenDay  *Window   `json:"seven_day,omitempty"`
	// Scoped are weekly limits of one model (such as Fable), shown in status
	// but not ranked: whether they matter depends on the model a session uses.
	Scoped []Scoped `json:"scoped,omitempty"`
}

// Scoped is a weekly limit of one model.
type Scoped struct {
	Name string `json:"name"`
	Window
}

// Status is what ccp knows about one account.
type Status struct {
	Name, Profile, Harness, Dir string
	Email                       string
	LoggedIn                    bool
	Usage                       *Snapshot // nil: never read
	Live                        bool      // Usage was read just now
	Note                        string    // why Usage is not live
	LastUsed                    time.Time // zero: never started by ccp
}

// Other login kinds shown read-only in `ccp account`.
const (
	OtherKindClaudeGlobal  = "claude global"
	OtherKindClaudeProfile = "claude profile"
	OtherKindAgyPlain      = "agy plain"
)

// OtherLogin is an unmanaged Claude Code or agy login outside ~/.ccp/accounts.
// It is shown read-only in `ccp account` and never enters --account auto pools.
type OtherLogin struct {
	Kind     string
	Where    string
	CacheKey string
	Harness  string
	Profile  string
	Email    string
	Usage    *Snapshot
	Live     bool
	Note     string
}

// AgyRunner runs `agy <args>` in dir with env and returns stdout.
type AgyRunner func(ctx context.Context, dir string, args, env []string) ([]byte, error)

// Reader reads account logins and their usage.
type Reader struct {
	Store      migration.CredentialStore
	Keychain   migration.KeychainInspector
	GOOS       string
	URL        string
	HTTP       *http.Client
	UserAgent  string
	Now        func() time.Time
	FreshFor   time.Duration
	RunAgy     AgyRunner
	HomeDir    string
	AgyTimeout time.Duration
	Env        []string
}

// NewReader returns a reader for this machine's credential store.
func NewReader(version string) (*Reader, error) {
	store, err := migration.NewCredentialStore(runtime.GOOS)
	if err != nil {
		return nil, err
	}
	var kc migration.KeychainInspector
	if inspector, ok := store.(migration.KeychainInspector); ok {
		kc = inspector
	}
	home, _ := os.UserHomeDir()
	return &Reader{
		Store:    store,
		Keychain: kc,
		GOOS:     runtime.GOOS,
		URL:      DefaultURL,
		HTTP: &http.Client{
			Timeout: 5 * time.Second,
			// The token must never follow a redirect to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		UserAgent:  "ccp/" + version,
		Now:        time.Now,
		FreshFor:   FreshFor,
		RunAgy:     defaultAgyRunner,
		HomeDir:    home,
		AgyTimeout: defaultAgyTimeout,
	}, nil
}

// Read returns the status of each account, in the given order. Accounts whose
// saved usage is older than FreshFor are read again, in parallel.
func (r *Reader) Read(accounts []profile.Account) []Status {
	out := make([]Status, len(accounts))
	var wg sync.WaitGroup
	for i, a := range accounts {
		out[i] = Status{Name: a.Name, Profile: a.Profile, Harness: a.Harness, Dir: a.Dir}
		wg.Add(1)
		go func(s *Status) {
			defer wg.Done()
			r.readOne(s)
		}(&out[i])
	}
	wg.Wait()
	return out
}

type usageTarget struct {
	usage **Snapshot
	live  *bool
	note  *string
}

func (r *Reader) readOne(s *Status) {
	if info, err := os.Stat(filepath.Join(s.Dir, LastUsedFile)); err == nil {
		s.LastUsed = info.ModTime()
	}
	t := usageTarget{usage: &s.Usage, live: &s.Live, note: &s.Note}
	cachePath := filepath.Join(s.Dir, UsageFile)
	if profile.DefaultHarness(s.Harness) == profile.HarnessAgy {
		saved := loadUsageFileAt(cachePath)
		s.Usage = saved.Last
		r.readAgy(s, saved, cachePath, t)
		return
	}
	email, loggedIn, _ := r.readClaudeLogin(migration.LoginKey(r.GOOS, s.Dir), s.Dir, cachePath, t)
	s.Email = email
	s.LoggedIn = loggedIn
}

func (r *Reader) readClaudeLogin(credKey, stateDir, cachePath string, t usageTarget) (email string, loggedIn bool, err error) {
	saved := loadUsageFileAt(cachePath)
	*t.usage = saved.Last
	email = accountEmail(stateDir)
	secret, _, found, err := r.Store.Get(credKey)
	if err != nil {
		*t.note = fmt.Sprintf("cannot read the login: %v", err)
		return email, false, err
	}
	login, ok := migration.ParseLogin(secret)
	if !found || !ok {
		*t.note = "not logged in"
		return email, false, nil
	}
	now := r.Now()
	if r.cachedUsageValid(t, saved, now) {
		return email, true, nil
	}
	if login.AccessToken == "" || login.ExpiresAt > 0 && now.UnixMilli() >= login.ExpiresAt {
		*t.note = "token expired; Claude Code renews it when it starts"
		return email, true, nil
	}
	snap, fetchErr := r.fetch(login.AccessToken)
	r.saveUsageResultAt(cachePath, t, saved, now, snap, fetchErr)
	return email, true, nil
}

func (r *Reader) cachedUsageValid(t usageTarget, saved usageFile, now time.Time) bool {
	if *t.usage != nil && now.Sub((*t.usage).FetchedAt) < r.FreshFor {
		return true
	}
	// A failed read is not retried within FreshFor either: the endpoint
	// answers 429 to a burst, and to a token it does not accept.
	if saved.Error != "" && now.Sub(saved.TriedAt) < r.FreshFor {
		*t.note = fmt.Sprintf("%s (%s)", saved.Error, Ago(now, saved.TriedAt))
		return true
	}
	return false
}

func (r *Reader) saveUsageResultAt(cachePath string, t usageTarget, saved usageFile, now time.Time, snap *Snapshot, err error) {
	if err != nil {
		*t.note = err.Error()
		saved.TriedAt, saved.Error = now, err.Error()
		if saveErr := saveUsageFileAt(cachePath, saved); saveErr != nil {
			*t.note += fmt.Sprintf("; not saved: %v", saveErr)
		}
		return
	}
	snap.FetchedAt = now
	*t.usage, *t.live = snap, true
	if saveErr := saveUsageFileAt(cachePath, usageFile{Last: snap, TriedAt: now}); saveErr != nil {
		*t.note = fmt.Sprintf("usage not saved: %v", saveErr)
	}
}

func (r *Reader) homeDir() (string, error) {
	if r.HomeDir != "" {
		return r.HomeDir, nil
	}
	return os.UserHomeDir()
}

func (r *Reader) env() []string {
	if r.Env != nil {
		return r.Env
	}
	return os.Environ()
}

func (r *Reader) keychainInspector() migration.KeychainInspector {
	if r.Keychain != nil {
		return r.Keychain
	}
	if kc, ok := r.Store.(migration.KeychainInspector); ok {
		return kc
	}
	return nil
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func (r *Reader) readAgy(s *Status, saved usageFile, cachePath string, t usageTarget) {
	home, err := r.homeDir()
	if err != nil {
		s.Note = fmt.Sprintf("cannot find home directory: %v", err)
		return
	}
	// Logged in iff the file-mode token exists and is non-empty; never read it.
	tokenPath := profile.AgyTokenPathIn(home, s.Name)
	info, err := os.Stat(tokenPath)
	if errors.Is(err, os.ErrNotExist) || (err == nil && (info.IsDir() || info.Size() == 0)) {
		s.Note = "not logged in"
		return
	}
	if err != nil {
		s.Note = fmt.Sprintf("cannot check login: %v", err)
		return
	}
	s.LoggedIn = true

	now := r.Now()
	if r.cachedUsageValid(t, saved, now) {
		return
	}
	snap, err := r.fetchAgy(s.Name, home)
	r.saveUsageResultAt(cachePath, t, saved, now, snap, err)
}

func (r *Reader) fetchAgy(account, home string) (*Snapshot, error) {
	flagArg, envEntry := profile.AgyLaunch(account)
	args := []string{flagArg, "-p", "/usage", "--output-format", "json"}
	env := profile.WithEnv(r.env(), envEntry)
	if home != "" {
		env = profile.WithEnv(env, "HOME="+home)
	}
	return r.runAgyUsage(home, args, env)
}

func (r *Reader) runAgyUsage(home string, args, env []string) (*Snapshot, error) {
	runner := r.RunAgy
	if runner == nil {
		runner = defaultAgyRunner
	}
	timeout := r.AgyTimeout
	if timeout <= 0 {
		timeout = defaultAgyTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := runner(ctx, home, args, env)
	if err != nil {
		return nil, err
	}
	return parseAgyUsage(out)
}

// ReadOtherLogins returns unmanaged Claude Code and agy logins that exist
// outside ~/.ccp/accounts, caching usage in ~/.ccp/state/usage/<key>.json.
func (r *Reader) ReadOtherLogins(paths *config.Paths) []OtherLogin {
	cacheDir := filepath.Join(paths.CcpDir, "state", "usage")

	type probe func() *OtherLogin
	var probes []probe

	// 1. Claude global (plain claude).
	globalCredKey := filepath.Join(paths.GlobalClaudeDir, ".credentials.json")
	if r.GOOS == "darwin" {
		globalCredKey = migration.KeychainServiceFor("", true)
	}
	globalStateDir := filepath.Dir(paths.GlobalClaudeDir)
	probes = append(probes, func() *OtherLogin {
		out := OtherLogin{
			Kind:     OtherKindClaudeGlobal,
			Where:    "~/.claude",
			CacheKey: "claude-global",
			Harness:  profile.HarnessClaude,
		}
		t := usageTarget{usage: &out.Usage, live: &out.Live, note: &out.Note}
		email, loggedIn, err := r.readClaudeLogin(globalCredKey, globalStateDir, filepath.Join(cacheDir, out.CacheKey+".json"), t)
		if err != nil || !loggedIn {
			return nil
		}
		out.Email = email
		return &out
	})

	// 2. Claude profile logins (each profile dir P except "shared").
	if entries, err := os.ReadDir(paths.ProfilesDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if name == "shared" {
				continue
			}
			pDir := paths.ProfileDir(name)
			info, err := os.Stat(pDir)
			if err != nil || !info.IsDir() {
				continue
			}
			absDir := absCleanPath(pDir)
			if r.GOOS != "darwin" && sameResolvedDir(paths.GlobalClaudeDir, absDir) {
				// Off macOS, ~/.claude/.credentials.json and <profile>/.credentials.json
				// name the same file when ~/.claude links to this profile.
				continue
			}
			credKey := migration.LoginKey(r.GOOS, absDir)
			probes = append(probes, func() *OtherLogin {
				out := OtherLogin{
					Kind:     OtherKindClaudeProfile,
					Where:    "profile " + name,
					CacheKey: "claude-profile-" + name,
					Harness:  profile.HarnessClaude,
					Profile:  name,
				}
				t := usageTarget{usage: &out.Usage, live: &out.Live, note: &out.Note}
				email, loggedIn, err := r.readClaudeLogin(credKey, absDir, filepath.Join(cacheDir, out.CacheKey+".json"), t)
				if err != nil || !loggedIn {
					return nil
				}
				out.Email = email
				return &out
			})
		}
	}

	// 3. agy plain (shared login), macOS only via attribute-only Keychain lookup.
	if r.GOOS == "darwin" {
		if inspector := r.keychainInspector(); inspector != nil {
			probes = append(probes, func() *OtherLogin {
				found, err := inspector.Exists("gemini", "antigravity")
				if err != nil || !found {
					return nil
				}
				out := OtherLogin{
					Kind:     OtherKindAgyPlain,
					Where:    "~/.gemini/antigravity-cli",
					CacheKey: "agy-global",
					Harness:  profile.HarnessAgy,
				}
				cachePath := filepath.Join(cacheDir, out.CacheKey+".json")
				saved := loadUsageFileAt(cachePath)
				out.Usage = saved.Last
				t := usageTarget{usage: &out.Usage, live: &out.Live, note: &out.Note}
				now := r.Now()
				if r.cachedUsageValid(t, saved, now) {
					return &out
				}
				home, homeErr := r.homeDir()
				if homeErr != nil {
					out.Usage = nil
					out.Note = fmt.Sprintf("cannot find home directory: %v", homeErr)
					return &out
				}
				childEnv, realSSH := stripCCPSshClient(r.env())
				if realSSH {
					tokenPath := filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token")
					if !nonEmptyFile(tokenPath) {
						out.Usage = nil
						out.Note = "SSH session without file token (~/.gemini/antigravity-cli/antigravity-oauth-token)"
						return &out
					}
				}
				if home != "" {
					childEnv = profile.WithEnv(childEnv, "HOME="+home)
				}
				snap, fetchErr := r.runAgyUsage(home, []string{"-p", "/usage", "--output-format", "json"}, childEnv)
				r.saveUsageResultAt(cachePath, t, saved, now, snap, fetchErr)
				return &out
			})
		}
	}

	results := make([]*OtherLogin, len(probes))
	var wg sync.WaitGroup
	for i, fn := range probes {
		wg.Add(1)
		go func(idx int, run probe) {
			defer wg.Done()
			results[idx] = run()
		}(i, fn)
	}
	wg.Wait()

	var out []OtherLogin
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// stripCCPSshClient removes SSH_CLIENT when it holds ccp's own "ccp 0 0" marker
// and reports whether a real SSH session env var remains set.
func stripCCPSshClient(env []string) (cleaned []string, realSSH bool) {
	cleaned = make([]string, 0, len(env))
	for _, entry := range env {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		switch k {
		case "SSH_CLIENT":
			if v == "ccp 0 0" {
				continue
			}
			if v != "" {
				realSSH = true
			}
		case "SSH_CONNECTION", "SSH_TTY":
			if v != "" {
				realSSH = true
			}
		}
		cleaned = append(cleaned, entry)
	}
	return cleaned, realSSH
}

func absCleanPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(p)
}

func sameResolvedDir(a, b string) bool {
	if profile.SamePath(a, b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func defaultAgyRunner(ctx context.Context, dir string, args, env []string) ([]byte, error) {
	exe, err := exec.LookPath("agy")
	if err != nil {
		return nil, errors.New("agy not found on PATH; install Antigravity CLI (agy) first")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	configureAgyCmd(cmd)
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("agy usage timed out")
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("agy usage failed (exit %d)", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("agy usage failed: %w", err)
	}
	return out, nil
}

func parseAgyUsage(data []byte) (*Snapshot, error) {
	var resp struct {
		Status  string `json:"status"`
		Command struct {
			Data struct {
				Groups []struct {
					Name    string `json:"name"`
					Buckets []struct {
						Window            string     `json:"window"`
						RemainingFraction *float64   `json:"remaining_fraction"`
						ResetTime         *time.Time `json:"reset_time"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"data"`
		} `json:"command"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &resp); err != nil {
		return nil, errors.New("agy usage output is not the expected JSON")
	}
	if resp.Status != "" && resp.Status != "SUCCESS" {
		return nil, fmt.Errorf("agy usage returned status %s", resp.Status)
	}
	snap := &Snapshot{}
	for _, g := range resp.Command.Data.Groups {
		for _, b := range g.Buckets {
			if b.RemainingFraction == nil {
				continue
			}
			used := (1 - *b.RemainingFraction) * 100
			if used < 0 {
				used = 0
			} else if used > 100 {
				used = 100
			}
			w := Window{Utilization: used}
			if b.ResetTime != nil {
				w.ResetsAt = *b.ResetTime
			}
			if g.Name == "Gemini Models" {
				switch b.Window {
				case "5h":
					snap.FiveHour = &w
				case "weekly":
					snap.SevenDay = &w
				}
			} else if b.Window == "weekly" && g.Name != "" {
				snap.Scoped = append(snap.Scoped, Scoped{Name: g.Name, Window: w})
			}
		}
	}
	if snap.FiveHour == nil && snap.SevenDay == nil {
		return nil, errors.New("agy usage response has no Gemini Models 5h or weekly limit")
	}
	return snap, nil
}

type apiWindow struct {
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// fetch reads usage with an access token. Errors never quote the token or the
// response body.
func (r *Reader) fetch(token string) (*Snapshot, error) {
	req, err := http.NewRequest(http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", r.UserAgent)
	resp, err := r.HTTP.Do(req)
	if err != nil {
		var uerr interface{ Timeout() bool }
		if errors.As(err, &uerr) && uerr.Timeout() {
			return nil, errors.New("usage request timed out")
		}
		return nil, errors.New("usage request failed (network)")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("usage request rate limited (HTTP 429)")
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("usage request refused the token (HTTP 401); Claude Code renews it when it starts")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("usage request failed (HTTP %d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, errors.New("usage response could not be read")
	}
	var api struct {
		FiveHour *apiWindow `json:"five_hour"`
		SevenDay *apiWindow `json:"seven_day"`
		Limits   []struct {
			Kind     string     `json:"kind"`
			Percent  *float64   `json:"percent"`
			ResetsAt *time.Time `json:"resets_at"`
			Scope    *struct {
				Model *struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			} `json:"scope"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(body, &api); err != nil {
		return nil, errors.New("usage response is not the expected JSON")
	}
	snap := &Snapshot{FiveHour: api.FiveHour.window(), SevenDay: api.SevenDay.window()}
	for _, l := range api.Limits {
		w := (&apiWindow{Utilization: l.Percent, ResetsAt: l.ResetsAt}).window()
		switch {
		case w == nil:
		case l.Kind == "weekly_all" && snap.SevenDay == nil:
			snap.SevenDay = w
		case l.Kind == "weekly_scoped" && l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != "":
			snap.Scoped = append(snap.Scoped, Scoped{Name: l.Scope.Model.DisplayName, Window: *w})
		}
	}
	if snap.FiveHour == nil && snap.SevenDay == nil {
		return nil, errors.New("usage response has no five_hour or seven_day limit")
	}
	return snap, nil
}

func (w *apiWindow) window() *Window {
	if w == nil || w.Utilization == nil {
		return nil
	}
	out := &Window{Utilization: *w.Utilization}
	if w.ResetsAt != nil {
		out.ResetsAt = *w.ResetsAt
	}
	return out
}

// usageFile is UsageFile: the last good read, and the last try when it failed.
type usageFile struct {
	Last    *Snapshot `json:"last,omitempty"`
	TriedAt time.Time `json:"tried_at,omitzero"`
	Error   string    `json:"error,omitempty"`
}

func loadUsageFile(dir string) usageFile {
	return loadUsageFileAt(filepath.Join(dir, UsageFile))
}

func loadUsageFileAt(path string) usageFile {
	var f usageFile
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &f) != nil {
		return usageFile{}
	}
	if f.Last != nil && f.Last.FetchedAt.IsZero() {
		f.Last = nil
	}
	return f
}

func saveUsageFile(dir string, f usageFile) error {
	return saveUsageFileAt(filepath.Join(dir, UsageFile), f)
}

func saveUsageFileAt(path string, f usageFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// accountEmail returns the email of the login Claude Code recorded in the
// account's .claude.json, or "".
func accountEmail(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if err != nil {
		return ""
	}
	var state struct {
		OauthAccount *struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(data, &state) != nil || state.OauthAccount == nil {
		return ""
	}
	return state.OauthAccount.EmailAddress
}

// Touch records that ccp started the account in dir now.
func Touch(dir string, now time.Time) error {
	path := filepath.Join(dir, LastUsedFile)
	if err := os.WriteFile(path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0644); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

// Pick returns the logged-in account with the most limit left, and a short
// reason. Accounts are ranked:
//  1. usage known, under 80% of the 5-hour and 95% of the 7-day limit, lowest 5-hour use first;
//  2. usage unknown (never read, or the read failed with nothing saved);
//  3. usage known and above those marks, lowest first;
//  4. a limit used up.
//
// Ties go to the account ccp started least recently, so with no usage data at
// all the pick rotates.
func Pick(statuses []Status, now time.Time) (Status, string, error) {
	var pool []Status
	for _, s := range statuses {
		if s.LoggedIn {
			pool = append(pool, s)
		}
	}
	if len(pool) == 0 {
		return Status{}, "", errors.New("no account in the pool is logged in")
	}
	type ranked struct {
		s          Status
		tier       int
		five, week float64
	}
	rs := make([]ranked, len(pool))
	for i, s := range pool {
		r := ranked{s: s, tier: 1}
		if s.Usage != nil {
			five, okF := s.Usage.FiveHour.At(now)
			week, okW := s.Usage.SevenDay.At(now)
			if okF || okW {
				r.five, r.week = five, week
				switch {
				case five >= 100 || week >= 100:
					r.tier = 3
				case five >= busyFiveHour || week >= busySevenDay:
					r.tier = 2
				default:
					r.tier = 0
				}
			}
		}
		rs[i] = r
	}
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if a.tier != 1 {
			if fa, fb := int(a.five), int(b.five); fa != fb {
				return fa < fb
			}
			if wa, wb := int(a.week), int(b.week); wa != wb {
				return wa < wb
			}
		}
		if !a.s.LastUsed.Equal(b.s.LastUsed) {
			return a.s.LastUsed.Before(b.s.LastUsed)
		}
		return a.s.Name < b.s.Name
	})
	best := rs[0]
	if best.tier == 1 {
		return best.s, "no usage data; least recently used", nil
	}
	reason := Describe(best.s.Usage, now)
	if !best.s.Live {
		reason += ", as of " + Ago(now, best.s.Usage.FetchedAt)
	}
	if best.tier == 3 {
		reason += "; every account has used up a limit"
	}
	return best.s, reason, nil
}

// Describe formats the ranked limits of a read: "5h 12%, 7d 40%", leaving
// out a window the plan does not report.
func Describe(s *Snapshot, now time.Time) string {
	var parts []string
	if v, ok := s.FiveHour.At(now); ok {
		parts = append(parts, fmt.Sprintf("5h %.0f%%", v))
	}
	if v, ok := s.SevenDay.At(now); ok {
		parts = append(parts, fmt.Sprintf("7d %.0f%%", v))
	}
	return strings.Join(parts, ", ")
}

// Ago formats how long before now t was, coarsely ("3m ago").
func Ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
