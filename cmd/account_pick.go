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
// or with --account and no name, the account's own profile (for "auto", the
// current profile: CLAUDE_CONFIG_DIR, else ~/.claude).
func activationProfile(paths *config.Paths, name, account string) (string, error) {
	mgr := profile.NewManager(paths)
	if name != "" {
		if !mgr.Exists(name) {
			return "", fmt.Errorf("profile not found: %s", name)
		}
		return name, nil
	}
	switch {
	case account == "":
		return "", fmt.Errorf("profile name required (or --account)")
	case account == profile.AutoAccount:
		p, err := mgr.GetCurrent()
		if err != nil {
			return "", err
		}
		if p == nil {
			return "", fmt.Errorf("no current profile for --account auto; name one: ccp run <profile> --account auto")
		}
		return p.Name, nil
	}
	if err := profile.ValidateAccountName(account); err != nil {
		return "", err
	}
	bound, err := profile.AccountProfile(paths.AccountDir(account))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("account %q does not exist yet; name its profile the first time: ccp run <profile> --account %s", account, account)
	}
	if err != nil {
		return "", err
	}
	return bound, nil
}

// chooseAccount resolves --account auto to the account linked to profileName
// with the matching harness and the most plan limit left (see
// accountusage.Pick), and reports the pick on stderr. Any other value is
// returned as is.
func chooseAccount(paths *config.Paths, profileName, account string, relink bool, harness string) (string, error) {
	if account != profile.AutoAccount {
		return account, nil
	}
	if relink {
		return "", fmt.Errorf("--relink needs a named account, not --account auto")
	}
	harness = profile.DefaultHarness(harness)
	bound, err := profile.AccountsBoundTo(paths, profileName)
	if err != nil {
		return "", err
	}
	var pool []profile.Account
	for _, a := range bound {
		if profile.DefaultHarness(a.Harness) == harness {
			pool = append(pool, a)
		}
	}
	if len(pool) == 0 {
		if harness == profile.HarnessAgy {
			return "", fmt.Errorf("%w: no agy account is linked to profile %q; add one with 'ccp account add <name> %s --harness agy'", errNoUsableAccount, profileName, profileName)
		}
		return "", fmt.Errorf("%w: no account is linked to profile %q; add one with 'ccp account add <name> %s' and /login in it", errNoUsableAccount, profileName, profileName)
	}
	reader, err := accountusage.NewReader(Version)
	if err != nil {
		return "", err
	}
	picked, reason, err := accountusage.Pick(reader.Read(pool), time.Now())
	if err != nil {
		if harness == profile.HarnessAgy {
			return "", fmt.Errorf("%w: %w (profile %q); sign in once with 'ccp run --account <name>'", errNoUsableAccount, err, profileName)
		}
		return "", fmt.Errorf("%w: %w (profile %q); log in with 'ccp run --account <name>' and /login", errNoUsableAccount, err, profileName)
	}
	fmt.Fprintf(os.Stderr, "ccp: account %s (%s)\n", picked.Name, reason)
	return picked.Name, nil
}

// refuseAgyAccountForEnvOrSession rejects agy accounts in commands that only
// export CLAUDE_CONFIG_DIR (mise/direnv/shell cannot pass --app_data_dir).
func refuseAgyAccountForEnvOrSession(paths *config.Paths, account string) error {
	if account == "" || account == profile.AutoAccount {
		return nil
	}
	if profile.IsAgyAccountDir(account, paths.AccountDir(account)) {
		return fmt.Errorf("agy accounts need a command-line flag; start them with ccp run --account %s -- agy ...", account)
	}
	return nil
}

// errNoUsableAccount marks the --account auto failures where the pool has no
// account to run: none linked to the profile, or none logged in. run
// --or-profile falls back to the profile folder on it.
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
