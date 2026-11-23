package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// activationProfile returns the profile a command activates: the one named,
// or with --account and no name, the account's own profile (for "auto" and
// "auto:<pool>", the current profile: CLAUDE_CONFIG_DIR, else ~/.claude). An
// agy account whose profile is gone runs on the current profile: for agy the
// binding only names the default profile.
func activationProfile(paths *config.Paths, name, account string) (string, error) {
	mgr := profile.NewManager(paths)
	if name != "" {
		if !mgr.Exists(name) {
			return "", fmt.Errorf("profile not found: %s", name)
		}
		return name, nil
	}
	if account == "" {
		return "", fmt.Errorf("profile name required (or --account)")
	}
	isAuto, _, err := profile.ParseAutoAccount(account)
	if err != nil {
		return "", err
	}
	current := func() (string, error) {
		p, err := mgr.GetCurrent()
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", fmt.Errorf("no current profile for --account %s; name one: ccp run <profile> --account %s", account, account)
		}
		return p.Name, nil
	}
	if isAuto {
		return current()
	}
	if err := profile.ValidateAccountName(account); err != nil {
		return "", err
	}
	bound, harness, err := profile.AccountInfo(paths.AccountDir(account))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("account %q does not exist yet; name its profile the first time: ccp run <profile> --account %s", account, account)
	}
	if err != nil {
		return "", err
	}
	if harness == profile.HarnessAgy && !mgr.Exists(bound) {
		p, err := mgr.GetCurrent()
		if err != nil || p == nil {
			return "", fmt.Errorf("account %q defaults to profile %q, which no longer exists; name one: ccp run <profile> --account %s", account, bound, account)
		}
		return p.Name, nil
	}
	return bound, nil
}

// chooseAccount resolves --account auto or auto:<pool> for env and session
// (and for ccp run, every case but pooled Claude accounts: see
// chooseRunAccount), and reports the pick on stderr. Plain auto picks among
// the accounts linked to profileName with the matching harness. auto:<pool>
// picks among the pool's accounts with that harness: here only those already
// linked to profileName, since env and session never relink across profiles,
// unless anyProfile is set (ccp run with agy accounts, which run on any
// profile without a relink). Any other value is returned as is.
func chooseAccount(paths *config.Paths, profileName, account string, relink bool, harness string) (string, error) {
	return pickAccount(paths, profileName, account, relink, harness, false)
}

func pickAccount(paths *config.Paths, profileName, account string, relink bool, harness string, anyProfile bool) (string, error) {
	name, err := pickAccountFrom(paths, profileName, account, relink, harness, anyProfile)
	if _, pool, _ := profile.ParseAutoAccount(account); pool != "" {
		err = withUnreadable(paths, err)
	}
	return name, err
}

func pickAccountFrom(paths *config.Paths, profileName, account string, relink bool, harness string, anyProfile bool) (string, error) {
	isAuto, pool, err := profile.ParseAutoAccount(account)
	if err != nil {
		return "", err
	}
	if !isAuto {
		return account, nil
	}
	if relink {
		return "", fmt.Errorf("--relink needs a named account, not --account %s", account)
	}
	harness = profile.DefaultHarness(harness)
	var candidates []profile.Account
	if pool == "" {
		bound, err := profile.AccountsBoundTo(paths, profileName)
		if err != nil {
			return "", err
		}
		candidates = withHarness(bound, harness)
		if len(candidates) == 0 {
			if harness == profile.HarnessAgy {
				return "", fmt.Errorf("%w: no agy account is linked to profile %q; add one with 'ccp account add <name> %s --harness agy'", errNoUsableAccount, profileName, profileName)
			}
			return "", fmt.Errorf("%w: no account is linked to profile %q; add one with 'ccp account add <name> %s' and /login in it", errNoUsableAccount, profileName, profileName)
		}
	} else {
		members, err := profile.AccountsInPool(paths, pool)
		if err != nil {
			return "", err
		}
		members = withHarness(members, harness)
		if anyProfile {
			candidates = members
		} else {
			for _, a := range members {
				if a.Profile == profileName {
					candidates = append(candidates, a)
				}
			}
		}
		if len(candidates) == 0 {
			switch {
			case len(members) == 0:
				return "", fmt.Errorf("%w: pool %q has no %s account; add one with 'ccp account pool <name> %s'", errNoUsableAccount, pool, harness, pool)
			default:
				return "", fmt.Errorf("%w: no account of pool %q is linked to profile %q, and env and session never relink across profiles; use 'ccp run --account auto:%s'", errNoUsableAccount, pool, profileName, pool)
			}
		}
	}
	statuses, err := readAccountUsage(candidates)
	if err != nil {
		return "", err
	}
	picked, reason, err := accountusage.Pick(statuses, time.Now())
	if err != nil {
		where := fmt.Sprintf("profile %q", profileName)
		if pool != "" {
			where = fmt.Sprintf("pool %q", pool)
		}
		if harness == profile.HarnessAgy {
			return "", fmt.Errorf("%w: %w (%s); sign in once with 'ccp run --account <name>'", errNoUsableAccount, err, where)
		}
		return "", fmt.Errorf("%w: %w (%s); log in with 'ccp run --account <name>' and /login", errNoUsableAccount, err, where)
	}
	if pool != "" {
		reason = "pool " + pool + ", " + reason
	}
	fmt.Fprintf(os.Stderr, "ccp: account %s (%s)\n", picked.Name, reason)
	return picked.Name, nil
}

// withHarness keeps the accounts of one harness.
func withHarness(accounts []profile.Account, harness string) []profile.Account {
	var out []profile.Account
	for _, a := range accounts {
		if profile.DefaultHarness(a.Harness) == harness {
			out = append(out, a)
		}
	}
	return out
}

// readAccountUsage reads the login and usage of each account; tests replace
// it (or newAccountUsageReader) so no Keychain or network is touched.
var readAccountUsage = func(accounts []profile.Account) ([]accountusage.Status, error) {
	reader, err := newAccountUsageReader()
	if err != nil {
		return nil, err
	}
	return reader.Read(accounts), nil
}

// refuseAgyAccountForEnvOrSession rejects agy accounts in commands that only
// export CLAUDE_CONFIG_DIR (mise/direnv/shell cannot pass --app_data_dir).
func refuseAgyAccountForEnvOrSession(paths *config.Paths, account string) error {
	if isAuto, _, _ := profile.ParseAutoAccount(account); account == "" || isAuto {
		return nil
	}
	if profile.IsAgyAccountDir(account, paths.AccountDir(account)) {
		return fmt.Errorf("agy accounts need a command-line flag; start them with ccp run --account %s -- agy ...", account)
	}
	return nil
}

// errNoUsableAccount marks the --account auto and auto:<pool> failures where
// no candidate account can run: none linked to the profile (or in the pool),
// none logged in, or every pooled one busy or lost. run --or-profile falls
// back to the profile folder on it.
var errNoUsableAccount = errors.New("no usable account")

// currentAccountDir returns CLAUDE_CONFIG_DIR when it is an existing account
// folder (a session already started on an account), else "".
func currentAccountDir(paths *config.Paths) string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) != filepath.Clean(paths.AccountsDir()) {
		return ""
	}
	if _, err := profile.AccountProfile(dir); err != nil {
		return ""
	}
	return dir
}

// markAccountUsed records that an account was started, for the
// least-recently-used fallback of --account auto. A failure only warns.
func markAccountUsed(dir string) {
	if err := accountusage.Touch(dir, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record the account's last use: %v\n", err)
	}
}
