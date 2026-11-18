package accountusage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
