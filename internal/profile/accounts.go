package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// An account is an option of Activation, not a concept: a folder under
// ~/.ccp/accounts/<name>/ used as CLAUDE_CONFIG_DIR. Claude Code keys its OAuth
// login by that folder path (the Keychain service name hashes the
// CLAUDE_CONFIG_DIR string; account info is "oauthAccount" in the folder's
// .claude.json), so each account folder holds its own login while every other
// entry is a symlink into the profile it is linked to.

const (
	// AccountFile records which profile an account folder is linked to.
	AccountFile = ".ccp-account.toml"
	// claudeStateFile is Claude Code's per-folder state; it carries the login.
	claudeStateFile = ".claude.json"
	// claudeCredentialsFile holds the OAuth token where there is no Keychain
	// (Linux); linking it would share one login across accounts.
	claudeCredentialsFile = ".credentials.json"

	// HarnessClaude is the default account harness (Claude Code).
	HarnessClaude = "claude"
	// HarnessAgy is the Antigravity CLI account harness (file token mode).
	HarnessAgy = "agy"
)

// DefaultHarness returns h, or HarnessClaude when h is empty.
func DefaultHarness(h string) string {
	if h == "" {
		return HarnessClaude
	}
	return h
}

// NormalizeHarness returns the canonical harness name ("claude" or "agy"),
// treating "" as "claude".
func NormalizeHarness(h string) (string, error) {
	switch h {
	case "", HarnessClaude:
		return HarnessClaude, nil
	case HarnessAgy:
		return HarnessAgy, nil
	default:
		return "", fmt.Errorf("unknown harness %q (valid: %s, %s)", h, HarnessClaude, HarnessAgy)
	}
}

var accountNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// AutoAccount is the --account value that picks the account with the most
// plan limit left; no account folder may take the name.
const AutoAccount = "auto"

// validateBoundProfile rejects a profile name an account cannot be linked to:
// empty, the shared data folder, or anything but a single path segment.
func validateBoundProfile(name string) error {
	if name == "" || name == "shared" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return fmt.Errorf("invalid profile name %q for an account", name)
	}
	return nil
}

// ValidateAccountName rejects names that are not a single safe path segment.
func ValidateAccountName(name string) error {
	if !accountNameRe.MatchString(name) {
		return fmt.Errorf("invalid account name %q: use letters, digits, '-' and '_', starting with a letter or digit", name)
	}
	if name == AutoAccount {
		return fmt.Errorf("account name %q is reserved: --account auto picks an account", name)
	}
	return nil
}

// ValidatePoolName applies the account name rule to a pool name: one safe
// segment, never "auto".
func ValidatePoolName(name string) error {
	if !accountNameRe.MatchString(name) {
		return fmt.Errorf("invalid pool name %q: use letters, digits, '-' and '_', starting with a letter or digit", name)
	}
	if name == AutoAccount {
		return fmt.Errorf("pool name %q is reserved", name)
	}
	return nil
}

// ParseAutoAccount reads an --account value: "auto" picks among the accounts
// linked to the profile, "auto:<pool>" among the accounts of a pool whatever
// profile they are linked to. Account names cannot hold ':', so any other
// value is a plain account name (isAuto false).
func ParseAutoAccount(v string) (isAuto bool, pool string, err error) {
	if v == AutoAccount {
		return true, "", nil
	}
	rest, ok := strings.CutPrefix(v, AutoAccount+":")
	if !ok {
		return false, "", nil
	}
	if rest == "" {
		return true, "", fmt.Errorf("--account auto: needs a pool name, as in auto:<pool>")
	}
	if err := ValidatePoolName(rest); err != nil {
		return true, "", err
	}
	return true, rest, nil
}

type accountBinding struct {
	Profile string `toml:"profile"`
	// Harness is "agy" for Antigravity CLI accounts; absent means "claude".
	Harness string `toml:"harness,omitempty"`
	// MigratedFrom is the login 'ccp migrate --accounts' moved into this
	// account (a Keychain service or a credentials file). Only that login may
	// be finished on a later run; any other is left alone.
	MigratedFrom string `toml:"migrated_from,omitempty"`
	// Pool groups accounts for --account auto:<pool>, across profiles; ""
	// means none.
	Pool string `toml:"pool,omitempty"`
}

func readBinding(accountDir string) (accountBinding, error) {
	return readBindingPool(accountDir, true)
}

// readBindingPool reads a binding; with checkPool false an invalid pool value
// is not an error, so SetAccountPool can repair a binding whose only fault is
// its pool.
func readBindingPool(accountDir string, checkPool bool) (accountBinding, error) {
	var b accountBinding
	path := filepath.Join(accountDir, AccountFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := toml.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("invalid %s: %w", path, err)
	}
	harness, err := NormalizeHarness(b.Harness)
	if err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	b.Harness = harness
	if b.Profile == "" {
		return b, fmt.Errorf("%s names no profile", path)
	}
	if err := validateBoundProfile(b.Profile); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if b.Pool != "" && checkPool {
		if err := ValidatePoolName(b.Pool); err != nil {
			return b, fmt.Errorf("%s: %w", path, err)
		}
	}
	return b, nil
}

// OnlyPoolInvalid reports whether the binding in accountDir would read
// cleanly but for its pool value ('ccp account pool <name> --clear' repairs
// that one).
func OnlyPoolInvalid(accountDir string) bool {
	if _, err := readBinding(accountDir); err == nil {
		return false
	}
	_, err := readBindingPool(accountDir, false)
	return err == nil
}

// rebound returns the binding to write when an account is linked to
// profileName: the existing one carried forward (pool, migrated_from) with
// only the profile and harness changed. A pool that does not validate (from a
// corrupt file being repaired with --relink) is dropped, or the rewritten
// binding would be unreadable again.
func rebound(existing accountBinding, profileName, harness string) accountBinding {
	b := existing
	b.Profile, b.Harness = profileName, harness
	if b.Pool != "" && ValidatePoolName(b.Pool) != nil {
		b.Pool = ""
	}
	return b
}

func writeAccountBinding(accountDir string, b accountBinding) error {
	if b.Harness == HarnessClaude {
		b.Harness = ""
	}
	data, err := toml.Marshal(b)
	if err != nil {
		return err
	}
	return writeFileAtomicMode(filepath.Join(accountDir, AccountFile), data, 0644)
}

// AccountProfile returns the profile an account folder is linked to. A
// binding naming something that cannot be a profile ("shared", "../hub") is
// an error, like an unreadable one.
func AccountProfile(accountDir string) (string, error) {
	b, err := readBinding(accountDir)
	if err != nil {
		return "", err
	}
	return b.Profile, nil
}

// AccountHarness returns the account's harness ("claude" or "agy").
func AccountHarness(accountDir string) (string, error) {
	b, err := readBinding(accountDir)
	if err != nil {
		return "", err
	}
	return b.Harness, nil
}

// AccountInfo returns the bound profile and normalized harness ("claude" or "agy").
func AccountInfo(accountDir string) (profileName, harness string, err error) {
	b, err := readBinding(accountDir)
	if err != nil {
		return "", "", err
	}
	return b.Profile, b.Harness, nil
}

// SetAccountPool records (or, with "", clears) the account's pool, keeping
// the rest of its binding. It also repairs a binding whose only fault is an
// invalid pool value. Callers serialize it with the account's relink lock.
func SetAccountPool(accountDir, pool string) error {
	if pool != "" {
		if err := ValidatePoolName(pool); err != nil {
			return err
		}
	}
	b, err := readBindingPool(accountDir, false)
	if err != nil {
		return err
	}
	b.Pool = pool
	return writeAccountBinding(accountDir, b)
}

// AccountMigratedFrom returns the login recorded as moved into the account,
// or "" when none was.
func AccountMigratedFrom(accountDir string) (string, error) {
	b, err := readBinding(accountDir)
	return b.MigratedFrom, err
}

// SetAccountMigratedFrom records (or, with "", clears) the login moved into
// the account, keeping its binding.
func SetAccountMigratedFrom(accountDir, key string) error {
	b, err := readBinding(accountDir)
	if err != nil {
		return err
	}
	b.MigratedFrom = key
	return writeAccountBinding(accountDir, b)
}

// ValidateProfileName rejects a name that cannot be a profile folder: empty,
// the shared data folder, or anything but a single path segment.
func ValidateProfileName(name string) error {
	return validateBoundProfile(name)
}

// ConfigDirProfile names the profile a CLAUDE_CONFIG_DIR value activates.
// For an account folder it returns the linked profile and the account name
// (profile is "" when the binding cannot be read); for any other folder it
// returns the folder's base name and an empty account.
func ConfigDirProfile(paths *config.Paths, dir string) (profileName, account string) {
	clean := absClean(dir)
	if filepath.Dir(clean) == absClean(paths.AccountsDir()) {
		account = filepath.Base(clean)
		profileName, _ = AccountProfile(clean)
		return profileName, account
	}
	return filepath.Base(clean), ""
}

// CcpConfigDirProfile names the ccp profile a config folder belongs to: a
// profile folder itself, or an account folder (its bound profile). ok is
// false for any folder outside ~/.ccp/profiles and ~/.ccp/accounts.
func CcpConfigDirProfile(paths *config.Paths, dir string) (profileName string, ok bool) {
	clean := absClean(dir)
	switch filepath.Dir(clean) {
	case absClean(paths.AccountsDir()):
		name, err := AccountProfile(clean)
		return name, err == nil
	case absClean(paths.ProfilesDir):
		if name := filepath.Base(clean); name != "shared" {
			return name, true
		}
	}
	return "", false
}

// InsideCcp reports whether dir is ~/.ccp or inside it (symlinks resolved
// where they exist).
func InsideCcp(paths *config.Paths, dir string) bool {
	// Resolve the deepest part that exists, so a folder not created yet
	// compares the same way (/var vs /private/var on macOS).
	resolve := func(p string) string {
		p = absClean(p)
		rest := ""
		for {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				return filepath.Join(real, rest)
			}
			parent := filepath.Dir(p)
			if parent == p {
				return filepath.Join(p, rest)
			}
			rest = filepath.Join(filepath.Base(p), rest)
			p = parent
		}
	}
	// Lexical or resolved: a real ~/.ccp whose profiles/ or accounts/ is a link
	// into a dotfiles repo still holds those folders, though their resolved
	// path is elsewhere.
	under := func(d, root string) bool {
		return d == root || strings.HasPrefix(d, root+string(filepath.Separator))
	}
	for _, d := range []string{absClean(dir), resolve(dir)} {
		for _, root := range []string{
			absClean(paths.CcpDir), resolve(paths.CcpDir),
			resolve(paths.ProfilesDir), resolve(paths.AccountsDir()),
		} {
			if under(d, root) {
				return true
			}
		}
	}
	return false
}

// Account is one folder under ~/.ccp/accounts.
type Account struct {
	Name    string
	Dir     string // absolute, clean
	Profile string // bound profile; "" when Err is set
	Harness string // "claude" or "agy"; "" when Err is set
	Pool    string // "" when none or when Err is set
	Err     error  // the binding could not be read
}

// ReadAccount returns the account folder of that name with its binding.
func ReadAccount(paths *config.Paths, name string) Account {
	a := Account{Name: name, Dir: absClean(paths.AccountDir(name))}
	b, err := readBinding(a.Dir)
	if err != nil {
		a.Err = err
		return a
	}
	a.Profile, a.Harness, a.Pool = b.Profile, b.Harness, b.Pool
	return a
}

// ListAccounts returns every account folder, sorted by name.
func ListAccounts(paths *config.Paths) ([]Account, error) {
	entries, err := os.ReadDir(paths.AccountsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Account
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, ReadAccount(paths, e.Name()))
	}
	return out, nil
}

// AccountsBoundTo returns the accounts linked to profileName.
func AccountsBoundTo(paths *config.Paths, profileName string) ([]Account, error) {
	all, err := ListAccounts(paths)
	if err != nil {
		return nil, err
	}
	var out []Account
	for _, a := range all {
		if a.Err == nil && a.Profile == profileName {
			out = append(out, a)
		}
	}
	return out, nil
}

// AccountsInPool returns the readable accounts of a pool, whatever profile
// they are linked to.
func AccountsInPool(paths *config.Paths, pool string) ([]Account, error) {
	all, err := ListAccounts(paths)
	if err != nil {
		return nil, err
	}
	var out []Account
	for _, a := range all {
		if a.Err == nil && pool != "" && a.Pool == pool {
			out = append(out, a)
		}
	}
	return out, nil
}

// RefreshAccounts relinks the accounts bound to profileName after a command
// changed the profile's folder, whichever profile that is (not only the
// active one): an account used through mise, direnv or a shell export never
// reruns --account. Entries an account holds as real files are reported by
// the commands that run per account (run/session/env) and by follow, so only
// failures are printed here.
func RefreshAccounts(paths *config.Paths, profileName string) {
	if _, err := RelinkAccounts(paths, profileName, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not relink accounts of '%s': %v\n", profileName, err)
	}
}

// RelinkAccounts brings every account linked to profileName up to date with
// the profile's entries, after a command changed them. With newName set the
// accounts are moved to that name (a profile rename).
func RelinkAccounts(paths *config.Paths, profileName, newName string) (warnings []string, err error) {
	accounts, err := AccountsBoundTo(paths, profileName)
	if err != nil {
		return nil, err
	}
	target, rebind := profileName, false
	if newName != "" {
		target, rebind = newName, true
	}
	var errs []error
	for _, a := range accounts {
		if a.Harness == HarnessAgy && newName == "" {
			continue
		}
		_, w, err := LinkAccount(paths, a.Name, target, rebind)
		warnings = append(warnings, w...)
		if err != nil {
			errs = append(errs, fmt.Errorf("account %q: %w", a.Name, err))
		}
	}
	return warnings, errors.Join(errs...)
}

// SamePath reports whether two paths name the same location once made
// absolute and clean (a relative CCP_DIR, a trailing slash).
func SamePath(a, b string) bool {
	return absClean(a) == absClean(b)
}

// SameDir reports whether two folder paths are the same: equal once absolute
// and clean, or resolving to the same folder through symlinks.
func SameDir(a, b string) bool {
	if SamePath(a, b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// LinkAccount prepares the account folder for running profileName and returns
// it as an absolute, clean path: that exact string is what Claude Code hashes
// to find the login, so callers must use it verbatim as CLAUDE_CONFIG_DIR.
//
// The folder keeps its own .claude.json (seeded once from the profile's state
// without "oauthAccount") and links every other profile entry. An account
// linked to another profile is refused unless relink is set: a running Claude
// session would see its files change under it. Entries that exist in the
// account folder as real files are left alone and reported as warnings.
func LinkAccount(paths *config.Paths, account, profileName string, relink bool) (dir string, warnings []string, err error) {
	if err := ValidateAccountName(account); err != nil {
		return "", nil, err
	}
	if err := validateBoundProfile(profileName); err != nil {
		return "", nil, err
	}
	profileDir, err := filepath.Abs(paths.ProfileDir(profileName))
	if err != nil {
		return "", nil, err
	}
	if info, err := os.Stat(profileDir); err != nil || !info.IsDir() {
		return "", nil, fmt.Errorf("profile not found: %s", profileName)
	}
	dir, err = filepath.Abs(paths.AccountDir(account))
	if err != nil {
		return "", nil, err
	}
	dir = filepath.Clean(dir)

	// Every check runs before the first write, so a refusal or a corrupt
	// state file leaves no folder behind.
	writeBinding := false
	existing, err := readBinding(dir)
	if existing.Harness == HarnessAgy || (err != nil && !errors.Is(err, os.ErrNotExist) && IsAgyAccountDir(account, dir)) {
		d, relinkErr := relinkAgyAccountDir(dir, account, profileName, existing, err, relink)
		return d, nil, relinkErr
	}
	bound := existing.Profile
	switch {
	case err == nil && bound == profileName:
		// already linked to this profile
	case err == nil && !relink:
		return "", nil, fmt.Errorf("account %q is linked to profile %q; close its Claude sessions, then rerun with --relink", account, bound)
	case err != nil && !errors.Is(err, os.ErrNotExist) && !relink:
		return "", nil, fmt.Errorf("%w; fix the file or rerun with --relink", err)
	default:
		writeBinding = true
	}
	seed, seedWarning, err := accountSeed(paths, dir, profileDir)
	if err != nil {
		return "", nil, err
	}
	if seedWarning != "" {
		warnings = append(warnings, seedWarning)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create account folder: %w", err)
	}
	if writeBinding {
		if err := writeAccountBinding(dir, rebound(existing, profileName, HarnessClaude)); err != nil {
			return "", nil, fmt.Errorf("failed to record account binding: %w", err)
		}
	}
	// The login files must be the account's own: a link there (made by hand or
	// an older version) would read or write another folder's login.
	for _, name := range []string{claudeStateFile, claudeCredentialsFile} {
		p := filepath.Join(dir, name)
		if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(p); err != nil {
				return "", nil, fmt.Errorf("failed to remove link %s: %w", p, err)
			}
			warnings = append(warnings, fmt.Sprintf("removed link %s in account %q: an account keeps its own login files", name, account))
		}
	}
	if seed != nil {
		if err := writeSeed(filepath.Join(dir, claudeStateFile), seed); err != nil {
			return "", nil, err
		}
	}

	if err := pruneAccountLinks(paths, dir, profileDir); err != nil {
		return "", nil, err
	}

	entries, err := os.ReadDir(profileDir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read profile: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if skipAccountEntry(name) {
			continue
		}
		target := filepath.Join(profileDir, name)
		link := filepath.Join(dir, name)
		info, err := os.Lstat(link)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return "", nil, err
		case info.Mode()&os.ModeSymlink == 0:
			warnings = append(warnings, fmt.Sprintf("%s is a real file in account %q, not a link to profile %q; left as is", name, account, profileName))
			continue
		default:
			if cur, _ := os.Readlink(link); cur == target {
				continue
			}
			if err := os.Remove(link); err != nil {
				return "", nil, err
			}
		}
		if err := os.Symlink(target, link); err != nil {
			return "", nil, fmt.Errorf("failed to link %s: %w", name, err)
		}
	}
	return dir, warnings, nil
}

// skipAccountEntry reports profile entries an account folder must own itself:
//   - the login state and its copies: .claude.json*, backups/ (Claude Code's
//     .claude.json backups carry oauthAccount), and .config.json, which Claude
//     Code 2.1 prefers over .claude.json when it is not empty;
//   - the credentials file Claude uses where there is no Keychain, and its
//     write lock (.storage-write*);
//   - per-folder runtime state: daemon*, jobs/, *.lock;
//   - ccp's own per-folder files (.ccp-*: the binding, usage, last use).
func skipAccountEntry(name string) bool {
	switch name {
	case "backups", "jobs", ".config.json", AccountFile, claudeCredentialsFile:
		return true
	}
	return strings.HasPrefix(name, claudeStateFile) || strings.HasPrefix(name, "daemon") || strings.HasPrefix(name, ".ccp-") ||
		strings.HasPrefix(name, ".storage-write") || strings.HasSuffix(name, ".lock")
}

// accountSeed returns the account's first .claude.json: the state Claude
// Code used for the profile, without the login. It returns nil when the
// account already has one or there is nothing to seed from, and an error for a
// state file that is not JSON. It writes nothing.
func accountSeed(paths *config.Paths, dir, profileDir string) ([]byte, string, error) {
	// A link in its place is removed before seeding, so it does not count.
	if info, err := os.Lstat(filepath.Join(dir, claudeStateFile)); err == nil && info.Mode()&os.ModeSymlink == 0 {
		return nil, "", nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	src := ProfileStatePath(paths, profileDir)
	// Claude Code 2.1 prefers a non-empty .config.json next to .claude.json;
	// ccp does not read it, so the account starts without that state.
	warning := ""
	if cfg := filepath.Join(filepath.Dir(src), ".config.json"); NonEmptyConfigJSON(cfg) {
		warning = fmt.Sprintf("%s holds Claude Code state that ccp does not copy; the account is seeded from %s only", cfg, src)
	}
	data, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil, warning, nil // Claude Code creates it on first run
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to read %s: %w", src, err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, "", fmt.Errorf("cannot seed account state from %s: %w", src, err)
	}
	if state == nil { // a top-level null
		state = map[string]json.RawMessage{}
	}
	delete(state, "oauthAccount")
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append(out, '\n'), warning, nil
}

// NonEmptyConfigJSON reports whether path is a .config.json Claude Code would
// read instead of .claude.json: present and not just {}, null or blank.
func NonEmptyConfigJSON(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	switch strings.Join(strings.Fields(string(data)), "") {
	case "", "{}", "null":
		return false
	}
	return true
}

// writeSeed creates the account's .claude.json (0600) atomically: the data is
// written to a temp file and hard-linked into place, so a crash or a Claude
// Code starting in parallel never sees an empty or partial file, and an
// existing file is never overwritten.
func writeSeed(dst string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".claude.json.ccp-seed-*")
	if err != nil {
		return fmt.Errorf("failed to seed %s: %w", dst, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to seed %s: %w", dst, err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), dst); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("failed to seed %s: %w", dst, err)
	}
	return nil
}

// pruneAccountLinks removes broken links, links into another profile (left
// from before a relink), and links to entries an account must own itself
// (such as backups/, linked by older versions). Real files are never touched.
func pruneAccountLinks(paths *config.Paths, dir, profileDir string) error {
	if h, err := AccountHarness(dir); err == nil && h == HarnessAgy {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	profilesDir, _ := filepath.Abs(paths.ProfilesDir)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		link := filepath.Join(dir, e.Name())
		stale := false
		if skipAccountEntry(e.Name()) && e.Name() != AccountFile {
			stale = true
		} else if _, err := os.Stat(link); err != nil {
			stale = true
		} else if target, err := os.Readlink(link); err == nil && filepath.IsAbs(target) {
			parent := filepath.Dir(target)
			stale = filepath.Dir(parent) == profilesDir && parent != profileDir
		}
		if stale {
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("failed to remove stale link %s: %w", link, err)
			}
		}
	}
	return nil
}
