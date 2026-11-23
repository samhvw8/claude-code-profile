package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var newAccountUsageReader = func() (*accountusage.Reader, error) {
	return accountusage.NewReader(Version)
}

// readAccountStatuses returns every account with its login and usage, or nil
// when there are none or they cannot be read (status still prints the rest).
func readAccountStatuses(paths *config.Paths) []accountusage.Status {
	accounts, err := profile.ListAccounts(paths)
	if err != nil || len(accounts) == 0 {
		return nil
	}
	reader, err := newAccountUsageReader()
	if err != nil {
		out := make([]accountusage.Status, len(accounts))
		for i, a := range accounts {
			out[i] = accountusage.Status{Name: a.Name, Profile: a.Profile, Harness: a.Harness, Dir: a.Dir, Pool: a.Pool, Note: err.Error()}
		}
		return out
	}
	return reader.Read(accounts)
}

// readOtherLogins returns unmanaged Claude and agy logins outside ~/.ccp/accounts.
func readOtherLogins(paths *config.Paths) []accountusage.OtherLogin {
	reader, err := newAccountUsageReader()
	if err != nil {
		return nil
	}
	return reader.ReadOtherLogins(paths)
}

func percent(w *accountusage.Window, now time.Time) string {
	if v, ok := w.At(now); ok {
		return fmt.Sprintf("%.0f%%", v)
	}
	return "-"
}

func printAccounts(statuses []accountusage.Status) {
	now := time.Now()
	fmt.Println("--- Accounts ---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ACCOUNT\tHARNESS\tPROFILE\tPOOL\tEMAIL\t5H\t7D\tMODEL WEEK\tAS OF\tLAST USED")
	for _, s := range statuses {
		harness := profile.DefaultHarness(s.Harness)
		prof, email, five, week, scoped, asOf, last := s.Profile, s.Email, "-", "-", "-", "-", "never"
		if prof == "" {
			prof = "?"
		}
		pool := s.Pool
		if pool == "" {
			pool = "-"
		}
		if email == "" {
			email = "-"
		}
		if !s.LoggedIn {
			email = "(not logged in)"
		}
		if s.Usage != nil {
			five, week = percent(s.Usage.FiveHour, now), percent(s.Usage.SevenDay, now)
			var parts []string
			for _, sc := range s.Usage.Scoped {
				parts = append(parts, sc.Name+" "+percent(&sc.Window, now))
			}
			if len(parts) > 0 {
				scoped = strings.Join(parts, ", ")
			}
			asOf = accountusage.Ago(now, s.Usage.FetchedAt)
		}
		if !s.LastUsed.IsZero() {
			last = accountusage.Ago(now, s.LastUsed)
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, harness, prof, pool, email, five, week, scoped, asOf, last)
	}
	w.Flush()
	for _, s := range statuses {
		if s.LoggedIn && s.Note != "" {
			fmt.Printf("  %s: %s\n", s.Name, s.Note)
		}
	}
	fmt.Println()
}

func printOtherLogins(others []accountusage.OtherLogin) {
	now := time.Now()
	fmt.Println("--- Other logins (not accounts) ---")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  KIND\tWHERE\tEMAIL\t5H\t7D\tAS OF")
	hasClaudeProfile := false
	hasAgy := false
	for _, o := range others {
		email, five, week, asOf := o.Email, "-", "-", "-"
		if email == "" {
			email = "-"
		}
		if o.Usage != nil {
			five, week = percent(o.Usage.FiveHour, now), percent(o.Usage.SevenDay, now)
			asOf = accountusage.Ago(now, o.Usage.FetchedAt)
		}
		if o.Kind == accountusage.OtherKindClaudeProfile {
			hasClaudeProfile = true
		}
		if o.Harness == profile.HarnessAgy {
			hasAgy = true
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n", o.Kind, o.Where, email, five, week, asOf)
	}
	w.Flush()
	for _, o := range others {
		if o.Note != "" {
			fmt.Printf("  %s (%s): %s\n", o.Kind, o.Where, o.Note)
		}
	}
	if hasClaudeProfile || hasAgy {
		fmt.Println()
		if hasClaudeProfile {
			fmt.Println("  ccp migrate --accounts --dry-run previews moving profile logins into accounts (it moves every profile, including the global login)")
		}
		if hasAgy {
			fmt.Println("  add more agy logins with: ccp account add <name> --harness agy")
		}
	}
	fmt.Println()
}

type accountStatusJSON struct {
	Name     string   `json:"name"`
	Harness  string   `json:"harness"`
	Profile  string   `json:"profile"`
	Pool     string   `json:"pool,omitempty"`
	Email    string   `json:"email,omitempty"`
	LoggedIn bool     `json:"logged_in"`
	FiveHour *float64 `json:"five_hour_percent,omitempty"`
	SevenDay *float64 `json:"seven_day_percent,omitempty"`
	AsOf     string   `json:"usage_as_of,omitempty"`
	LastUsed string   `json:"last_used,omitempty"`
	Note     string   `json:"note,omitempty"`
}

func accountJSON(statuses []accountusage.Status) []accountStatusJSON {
	now := time.Now()
	out := make([]accountStatusJSON, len(statuses))
	for i, s := range statuses {
		harness := profile.DefaultHarness(s.Harness)
		j := accountStatusJSON{Name: s.Name, Harness: harness, Profile: s.Profile, Pool: s.Pool, Email: s.Email, LoggedIn: s.LoggedIn, Note: s.Note}
		if s.Usage != nil {
			if v, ok := s.Usage.FiveHour.At(now); ok {
				j.FiveHour = &v
			}
			if v, ok := s.Usage.SevenDay.At(now); ok {
				j.SevenDay = &v
			}
			j.AsOf = s.Usage.FetchedAt.Format(time.RFC3339)
		}
		if !s.LastUsed.IsZero() {
			j.LastUsed = s.LastUsed.Format(time.RFC3339)
		}
		out[i] = j
	}
	return out
}
