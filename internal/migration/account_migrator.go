package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// AccountMigrator moves each profile's Claude Code login into the account
// folder of the same name (profile "default" -> account "default"). It is
// opt-in (ccp migrate --accounts): moving the default login logs out plain
// "claude".
type AccountMigrator struct {
	paths *config.Paths
	goos  string
	store CredentialStore
	procs ProcessLister
}

// NewAccountMigrator returns a migrator for this machine's credential store.
func NewAccountMigrator(paths *config.Paths) (*AccountMigrator, error) {
	m := &AccountMigrator{paths: paths, goos: runtime.GOOS}
	switch runtime.GOOS {
	case "darwin":
		m.store = NewKeychainStore()
		m.procs = newSysctlLister()
	case "linux":
		m.store = FileStore{}
		m.procs = procLister{root: "/proc"}
	default:
		return nil, fmt.Errorf("moving logins is not supported on %s", runtime.GOOS)
	}
	return m, nil
}

// NewAccountMigratorWith returns a migrator over the given credential store
// and process lister, naming logins the way Claude Code does on goos.
func NewAccountMigratorWith(paths *config.Paths, goos string, store CredentialStore, procs ProcessLister) *AccountMigrator {
	return &AccountMigrator{paths: paths, goos: goos, store: store, procs: procs}
}

// Account migration outcomes.
const (
	AccountMoved     = "moved"
	AccountWouldMove = "would move"
	AccountSkipped   = "skipped"
	AccountRefused   = "refused"
	AccountFailed    = "failed"
)

// AccountMigrationResult is the outcome for one profile.
type AccountMigrationResult struct {
	Profile  string
	Status   string
	Reason   string
	Global   bool // the login moved (or would move) was the default ~/.claude one
	Warnings []string
}

type loginSource struct {
	key       string // Keychain service or credentials file
	stateFile string // .claude.json holding oauthAccount
	global    bool
}

// Migrate moves every profile's login, or only plans it when dryRun is set.
func (m *AccountMigrator) Migrate(dryRun bool) ([]AccountMigrationResult, error) {
	profiles, err := profile.NewManager(m.paths).List()
	if err != nil {
		return nil, err
	}
	var procs []ClaudeProcess
	var procsErr error
	listed := false
	listProcs := func() ([]ClaudeProcess, error) {
		if !listed {
			procs, procsErr = m.procs.ClaudeProcesses()
			listed = true
		}
		return procs, procsErr
	}
	var results []AccountMigrationResult
	for _, p := range profiles {
		r, err := m.migrateOne(p.Name, dryRun, listProcs)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// migrateOne moves one profile's login. Every check runs before the first
// write, so a skipped or refused profile is left exactly as it was.
func (m *AccountMigrator) migrateOne(name string, dryRun bool, listProcs func() ([]ClaudeProcess, error)) (AccountMigrationResult, error) {
	r := AccountMigrationResult{Profile: name}
	done := func(status, reason string) (AccountMigrationResult, error) {
		r.Status, r.Reason = status, reason
		return r, nil
	}
	if err := profile.ValidateAccountName(name); err != nil {
		return done(AccountSkipped, err.Error())
	}
	dir, err := filepath.Abs(m.paths.AccountDir(name))
	if err != nil {
		return r, err
	}
	dir = filepath.Clean(dir)
	target := m.credentialKey(dir, false)
	targetSecret, _, targetFound, err := m.store.Get(target)
	if err != nil {
		return done(AccountFailed, err.Error())
	}

	src, warn, err := m.findSource(name)
	r.Warnings = append(r.Warnings, warn...)
	if err != nil {
		return done(AccountFailed, err.Error())
	}
	if src == nil {
		if targetFound {
			return done(AccountSkipped, "already migrated")
		}
		return done(AccountSkipped, "no login")
	}
	r.Global = src.global

	// A target already holding the same secret is a move that stopped
	// between writing the target and deleting the source: finish it.
	resume := false
	if targetFound {
		srcSecret, _, _, err := m.store.Get(src.key)
		if err != nil {
			return done(AccountFailed, err.Error())
		}
		if !bytes.Equal(srcSecret, targetSecret) {
			return done(AccountRefused, fmt.Sprintf("both %s and %s hold a login; remove one by hand", m.describe(src.key), m.describe(target)))
		}
		resume = true
	}

	if bound, err := profile.AccountProfile(dir); err == nil && bound != name {
		return done(AccountRefused, fmt.Sprintf("account %q is linked to profile %q", name, bound))
	}
	if _, _, err := readJSONKey(src.stateFile, "oauthAccount"); err != nil {
		return done(AccountFailed, err.Error())
	}
	procs, err := listProcs()
	if err != nil {
		return r, fmt.Errorf("cannot list Claude Code processes: %w", err)
	}
	if pids := m.usingSource(procs, name, src, dir); len(pids) > 0 {
		return done(AccountRefused, fmt.Sprintf("Claude Code is running on this login or account (PID %s); close those sessions and rerun", strings.Join(pids, ", ")))
	}

	plan := fmt.Sprintf("%s -> %s", m.describe(src.key), m.describe(target))
	if resume {
		plan += " (finished an interrupted move)"
	}
	if dryRun {
		if resume {
			plan = fmt.Sprintf("%s -> %s (finish an interrupted move)", m.describe(src.key), m.describe(target))
		}
		return done(AccountWouldMove, plan)
	}

	linked, warnings, err := profile.LinkAccount(m.paths, name, name, false)
	r.Warnings = append(r.Warnings, warnings...)
	if err == nil && linked != dir {
		err = fmt.Errorf("account folder %s, expected %s", linked, dir)
	}
	if err != nil {
		return done(AccountFailed, err.Error())
	}
	moveWarn, err := m.move(src, target, dir, resume)
	r.Warnings = append(r.Warnings, moveWarn...)
	if err != nil {
		return done(AccountFailed, err.Error())
	}
	return done(AccountMoved, plan)
}

// credentialKey is where Claude Code keeps the login for a config folder.
func (m *AccountMigrator) credentialKey(dir string, global bool) string {
	if m.goos == "darwin" {
		return KeychainServiceFor(dir, global)
	}
	if global {
		return filepath.Join(m.paths.GlobalClaudeDir, ".credentials.json")
	}
	return filepath.Join(dir, ".credentials.json")
}

func (m *AccountMigrator) describe(key string) string {
	if m.goos == "darwin" {
		return fmt.Sprintf("Keychain %q", key)
	}
	return key
}

// findSource picks the profile's login: the default ~/.claude one when
// ~/.claude resolves to the profile, else the one keyed by the profile path.
// When both exist the default one wins and the other is left untouched.
func (m *AccountMigrator) findSource(name string) (*loginSource, []string, error) {
	profileDir, err := filepath.Abs(m.paths.ProfileDir(name))
	if err != nil {
		return nil, nil, err
	}
	profileDir = filepath.Clean(profileDir)
	env := &loginSource{
		key:       m.credentialKey(profileDir, false),
		stateFile: filepath.Join(profileDir, ".claude.json"),
	}
	var global *loginSource
	if m.isGlobal(profileDir) {
		global = &loginSource{
			key:       m.credentialKey(profileDir, true),
			stateFile: filepath.Join(filepath.Dir(m.paths.GlobalClaudeDir), ".claude.json"),
			global:    true,
		}
	}

	var globalFound bool
	if global != nil {
		if _, _, globalFound, err = m.store.Get(global.key); err != nil {
			return nil, nil, err
		}
	}
	// Off macOS both keys name the same file when ~/.claude is the profile.
	envFound := false
	if global == nil || m.goos == "darwin" {
		if _, _, envFound, err = m.store.Get(env.key); err != nil {
			return nil, nil, err
		}
	}
	switch {
	case globalFound && envFound:
		return global, []string{fmt.Sprintf("profile %q also has a CLAUDE_CONFIG_DIR login (%s); it is left untouched", name, m.describe(env.key))}, nil
	case globalFound:
		return global, nil, nil
	case envFound:
		return env, nil, nil
	}
	return nil, nil, nil
}

func (m *AccountMigrator) isGlobal(profileDir string) bool {
	global, err := filepath.EvalSymlinks(m.paths.GlobalClaudeDir)
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(profileDir)
	return err == nil && global == real
}

// usingSource returns the PIDs of Claude Code processes that use the source
// login's folder or the target account folder (a session there, even one
// sitting at /login, would write its own login under the move). A process whose environment cannot be read counts as using
// it: refusing is safer than moving a login under a running session.
func (m *AccountMigrator) usingSource(procs []ClaudeProcess, name string, src *loginSource, accountDir string) []string {
	profileDir, _ := filepath.Abs(m.paths.ProfileDir(name))
	profileDir = filepath.Clean(profileDir)
	realProfile, _ := filepath.EvalSymlinks(profileDir)
	resolvesToProfile := func(dir string) bool {
		if filepath.Clean(dir) == profileDir {
			return true
		}
		real, err := filepath.EvalSymlinks(dir)
		return err == nil && realProfile != "" && real == realProfile
	}
	var pids []string
	for _, p := range procs {
		match := false
		switch {
		case !p.EnvKnown:
			match = true
		case src.global:
			match = p.ConfigDir == "" || filepath.Clean(p.ConfigDir) == filepath.Clean(m.paths.GlobalClaudeDir) || resolvesToProfile(p.ConfigDir)
		default:
			match = p.ConfigDir != "" && resolvesToProfile(p.ConfigDir)
		}
		if p.EnvKnown && p.ConfigDir != "" && profile.SamePath(p.ConfigDir, accountDir) {
			match = true
		}
		if match {
			pids = append(pids, fmt.Sprint(p.PID))
		}
	}
	return pids
}

// move copies the login to target, verifies it, carries oauthAccount into the
// account's .claude.json, and only then deletes the source. A failure before
// the source is deleted rolls the target back.
func (m *AccountMigrator) move(src *loginSource, target, dir string, resume bool) (warnings []string, err error) {
	secret, acct, found, err := m.store.Get(src.key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("login %s disappeared", m.describe(src.key))
	}

	targetState := filepath.Join(dir, ".claude.json")
	prevState, prevErr := os.ReadFile(targetState)
	stateChanged := false
	rollback := func(cause error) error {
		var errs []error
		if _, err := m.store.Delete(target, acct); err != nil {
			errs = append(errs, fmt.Errorf("could not remove %s: %w", m.describe(target), err))
		}
		if stateChanged {
			var err error
			if prevErr == nil {
				err = writeFileAtomic(targetState, prevState, 0600)
			} else {
				err = os.Remove(targetState)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("could not restore %s: %w", targetState, err))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%w; rollback incomplete: %w", cause, errors.Join(errs...))
		}
		return fmt.Errorf("%w (rolled back; the login is unchanged)", cause)
	}

	if !resume {
		if err := m.store.Set(target, acct, secret); err != nil {
			return nil, rollback(fmt.Errorf("writing %s: %w", m.describe(target), err))
		}
	}
	got, _, found, err := m.store.Get(target)
	if err != nil || !found || !bytes.Equal(got, secret) {
		if err == nil {
			err = errors.New("read-back differs from the source")
		}
		return nil, rollback(fmt.Errorf("verifying %s: %w", m.describe(target), err))
	}

	oauth, hasOauth, err := readJSONKey(src.stateFile, "oauthAccount")
	if err != nil {
		return nil, rollback(err)
	}
	if hasOauth {
		if err := setJSONKey(targetState, "oauthAccount", oauth, 0600); err != nil {
			return nil, rollback(fmt.Errorf("writing oauthAccount to %s: %w", targetState, err))
		}
		stateChanged = true
	} else {
		warnings = append(warnings, fmt.Sprintf("no oauthAccount in %s; Claude Code fills it in on first run", src.stateFile))
	}

	warn, err := m.store.Delete(src.key, acct)
	if err != nil {
		return nil, rollback(fmt.Errorf("removing %s: %w", m.describe(src.key), err))
	}
	if warn != "" {
		warnings = append(warnings, warn)
	}
	if hasOauth {
		if err := setJSONKey(src.stateFile, "oauthAccount", nil, 0); err != nil {
			warnings = append(warnings, fmt.Sprintf("login moved, but oauthAccount is still in %s (%v); remove it by hand", src.stateFile, err))
		}
	}
	return warnings, nil
}

// readJSONKey returns one top-level key of a JSON object file; a missing file
// has no keys.
func readJSONKey(path, key string) (json.RawMessage, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, false, fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	v, ok := obj[key]
	return v, ok, nil
}

// setJSONKey sets (value != nil) or removes (value == nil) a top-level key of
// a JSON object file, atomically. The file keeps its mode; a new file gets
// newMode. A missing file is created only when setting.
func setJSONKey(path, key string, value json.RawMessage, newMode os.FileMode) error {
	obj := map[string]json.RawMessage{}
	mode := newMode
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("%s is not a JSON object: %w", path, err)
		}
		if obj == nil { // a top-level null
			obj = map[string]json.RawMessage{}
		}
		if info, err := os.Stat(path); err == nil && newMode == 0 {
			mode = info.Mode().Perm()
		}
	case errors.Is(err, os.ErrNotExist):
		if value == nil {
			return nil
		}
	default:
		return err
	}
	if value == nil {
		delete(obj, key)
	} else {
		obj[key] = value
	}
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(out, '\n'), mode)
}
