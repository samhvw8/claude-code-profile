package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/samhvw8/claude-code-profile/internal/accountusage"
	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/migration"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// relinkLockFile is the per-account lock every 'ccp run --account auto:<pool>'
// Claude launch holds from its re-check until its process has started. Its
// name starts with .ccp- and ends with .lock, so it is never linked.
const relinkLockFile = ".ccp-relink.lock"

// errNoFileLock is lockAccountDir's error where the system has no flock.
var errNoFileLock = errors.New("account locks need flock, which this system lacks")

// lockAccount takes an account's relink lock; tests replace it.
var lockAccount = lockAccountDir

// lockForLink takes the account's relink lock around a binding change by a
// caller that is not a pooled pick: named --account, plain auto, env,
// session, 'ccp account pool'. An account folder that does not exist yet has
// nothing to race on, and a system without flock goes on unlocked as before.
func lockForLink(dir string) (func(), error) {
	release, err := lockAccount(dir)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errNoFileLock) {
		return noRelease, nil
	}
	if err != nil {
		return noRelease, fmt.Errorf("cannot lock account folder %s: %w", dir, err)
	}
	return release, nil
}

// lockedActivationDir is activationDir with the account's relink lock held
// around LinkAccount, so it never interleaves with a pooled relink: under the
// lock LinkAccount re-reads the binding and refuses one linked to another
// profile unless relink is set. The caller releases the lock (ccp run right
// after the start; env and session at once).
func lockedActivationDir(paths *config.Paths, profileName, account string, accountSet, relink bool) (string, func(), error) {
	if account == "" {
		dir, err := activationDir(paths, profileName, account, accountSet, relink)
		return dir, noRelease, err
	}
	if err := profile.ValidateAccountName(account); err != nil {
		return "", noRelease, err
	}
	release, err := lockForLink(paths.AccountDir(account))
	if err != nil {
		return "", noRelease, err
	}
	dir, err := activationDir(paths, profileName, account, accountSet, relink)
	if err != nil {
		release()
		return "", noRelease, err
	}
	return dir, release, nil
}

// setPoolLocked rewrites an account's pool under its relink lock.
func setPoolLocked(paths *config.Paths, name, pool string) error {
	release, err := lockForLink(paths.AccountDir(name))
	if err != nil {
		return err
	}
	defer release()
	return profile.SetAccountPool(paths.AccountDir(name), pool)
}

// withUnreadable adds to a pool error the accounts whose binding cannot be
// read: their pool is unknown, so they may be the members that are missing.
func withUnreadable(paths *config.Paths, err error) error {
	if !errors.Is(err, errNoUsableAccount) {
		return err
	}
	all, _ := profile.ListAccounts(paths)
	var names []string
	for _, a := range all {
		if a.Err != nil {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		return err
	}
	return fmt.Errorf("%w; the binding of %s cannot be read, so their pool is unknown (see 'ccp doctor'; 'ccp account pool <name> --clear' repairs a bad pool value)", err, strings.Join(names, ", "))
}

// newProcessLister lists running Claude Code processes; tests replace it.
// nil means the system cannot tell, which counts as busy.
var newProcessLister = migration.NewProcessLister

func noRelease() {}

// chooseRunAccount resolves --account for ccp run. Pooled Claude accounts
// (auto:<pool>) go through choosePooledClaude, which may relink an account
// linked to another profile and returns a release func that must be called
// once the process has started. Everything else goes through pickAccount;
// pooled agy accounts are picked from any profile, since agy runs on any
// profile without a relink. release is never nil.
func chooseRunAccount(paths *config.Paths, profileName, account string, relink bool, harness string) (string, func(), error) {
	_, pool, err := profile.ParseAutoAccount(account)
	if err != nil {
		return "", noRelease, err
	}
	if pool == "" || harness == profile.HarnessAgy {
		name, err := pickAccount(paths, profileName, account, relink, harness, pool != "")
		return name, noRelease, err
	}
	if relink {
		return "", noRelease, fmt.Errorf("--relink needs a named account, not --account %s", account)
	}
	name, release, err := choosePooledClaude(paths, profileName, pool)
	return name, release, withUnreadable(paths, err)
}

// choosePooledClaude picks a Claude account of the pool for profileName. A
// member linked to profileName is usable as is (parallel sessions on one
// account and profile keep working). A member linked to another profile is
// usable only while no Claude Code process runs on its folder: the relink
// would change its files under that session. Busy members are dropped before
// the usage read. The pick is then locked and checked again (binding and
// processes), relinked when needed, and the lock is handed back to the caller
// to release once the process has started. A candidate that fails the
// re-check is released and the next one is tried.
func choosePooledClaude(paths *config.Paths, profileName, pool string) (string, func(), error) {
	members, err := profile.AccountsInPool(paths, pool)
	if err != nil {
		return "", noRelease, err
	}
	members = withHarness(members, profile.HarnessClaude)
	if len(members) == 0 {
		return "", noRelease, fmt.Errorf("%w: pool %q has no Claude account; add one with 'ccp account pool <name> %s'", errNoUsableAccount, pool, pool)
	}
	var candidates []profile.Account
	var procs []migration.ClaudeProcess
	var scanErr error
	scanned := false
	for _, a := range members {
		if a.Profile == profileName {
			candidates = append(candidates, a)
			continue
		}
		if !scanned {
			procs, scanErr = scanClaudeProcesses()
			scanned = true
		}
		if !accountBusy(procs, scanErr, a.Dir) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		why := "a Claude Code session runs on each"
		if scanErr != nil {
			why = "running sessions cannot be checked (" + scanErr.Error() + ")"
		}
		return "", noRelease, fmt.Errorf("%w: every account of pool %q is linked to another profile and %s", errNoUsableAccount, pool, why)
	}
	statuses, err := readAccountUsage(candidates)
	if err != nil {
		return "", noRelease, err
	}
	var lost []string
	for len(statuses) > 0 {
		picked, reason, err := accountusage.Pick(statuses, time.Now())
		if err != nil {
			return "", noRelease, fmt.Errorf("%w: %w (pool %q); log in with 'ccp run --account <name>' and /login", errNoUsableAccount, err, pool)
		}
		drop := func(why string) {
			lost = append(lost, picked.Name+": "+why)
			statuses = slices.DeleteFunc(statuses, func(s accountusage.Status) bool { return s.Name == picked.Name })
		}
		release, err := lockAccount(picked.Dir)
		if err != nil {
			drop("lock: " + err.Error())
			continue
		}
		from, why, err := claimPooled(paths, picked.Name, profileName, pool)
		if err != nil {
			why = err.Error()
		}
		if why != "" {
			release()
			drop(why)
			continue
		}
		if from != "" {
			reason = "relinked from " + from + ", " + reason
		}
		fmt.Fprintf(os.Stderr, "ccp: account %s (pool %s, %s)\n", picked.Name, pool, reason)
		return picked.Name, release, nil
	}
	return "", noRelease, fmt.Errorf("%w: no account of pool %q could be taken: %s", errNoUsableAccount, pool, strings.Join(lost, "; "))
}

// claimPooled re-checks a picked account under its lock: still in the pool,
// a readable Claude binding, and either linked to profileName or with no
// Claude Code process on its folder, in which case it is relinked to
// profileName. Either way it links the account (printing the warnings), so
// the caller does not link it again. It returns the profile it was relinked from ("" when none was
// needed), or why it can no longer be used.
func claimPooled(paths *config.Paths, name, profileName, pool string) (from, why string, err error) {
	a := profile.ReadAccount(paths, name)
	switch {
	case a.Err != nil:
		return "", "binding unreadable: " + a.Err.Error(), nil
	case a.Pool != pool:
		return "", "left the pool", nil
	case a.Harness != profile.HarnessClaude:
		return "", "not a Claude account", nil
	}
	relink := a.Profile != profileName
	if relink {
		procs, scanErr := scanClaudeProcesses()
		if accountBusy(procs, scanErr, a.Dir) {
			return "", "in use on profile " + a.Profile, nil
		}
	}
	// The one link of a pooled pick: ccp run uses the folder as linked here.
	_, warnings, err := profile.LinkAccount(paths, name, profileName, relink)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	if err != nil {
		return "", "", fmt.Errorf("link account %q to %q: %w", name, profileName, err)
	}
	if !relink {
		return "", "", nil
	}
	return a.Profile, "", nil
}

// scanClaudeProcesses lists running Claude Code processes, or says why it
// cannot.
func scanClaudeProcesses() ([]migration.ClaudeProcess, error) {
	lister := newProcessLister()
	if lister == nil {
		return nil, errors.New("no process lister on this system")
	}
	return lister.ClaudeProcesses()
}

// accountBusy reports whether a Claude Code process may be running on the
// account folder dir. It fails closed: a failed scan, or any process whose
// environment could not be read, counts as running there.
func accountBusy(procs []migration.ClaudeProcess, scanErr error, dir string) bool {
	if scanErr != nil {
		return true
	}
	for _, p := range procs {
		if !p.EnvKnown {
			return true
		}
		// A relative CLAUDE_CONFIG_DIR is relative to that process's
		// working directory, not ccp's: it cannot be compared, so it counts.
		if p.ConfigDir != "" && (!filepath.IsAbs(p.ConfigDir) || profile.SameDir(p.ConfigDir, dir)) {
			return true
		}
	}
	return false
}
