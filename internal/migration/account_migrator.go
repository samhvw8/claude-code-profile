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
		m.procs = psLister{run: execRunner}
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
	listed := false
	var results []AccountMigrationResult
	for _, p := range profiles {
		r := AccountMigrationResult{Profile: p.Name}
		src, warn, err := m.findSource(p.Name)
		r.Warnings = append(r.Warnings, warn...)
		if err != nil {
			r.Status, r.Reason = AccountFailed, err.Error()
			results = append(results, r)
			continue
		}
		if src == nil {
			r.Status, r.Reason = AccountSkipped, "no login"
			results = append(results, r)
			continue
		}
		r.Global = src.global
		if err := profile.ValidateAccountName(p.Name); err != nil {
			r.Status, r.Reason = AccountSkipped, err.Error()
			results = append(results, r)
			continue
		}
		dir, err := filepath.Abs(m.paths.AccountDir(p.Name))
		if err != nil {
			return nil, err
		}
		dir = filepath.Clean(dir)
		if bound, err := profile.AccountProfile(dir); err == nil && bound != p.Name {
			r.Status, r.Reason = AccountRefused, fmt.Sprintf("account %q is linked to profile %q", p.Name, bound)
			results = append(results, r)
			continue
		}
		target := m.credentialKey(dir, false)
		if _, _, found, err := m.store.Get(target); err != nil {
			r.Status, r.Reason = AccountFailed, err.Error()
			results = append(results, r)
			continue
		} else if found {
			r.Status, r.Reason = AccountSkipped, "already migrated"
			results = append(results, r)
			continue
		}
		if !listed {
			if procs, err = m.procs.ClaudeProcesses(); err != nil {
				return nil, fmt.Errorf("cannot list Claude Code processes: %w", err)
			}
			listed = true
		}
		if pids := m.usingSource(procs, p.Name, src); len(pids) > 0 {
			r.Status = AccountRefused
			r.Reason = fmt.Sprintf("Claude Code is running on this login (PID %s); close those sessions and rerun", strings.Join(pids, ", "))
			results = append(results, r)
			continue
		}
		if dryRun {
			r.Status = AccountWouldMove
			r.Reason = fmt.Sprintf("%s -> %s", m.describe(src.key), m.describe(target))
			results = append(results, r)
			continue
		}
		linked, warnings, err := profile.LinkAccount(m.paths, p.Name, p.Name, false)
		r.Warnings = append(r.Warnings, warnings...)
		if err == nil && linked != dir {
			err = fmt.Errorf("account folder %s, expected %s", linked, dir)
		}
		if err != nil {
			r.Status, r.Reason = AccountFailed, err.Error()
			results = append(results, r)
			continue
		}
		moveWarn, err := m.move(src, target, dir)
		r.Warnings = append(r.Warnings, moveWarn...)
		if err != nil {
			r.Status, r.Reason = AccountFailed, err.Error()
		} else {
			r.Status, r.Reason = AccountMoved, fmt.Sprintf("%s -> %s", m.describe(src.key), m.describe(target))
		}
		results = append(results, r)
	}
	return results, nil
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
// login's folder. A process whose environment cannot be read counts as using
// it: refusing is safer than moving a login under a running session.
func (m *AccountMigrator) usingSource(procs []ClaudeProcess, name string, src *loginSource) []string {
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
		if match {
			pids = append(pids, fmt.Sprint(p.PID))
		}
	}
	return pids
}

// move copies the login to target, verifies it, carries oauthAccount into the
// account's .claude.json, and only then deletes the source. A failure before
// the source is deleted rolls the target back.
func (m *AccountMigrator) move(src *loginSource, target, dir string) (warnings []string, err error) {
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
		if err := m.store.Delete(target); err != nil {
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

	if err := m.store.Set(target, acct, secret); err != nil {
		return nil, rollback(fmt.Errorf("writing %s: %w", m.describe(target), err))
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

	if err := m.store.Delete(src.key); err != nil {
		return nil, rollback(fmt.Errorf("removing %s: %w", m.describe(src.key), err))
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
