package accountusage

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var now = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

func snap(five, week float64, fetched time.Time) *Snapshot {
	return &Snapshot{FetchedAt: fetched, FiveHour: &Window{Utilization: five}, SevenDay: &Window{Utilization: week}}
}

func TestPick(t *testing.T) {
	old := now.Add(-2 * time.Hour)
	tests := []struct {
		name   string
		in     []Status
		want   string
		reason string
	}{
		{"lowest 5h wins", []Status{
			{Name: "a", LoggedIn: true, Live: true, Usage: snap(60, 10, now)},
			{Name: "b", LoggedIn: true, Live: true, Usage: snap(20, 90, now)},
		}, "b", "5h 20%, 7d 90%"},
		{"7d breaks a 5h tie", []Status{
			{Name: "a", LoggedIn: true, Live: true, Usage: snap(20, 50, now)},
			{Name: "b", LoggedIn: true, Live: true, Usage: snap(20, 30, now)},
		}, "b", ""},
		{"unknown beats busy", []Status{
			{Name: "busy", LoggedIn: true, Live: true, Usage: snap(85, 10, now)},
			{Name: "unknown", LoggedIn: true},
		}, "unknown", "no usage data; least recently used"},
		{"known and free beats unknown", []Status{
			{Name: "unknown", LoggedIn: true},
			{Name: "free", LoggedIn: true, Live: true, Usage: snap(40, 10, now)},
		}, "free", ""},
		{"busy beats used up", []Status{
			{Name: "out", LoggedIn: true, Live: true, Usage: snap(100, 10, now)},
			{Name: "busy", LoggedIn: true, Live: true, Usage: snap(90, 96, now)},
		}, "busy", ""},
		{"no data: least recently used", []Status{
			{Name: "a", LoggedIn: true, LastUsed: now.Add(-time.Minute)},
			{Name: "b", LoggedIn: true, LastUsed: now.Add(-time.Hour)},
			{Name: "c", LoggedIn: true, LastUsed: now.Add(-10 * time.Minute)},
		}, "b", "no usage data; least recently used"},
		{"never used goes first", []Status{
			{Name: "a", LoggedIn: true, LastUsed: now.Add(-time.Hour)},
			{Name: "b", LoggedIn: true},
		}, "b", ""},
		{"logged-out accounts never picked", []Status{
			{Name: "a", Usage: snap(0, 0, now)},
			{Name: "b", LoggedIn: true, Live: true, Usage: snap(99, 99, now)},
		}, "b", ""},
		{"a reset window counts as 0", []Status{
			{Name: "a", LoggedIn: true, Live: true, Usage: snap(30, 10, now)},
			{Name: "b", LoggedIn: true, Usage: &Snapshot{FetchedAt: old,
				FiveHour: &Window{Utilization: 95, ResetsAt: now.Add(-time.Minute)},
				SevenDay: &Window{Utilization: 5}}},
		}, "b", "5h 0%, 7d 5%, as of 2h ago"},
		{"all used up still picks", []Status{
			{Name: "a", LoggedIn: true, Live: true, Usage: snap(100, 100, now)},
		}, "a", "5h 100%, 7d 100%; every account has used up a limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason, err := Pick(tt.in, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.want {
				t.Errorf("picked %s, want %s", got.Name, tt.want)
			}
			if tt.reason != "" && reason != tt.reason {
				t.Errorf("reason %q, want %q", reason, tt.reason)
			}
		})
	}
	if _, _, err := Pick([]Status{{Name: "a"}}, now); err == nil {
		t.Error("a pool with no login must fail")
	}
}

const token = "sk-ant-oat01-SECRET"

func cred(expires time.Time) []byte {
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": token, "refreshToken": "r", "expiresAt": expires.UnixMilli(),
	}})
	return b
}

// setup makes one account folder holding secret as its Linux login.
func setup(t *testing.T, secret []byte) (profile.Account, *Reader, *int32, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	if secret != nil {
		os.WriteFile(filepath.Join(dir, ".credentials.json"), secret, 0600)
	}
	os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@example.invalid"}}`), 0600)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"five_hour":{"utilization":42.4,"resets_at":"2026-11-18T12:00:00.123+00:00"},"seven_day":{"utilization":7,"resets_at":null},"seven_day_opus":null}`))
	}))
	t.Cleanup(srv.Close)
	r := &Reader{Store: migration.FileStore{}, GOOS: "linux", URL: srv.URL, HTTP: srv.Client(),
		UserAgent: "ccp/test", Now: func() time.Time { return now }, FreshFor: FreshFor}
	return profile.Account{Name: "w1", Profile: "dev", Dir: dir}, r, &calls, srv
}

func TestReader_LiveReadIsSaved(t *testing.T) {
	a, r, calls, _ := setup(t, cred(now.Add(time.Hour)))
	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || !s.Live || s.Email != "a@example.invalid" || *calls != 1 {
		t.Fatalf("status %+v, calls %d", s, *calls)
	}
	if v, _ := s.Usage.FiveHour.At(now); v != 42.4 {
		t.Errorf("five hour %v", v)
	}
	if !s.Usage.FiveHour.ResetsAt.Equal(time.Date(2026, 11, 18, 12, 0, 0, 123e6, time.UTC)) {
		t.Errorf("resets_at %v", s.Usage.FiveHour.ResetsAt)
	}
	if !s.Usage.SevenDay.ResetsAt.IsZero() {
		t.Errorf("null resets_at gave %v", s.Usage.SevenDay.ResetsAt)
	}
	saved := loadUsageFile(a.Dir).Last
	if saved == nil || saved.FiveHour.Utilization != 42.4 || !saved.FetchedAt.Equal(now) {
		t.Fatalf("saved %+v", saved)
	}
	data, _ := os.ReadFile(filepath.Join(a.Dir, UsageFile))
	if strings.Contains(string(data), token) {
		t.Error("the usage file holds the token")
	}

	// Within FreshFor the saved read is used, without a request.
	s = r.Read([]profile.Account{a})[0]
	if s.Live || *calls != 1 || s.Usage == nil {
		t.Errorf("fresh read: live %v calls %d", s.Live, *calls)
	}
	// After it, a new request.
	r.Now = func() time.Time { return now.Add(6 * time.Minute) }
	if s = r.Read([]profile.Account{a})[0]; !s.Live || *calls != 2 {
		t.Errorf("stale read: live %v calls %d", s.Live, *calls)
	}
}

func TestReader_FailuresKeepSavedUsage(t *testing.T) {
	a, r, _, srv := setup(t, cred(now.Add(time.Hour)))
	saveUsageFile(a.Dir, usageFile{Last: snap(10, 20, now.Add(-time.Hour))})
	r.FreshFor = 0 // retry at once, to see each failure

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(token)) // a body echoing the token must never reach a message
	})
	s := r.Read([]profile.Account{a})[0]
	if s.Live || s.Usage == nil || s.Usage.FiveHour.Utilization != 10 || !strings.Contains(s.Note, "429") {
		t.Errorf("429: %+v", s)
	}
	if strings.Contains(s.Note, token) {
		t.Error("note quotes the token")
	}

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"unexpected":1}`)) })
	if s = r.Read([]profile.Account{a})[0]; s.Live || s.Usage == nil || !strings.Contains(s.Note, "no five_hour") {
		t.Errorf("changed shape: %+v", s)
	}

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/steal", http.StatusFound)
	})
	r.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if s = r.Read([]profile.Account{a})[0]; s.Live || !strings.Contains(s.Note, "HTTP 302") {
		t.Errorf("redirect: %+v", s)
	}
}

// The shape the endpoint returned for a team plan in November 2026: no
// 5-hour window, the week in seven_day and in limits, a model-scoped week.
const teamBody = `{"five_hour":null,"seven_day":{"utilization":96,"resets_at":"2026-11-21T18:59:59.963511+00:00","used_dollars":null},
"limits":[{"kind":"weekly_all","percent":96,"resets_at":"2026-11-21T18:59:59.963511+00:00","scope":null},
{"kind":"weekly_scoped","percent":40,"resets_at":"2026-11-21T19:00:00+00:00","scope":{"model":{"display_name":"Fable","id":null},"surface":null}}],
"extra_usage":{"utilization":null},"seven_day_opus":null}`

func TestReader_TeamPlanShape(t *testing.T) {
	a, r, _, srv := setup(t, cred(now.Add(time.Hour)))
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(teamBody)) })
	s := r.Read([]profile.Account{a})[0]
	if !s.Live || s.Usage.FiveHour != nil || s.Usage.SevenDay == nil || s.Usage.SevenDay.Utilization != 96 {
		t.Fatalf("status %+v usage %+v", s, s.Usage)
	}
	if len(s.Usage.Scoped) != 1 || s.Usage.Scoped[0].Name != "Fable" || s.Usage.Scoped[0].Utilization != 40 {
		t.Errorf("scoped %+v", s.Usage.Scoped)
	}
	if d := Describe(s.Usage, now); d != "7d 96%" {
		t.Errorf("describe %q", d)
	}
	// seven_day missing: the weekly_all limit stands in.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"limits":[{"kind":"weekly_all","percent":12,"resets_at":null}]}`))
	})
	r.FreshFor = 0
	if s = r.Read([]profile.Account{a})[0]; s.Usage == nil || s.Usage.SevenDay == nil || s.Usage.SevenDay.Utilization != 12 {
		t.Errorf("weekly_all fallback: %+v", s)
	}
	// A busy week loses to a free one even with no 5-hour data.
	free := Status{Name: "free", LoggedIn: true, Live: true, Usage: &Snapshot{FetchedAt: now, SevenDay: &Window{Utilization: 30}}}
	busy := Status{Name: "busy", LoggedIn: true, Live: true, Usage: &Snapshot{FetchedAt: now, SevenDay: &Window{Utilization: 96}}}
	if got, reason, _ := Pick([]Status{busy, free}, now); got.Name != "free" || reason != "7d 30%" {
		t.Errorf("picked %s (%s)", got.Name, reason)
	}
}

func TestReader_ExpiredTokenIsNotSent(t *testing.T) {
	a, r, calls, _ := setup(t, cred(now.Add(-time.Minute)))
	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || s.Live || *calls != 0 || !strings.Contains(s.Note, "expired") {
		t.Errorf("status %+v, calls %d", s, *calls)
	}
}

func TestReader_NotLoggedIn(t *testing.T) {
	for name, secret := range map[string][]byte{
		"no file":  nil,
		"mcp only": []byte(`{"mcpOAuth":{"srv":{"accessToken":"x"}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			a, r, calls, _ := setup(t, secret)
			s := r.Read([]profile.Account{a})[0]
			if s.LoggedIn || *calls != 0 || s.Note != "not logged in" {
				t.Errorf("status %+v, calls %d", s, *calls)
			}
		})
	}
}

func TestTouch(t *testing.T) {
	dir := t.TempDir()
	if err := Touch(dir, now); err != nil {
		t.Fatal(err)
	}
	r := &Reader{Store: migration.FileStore{}, GOOS: "linux", Now: func() time.Time { return now }}
	s := r.Read([]profile.Account{{Name: "a", Dir: dir}})[0]
	if !s.LastUsed.Equal(now) {
		t.Errorf("last used %v", s.LastUsed)
	}
}

func TestReader_FailedReadWaits(t *testing.T) {
	a, r, calls, srv := setup(t, cred(now.Add(time.Hour)))
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	r.Read([]profile.Account{a})
	s := r.Read([]profile.Account{a})[0]
	if *calls != 1 || !strings.Contains(s.Note, "429") || !strings.Contains(s.Note, "just now") {
		t.Errorf("second read within FreshFor: calls %d, note %q", *calls, s.Note)
	}
	r.Now = func() time.Time { return now.Add(6 * time.Minute) }
	r.Read([]profile.Account{a})
	if *calls != 2 {
		t.Errorf("after FreshFor: calls %d, want a retry", *calls)
	}
}

const agyAcctCSample = `{"conversation_id":"","status":"SUCCESS","response":"","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":0.6148027181625366,"reset_time":"2026-11-24T23:40:35Z"},{"id":"gemini-5h","name":"Five Hour Limit Remaining","window":"5h","remaining_fraction":0.8139199018478394,"reset_time":"2026-11-20T01:40:35Z"}]},{"name":"Claude and GPT models","buckets":[{"id":"3p-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":0.030352680012583733,"reset_time":"2026-11-25T03:27:02Z"},{"id":"3p-5h","name":"Five Hour Limit Remaining","window":"5h","remaining_fraction":1,"reset_time":"2026-11-20T05:47:01Z"}]}]}}}`

func setupAgy(t *testing.T, name string, tokenContent []byte, runner AgyRunner) (profile.Account, *Reader, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	acctDir := filepath.Join(root, "ccp", "accounts", name)
	if err := os.MkdirAll(acctDir, 0755); err != nil {
		t.Fatal(err)
	}
	if tokenContent != nil {
		tokPath := filepath.Join(home, ".gemini", profile.AgyAppDataDirName(name), "antigravity-oauth-token")
		if err := os.MkdirAll(filepath.Dir(tokPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tokPath, tokenContent, 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := &Reader{
		Store:      migration.FileStore{},
		GOOS:       "linux",
		Now:        func() time.Time { return now },
		FreshFor:   FreshFor,
		HomeDir:    home,
		RunAgy:     runner,
		AgyTimeout: time.Second,
	}
	return profile.Account{Name: name, Profile: "dev", Harness: profile.HarnessAgy, Dir: acctDir}, r, home
}

func TestReader_AgyNotLoggedIn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		token []byte
	}{
		{"missing token file", nil},
		{"empty token file", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			a, r, _ := setupAgy(t, "agy1", tc.token, func(context.Context, string, []string, []string) ([]byte, error) {
				atomic.AddInt32(&calls, 1)
				return []byte(agyAcctCSample), nil
			})
			s := r.Read([]profile.Account{a})[0]
			if s.LoggedIn || calls != 0 || s.Note != "not logged in" {
				t.Errorf("status %+v, calls %d", s, calls)
			}
		})
	}
}

func TestReader_AgySampleAcctCAndCaching(t *testing.T) {
	var calls int32
	var gotDir string
	var gotArgs, gotEnv []string
	a, r, home := setupAgy(t, "acctC", []byte("secret-never-read"), func(_ context.Context, dir string, args, env []string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		gotDir, gotArgs, gotEnv = dir, args, env
		return []byte(agyAcctCSample), nil
	})
	// Make token file 0000 to verify ccp only stats it and never opens/reads it.
	tokPath := filepath.Join(home, ".gemini", profile.AgyAppDataDirName("acctC"), "antigravity-oauth-token")
	if err := os.Chmod(tokPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(tokPath, 0600) })

	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || !s.Live || s.Email != "" || s.Harness != profile.HarnessAgy || calls != 1 {
		t.Fatalf("status %+v, calls %d", s, calls)
	}
	if gotDir != home {
		t.Errorf("cwd = %q, want home %q", gotDir, home)
	}
	wantArgs := []string{"--app_data_dir=antigravity-cli-ccp-acctC", "-p", "/usage", "--output-format", "json"}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Errorf("args = %v, want %v", gotArgs, wantArgs)
	}
	if !slices.Contains(gotEnv, "SSH_CLIENT=ccp 0 0") {
		t.Errorf("env lacks SSH_CLIENT=ccp 0 0")
	}
	if v, ok := s.Usage.SevenDay.At(now); !ok || math.Abs(v-38.519728) > 0.01 {
		t.Errorf("acctC 7d = %v (%v), want ~38.52", v, ok)
	}
	if v, ok := s.Usage.FiveHour.At(now); !ok || math.Abs(v-18.608010) > 0.01 {
		t.Errorf("acctC 5h = %v (%v), want ~18.61", v, ok)
	}
	if len(s.Usage.Scoped) != 1 || s.Usage.Scoped[0].Name != "Claude and GPT models" || math.Abs(s.Usage.Scoped[0].Utilization-96.964732) > 0.01 {
		t.Errorf("scoped = %+v", s.Usage.Scoped)
	}

	// Second read within 5 minutes reuses cache without invoking agy.
	s = r.Read([]profile.Account{a})[0]
	if s.Live || calls != 1 || s.Usage == nil {
		t.Errorf("cached read: live=%v calls=%d usage=%v", s.Live, calls, s.Usage)
	}

	// Verify exact 0.617 -> 38.3% mapping.
	r.Now = func() time.Time { return now.Add(6 * time.Minute) }
	r.RunAgy = func(context.Context, string, []string, []string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte(`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"window":"weekly","remaining_fraction":0.617,"reset_time":"2026-11-24T23:40:35Z"},{"window":"5h","remaining_fraction":0.8,"reset_time":"2026-11-20T01:40:35Z"}]}]}}}`), nil
	}
	s = r.Read([]profile.Account{a})[0]
	if !s.Live || calls != 2 {
		t.Fatalf("stale refresh: live=%v calls=%d", s.Live, calls)
	}
	if v, _ := s.Usage.SevenDay.At(now); math.Abs(v-38.3) > 1e-6 {
		t.Errorf("0.617 remaining gave %v%% used, want 38.3%%", v)
	}
	if v, _ := s.Usage.FiveHour.At(now); math.Abs(v-20) > 1e-6 {
		t.Errorf("0.8 remaining gave %v%% used, want 20%%", v)
	}
}

func TestReader_AgyFailureCacheAndTimeout(t *testing.T) {
	var calls int32
	a, r, _ := setupAgy(t, "agy1", []byte("tok"), func(ctx context.Context, _ string, _ []string, _ []string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		<-ctx.Done()
		return nil, errors.New("agy usage timed out")
	})
	r.AgyTimeout = 20 * time.Millisecond
	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || s.Live || !strings.Contains(s.Note, "timed out") || calls != 1 {
		t.Fatalf("timeout read: %+v calls=%d", s, calls)
	}
	// Failure is cached for FreshFor.
	s = r.Read([]profile.Account{a})[0]
	if calls != 1 || !strings.Contains(s.Note, "timed out") {
		t.Errorf("cached failure: %+v calls=%d", s, calls)
	}
}

func TestReader_MixedClaudeAndAgyParallel(t *testing.T) {
	claudeAcc, r, _, srv := setup(t, cred(now.Add(time.Hour)))
	_ = srv
	// Avoid loopback bind in sandboxed environments: test Mixed Read with expired Claude + fake Agy.
	claudeAcc, r, _, _ = setup(t, cred(now.Add(-time.Minute)))
	home := t.TempDir()
	r.HomeDir = home
	agyDir := t.TempDir()
	tokPath := filepath.Join(home, ".gemini", profile.AgyAppDataDirName("agy1"), "antigravity-oauth-token")
	os.MkdirAll(filepath.Dir(tokPath), 0755)
	os.WriteFile(tokPath, []byte("tok"), 0600)
	r.RunAgy = func(context.Context, string, []string, []string) ([]byte, error) {
		return []byte(agyAcctCSample), nil
	}
	agyAcc := profile.Account{Name: "agy1", Profile: "dev", Harness: profile.HarnessAgy, Dir: agyDir}
	got := r.Read([]profile.Account{claudeAcc, agyAcc})
	if len(got) != 2 || !got[0].LoggedIn || !got[1].LoggedIn || !got[1].Live {
		t.Fatalf("mixed read = %+v", got)
	}
}

func TestReader_AgyMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a, r, _ := setupAgy(t, "agy1", []byte("tok"), nil)
	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || s.Live || !strings.Contains(s.Note, "agy not found") {
		t.Errorf("missing agy: %+v", s)
	}
}

func TestParseAgyUsage_EdgeCases(t *testing.T) {
	if _, err := parseAgyUsage([]byte("not-json")); err == nil {
		t.Error("invalid JSON accepted")
	}
	if _, err := parseAgyUsage([]byte(`{"status":"ERROR"}`)); err == nil || !strings.Contains(err.Error(), "ERROR") {
		t.Errorf("non-SUCCESS status err = %v", err)
	}
	if _, err := parseAgyUsage([]byte(`{"status":"SUCCESS","command":{"data":{"groups":[{"name":"Other","buckets":[{"window":"weekly","remaining_fraction":0.5}]}]}}}`)); err == nil {
		t.Error("missing Gemini Models accepted")
	}
	snap, err := parseAgyUsage([]byte(`{"status":"SUCCESS","command":{"data":{"groups":[{"name":"Gemini Models","buckets":[{"window":"5h","remaining_fraction":1.5},{"window":"weekly","remaining_fraction":-0.2}]}]}}}`))
	if err != nil {
		t.Fatalf("clamping parse failed: %v", err)
	}
	if v, ok := snap.FiveHour.At(now); !ok || v != 0 || !snap.FiveHour.ResetsAt.IsZero() {
		t.Errorf("5h clamped = %v (%v) reset=%v", v, ok, snap.FiveHour.ResetsAt)
	}
	if v, ok := snap.SevenDay.At(now); !ok || v != 100 {
		t.Errorf("7d clamped = %v (%v)", v, ok)
	}
}

func TestDefaultAgyRunner_PropagatesHomeDirAndExitError(t *testing.T) {
	binDir := t.TempDir()
	a, r, home := setupAgy(t, "agy1", []byte("tok"), nil)
	t.Setenv("HOME", t.TempDir()) // deliberately different from r.HomeDir
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	script := "#!/bin/sh\nif [ \"$HOME\" != \"" + home + "\" ]; then\n  echo \"wrong HOME: $HOME\" >&2\n  exit 2\nfi\nprintf '%s' '{\"status\":\"SUCCESS\",\"command\":{\"data\":{\"groups\":[{\"name\":\"Gemini Models\",\"buckets\":[{\"window\":\"5h\",\"remaining_fraction\":0.5,\"reset_time\":\"2026-11-20T01:40:35Z\"}]}]}}}'\n"
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	s := r.Read([]profile.Account{a})[0]
	if !s.LoggedIn || !s.Live || s.Usage == nil || s.Usage.FiveHour == nil || s.Usage.FiveHour.Utilization != 50 {
		t.Fatalf("defaultAgyRunner with custom HomeDir: %+v", s)
	}

	// Non-zero exit from agy records exit code error.
	a2, r2, _ := setupAgy(t, "agy2", []byte("tok"), nil)
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	s2 := r2.Read([]profile.Account{a2})[0]
	if !s2.LoggedIn || s2.Live || !strings.Contains(s2.Note, "exit 7") {
		t.Errorf("exit error: %+v", s2)
	}
}

func TestPick_MultipleAgyAccounts(t *testing.T) {
	statuses := []Status{
		{Name: "agy-busy", Harness: profile.HarnessAgy, LoggedIn: true, Usage: snap(60, 40, now)},
		{Name: "agy-free", Harness: profile.HarnessAgy, LoggedIn: true, Usage: snap(15, 20, now)},
		{Name: "agy-out", Harness: profile.HarnessAgy, LoggedIn: false, Usage: snap(0, 0, now)},
	}
	picked, _, err := Pick(statuses, now)
	if err != nil || picked.Name != "agy-free" {
		t.Errorf("picked = %+v, err = %v", picked, err)
	}
}

type fakeCredKeychain struct {
	mu       sync.Mutex
	secrets  map[string][]byte
	services map[[2]string]bool
	existsFn func(service, acct string) (bool, error)
}

func (f *fakeCredKeychain) Get(key string) ([]byte, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.secrets[key]
	return v, "", ok, nil
}

func (f *fakeCredKeychain) Set(string, string, []byte) error       { return nil }
func (f *fakeCredKeychain) Delete(string, string) (string, error) { return "", nil }

func (f *fakeCredKeychain) Exists(service, acct string) (bool, error) {
	if f.existsFn != nil {
		return f.existsFn(service, acct)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.services[[2]string{service, acct}], nil
}

func (f *fakeCredKeychain) ListServices(string) ([]string, error) { return nil, nil }

func otherTestPaths(t *testing.T) (*config.Paths, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	ccpDir := filepath.Join(root, "ccp")
	os.MkdirAll(home, 0755)
	paths := &config.Paths{
		CcpDir:          ccpDir,
		ClaudeDir:       filepath.Join(home, ".claude"),
		GlobalClaudeDir: filepath.Join(home, ".claude"),
		HubDir:          filepath.Join(ccpDir, "hub"),
		ProfilesDir:     filepath.Join(ccpDir, "profiles"),
		SharedDir:       filepath.Join(ccpDir, "profiles", "shared"),
		StoreDir:        filepath.Join(ccpDir, "store"),
	}
	os.MkdirAll(paths.HubDir, 0755)
	os.MkdirAll(paths.SharedDir, 0755)
	return paths, home
}

func TestReader_ReadOtherLogins_ClaudeDarwinAndCache(t *testing.T) {
	paths, home := otherTestPaths(t)
	defDir := paths.ProfileDir("default")
	gtDir := paths.ProfileDir("companyA")
	emptyDir := paths.ProfileDir("empty")
	for _, d := range []string{defDir, gtDir, emptyDir} {
		os.MkdirAll(d, 0755)
	}
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"global@example.invalid"}}`), 0600)
	os.WriteFile(filepath.Join(defDir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"default@example.invalid"}}`), 0600)

	absDef, _ := filepath.Abs(defDir)
	absGt, _ := filepath.Abs(gtDir)
	store := &fakeCredKeychain{
		secrets: map[string][]byte{
			migration.KeychainServiceFor("", true):       cred(now.Add(time.Hour)),
			migration.LoginKey("darwin", filepath.Clean(absDef)): cred(now.Add(time.Hour)),
			migration.LoginKey("darwin", filepath.Clean(absGt)):  []byte(`{"mcpOAuth":{"s":{"accessToken":"x"}}}`),
		},
	}

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"five_hour":{"utilization":15},"seven_day":{"utilization":40}}`))
	}))
	t.Cleanup(srv.Close)

	cur := now
	r := &Reader{
		Store:     store,
		Keychain:  store,
		GOOS:      "darwin",
		URL:       srv.URL,
		HTTP:      srv.Client(),
		UserAgent: "ccp/test",
		Now:       func() time.Time { return cur },
		FreshFor:  FreshFor,
		HomeDir:   home,
		Env:       []string{"PATH=/usr/bin"},
	}

	got := r.ReadOtherLogins(paths)
	if len(got) != 2 {
		t.Fatalf("got %d other logins, want 2: %+v", len(got), got)
	}
	if got[0].Kind != OtherKindClaudeGlobal || got[0].Email != "global@example.invalid" || !got[0].Live {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Kind != OtherKindClaudeProfile || got[1].Profile != "default" || got[1].Email != "default@example.invalid" || !got[1].Live {
		t.Errorf("got[1] = %+v", got[1])
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("HTTP calls = %d, want 2", calls)
	}

	// Cache files live in ~/.ccp/state/usage/<key>.json (0600), never in profile folders.
	for _, key := range []string{"claude-global", "claude-profile-default"} {
		p := filepath.Join(paths.CcpDir, "state", "usage", key+".json")
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing cache %s: %v", p, err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s mode = %o, want 0600", p, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(defDir, UsageFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("wrote usage file inside profile dir: %v", err)
	}

	// Within 5m fresh window, no extra HTTP requests are made.
	got = r.ReadOtherLogins(paths)
	if atomic.LoadInt32(&calls) != 2 || got[0].Live || got[1].Live {
		t.Errorf("fresh cache failed: calls=%d got=%+v", calls, got)
	}

	// After 5m, a 429 is cached for FreshFor as well.
	cur = cur.Add(6 * time.Minute)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	got = r.ReadOtherLogins(paths)
	if atomic.LoadInt32(&calls) != 4 || !strings.Contains(got[0].Note, "429") {
		t.Errorf("429 refresh: calls=%d got=%+v", calls, got)
	}
	got = r.ReadOtherLogins(paths)
	if atomic.LoadInt32(&calls) != 4 || !strings.Contains(got[0].Note, "429") {
		t.Errorf("cached failure retried early: calls=%d got=%+v", calls, got)
	}
}

func TestReader_ReadOtherLogins_LinuxDeduplicatesGlobalSymlink(t *testing.T) {
	paths, home := otherTestPaths(t)
	defDir := paths.ProfileDir("default")
	workDir := paths.ProfileDir("work")
	os.MkdirAll(defDir, 0755)
	os.MkdirAll(workDir, 0755)
	if err := os.Symlink(defDir, paths.GlobalClaudeDir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(defDir, ".credentials.json"), cred(now.Add(-time.Minute)), 0600)
	os.WriteFile(filepath.Join(workDir, ".credentials.json"), cred(now.Add(-time.Minute)), 0600)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"g@example.invalid"}}`), 0600)
	os.WriteFile(filepath.Join(workDir, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"w@example.invalid"}}`), 0600)

	r := &Reader{
		Store:    migration.FileStore{},
		GOOS:     "linux",
		Now:      func() time.Time { return now },
		FreshFor: FreshFor,
		HomeDir:  home,
	}
	got := r.ReadOtherLogins(paths)
	if len(got) != 2 {
		t.Fatalf("got %d logins, want 2 (global + work, no duplicate default): %+v", len(got), got)
	}
	if got[0].Kind != OtherKindClaudeGlobal || got[1].Profile != "work" {
		t.Errorf("got = %+v", got)
	}
}

func TestReader_ReadOtherLogins_AgyPlainAndGuards(t *testing.T) {
	paths, home := otherTestPaths(t)
	store := &fakeCredKeychain{
		secrets:  map[string][]byte{},
		services: map[[2]string]bool{},
	}

	var calls int32
	var gotArgs, gotEnv []string
	runner := func(_ context.Context, _ string, args, env []string) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		gotArgs, gotEnv = args, env
		return []byte(agyAcctCSample), nil
	}

	cur := now
	r := &Reader{
		Store:      store,
		Keychain:   store,
		GOOS:       "darwin",
		Now:        func() time.Time { return cur },
		FreshFor:   FreshFor,
		HomeDir:    home,
		RunAgy:     runner,
		AgyTimeout: time.Second,
		Env:        []string{"PATH=/usr/bin", "SSH_CLIENT=ccp 0 0"},
	}

	// Not signed in (no gemini/antigravity in Keychain): omitted, runner never called.
	if got := r.ReadOtherLogins(paths); len(got) != 0 || calls != 0 {
		t.Fatalf("signed-out agy: got=%+v calls=%d", got, calls)
	}

	// Signed in in Keychain, with SSH_CLIENT=ccp 0 0 stripped before spawning plain agy.
	store.services[[2]string{"gemini", "antigravity"}] = true
	got := r.ReadOtherLogins(paths)
	if len(got) != 1 || got[0].Kind != OtherKindAgyPlain || got[0].Email != "" || !got[0].Live || calls != 1 {
		t.Fatalf("agy plain = %+v, calls=%d", got, calls)
	}
	wantArgs := []string{"-p", "/usage", "--output-format", "json"}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Errorf("args = %v, want %v (no --app_data_dir)", gotArgs, wantArgs)
	}
	for _, e := range gotEnv {
		if strings.HasPrefix(e, "SSH_CLIENT=") {
			t.Errorf("SSH_CLIENT=ccp 0 0 leaked into child env: %v", gotEnv)
		}
	}

	// Real SSH without ~/.gemini/antigravity-cli/antigravity-oauth-token skips spawn and shows note.
	cur = cur.Add(6 * time.Minute)
	for _, sshEnv := range [][]string{
		{"PATH=/usr/bin", "SSH_CLIENT=10.0.0.1 12345 22"},
		{"PATH=/usr/bin", "SSH_CONNECTION=10.0.0.1 12345 10.0.0.2 22"},
		{"PATH=/usr/bin", "SSH_TTY=/dev/pts/0"},
		{"PATH=/usr/bin", "SSH_CLIENT=ccp 0 0", "SSH_TTY=/dev/pts/0"},
	} {
		r.Env = sshEnv
		got = r.ReadOtherLogins(paths)
		if len(got) != 1 || calls != 1 || got[0].Usage != nil || !strings.Contains(got[0].Note, "SSH") {
			t.Errorf("env %v: got=%+v calls=%d", sshEnv, got, calls)
		}
	}

	// Real SSH WITH non-empty file token runs plain agy and caches to agy-global.json (0600).
	tokPath := filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token")
	os.MkdirAll(filepath.Dir(tokPath), 0755)
	os.WriteFile(tokPath, []byte("tok"), 0600)
	r.Env = []string{"PATH=/usr/bin", "SSH_TTY=/dev/pts/0"}
	got = r.ReadOtherLogins(paths)
	if len(got) != 1 || calls != 2 || !got[0].Live || got[0].Usage == nil {
		t.Fatalf("SSH with file token: got=%+v calls=%d", got, calls)
	}
	info, err := os.Stat(filepath.Join(paths.CcpDir, "state", "usage", "agy-global.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("agy-global.json stat: err=%v mode=%v", err, info)
	}

	// Non-darwin skips agy plain Keychain lookup.
	r.GOOS = "linux"
	if got := r.ReadOtherLogins(paths); len(got) != 0 {
		t.Errorf("linux agy plain = %+v, want empty", got)
	}
}


