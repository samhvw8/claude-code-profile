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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

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
	Name, Profile, Dir string
	Email              string
	LoggedIn           bool
	Usage              *Snapshot // nil: never read
	Live               bool      // Usage was read just now
	Note               string    // why Usage is not live
	LastUsed           time.Time // zero: never started by ccp
}

// Reader reads account logins and their usage.
type Reader struct {
	Store     migration.CredentialStore
	GOOS      string
	URL       string
	HTTP      *http.Client
	UserAgent string
	Now       func() time.Time
	FreshFor  time.Duration
}

// NewReader returns a reader for this machine's credential store.
func NewReader(version string) (*Reader, error) {
	store, err := migration.NewCredentialStore(runtime.GOOS)
	if err != nil {
		return nil, err
	}
	return &Reader{
		Store: store,
		GOOS:  runtime.GOOS,
		URL:   DefaultURL,
		HTTP: &http.Client{
			Timeout: 5 * time.Second,
			// The token must never follow a redirect to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		UserAgent: "ccp/" + version,
		Now:       time.Now,
		FreshFor:  FreshFor,
	}, nil
}

// Read returns the status of each account, in the given order. Accounts whose
// saved usage is older than FreshFor are read again, in parallel.
func (r *Reader) Read(accounts []profile.Account) []Status {
	out := make([]Status, len(accounts))
	var wg sync.WaitGroup
	for i, a := range accounts {
		out[i] = Status{Name: a.Name, Profile: a.Profile, Dir: a.Dir}
		wg.Add(1)
		go func(s *Status) {
			defer wg.Done()
			r.readOne(s)
		}(&out[i])
	}
	wg.Wait()
	return out
}

func (r *Reader) readOne(s *Status) {
	s.Email = accountEmail(s.Dir)
	if info, err := os.Stat(filepath.Join(s.Dir, LastUsedFile)); err == nil {
		s.LastUsed = info.ModTime()
	}
	saved := loadUsageFile(s.Dir)
	s.Usage = saved.Last

	secret, _, found, err := r.Store.Get(migration.LoginKey(r.GOOS, s.Dir))
	if err != nil {
		s.Note = fmt.Sprintf("cannot read the login: %v", err)
		return
	}
	login, ok := migration.ParseLogin(secret)
	if !found || !ok {
		s.Note = "not logged in"
		return
	}
	s.LoggedIn = true

	now := r.Now()
	if s.Usage != nil && now.Sub(s.Usage.FetchedAt) < r.FreshFor {
		return
	}
	// A failed read is not retried within FreshFor either: the endpoint
	// answers 429 to a burst, and to a token it does not accept.
	if saved.Error != "" && now.Sub(saved.TriedAt) < r.FreshFor {
		s.Note = fmt.Sprintf("%s (%s)", saved.Error, Ago(now, saved.TriedAt))
		return
	}
	if login.AccessToken == "" || login.ExpiresAt > 0 && now.UnixMilli() >= login.ExpiresAt {
		s.Note = "token expired; Claude Code renews it when it starts"
		return
	}
	snap, err := r.fetch(login.AccessToken)
	if err != nil {
		s.Note = err.Error()
		saved.TriedAt, saved.Error = now, err.Error()
		if err := saveUsageFile(s.Dir, saved); err != nil {
			s.Note += fmt.Sprintf("; not saved: %v", err)
		}
		return
	}
	snap.FetchedAt = now
	s.Usage, s.Live = snap, true
	if err := saveUsageFile(s.Dir, usageFile{Last: snap, TriedAt: now}); err != nil {
		s.Note = fmt.Sprintf("usage not saved: %v", err)
	}
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
	var f usageFile
	data, err := os.ReadFile(filepath.Join(dir, UsageFile))
	if err != nil || json.Unmarshal(data, &f) != nil {
		return usageFile{}
	}
	if f.Last != nil && f.Last.FetchedAt.IsZero() {
		f.Last = nil
	}
	return f
}

func saveUsageFile(dir string, f usageFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+UsageFile+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, UsageFile))
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
