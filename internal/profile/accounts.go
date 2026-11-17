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
)

var accountNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidateAccountName rejects names that are not a single safe path segment.
func ValidateAccountName(name string) error {
	if !accountNameRe.MatchString(name) {
		return fmt.Errorf("invalid account name %q: use letters, digits, '-' and '_', starting with a letter or digit", name)
	}
	return nil
}

type accountBinding struct {
	Profile string `toml:"profile"`
}

// AccountProfile returns the profile an account folder is linked to.
func AccountProfile(accountDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(accountDir, AccountFile))
	if err != nil {
		return "", err
	}
	var b accountBinding
	if err := toml.Unmarshal(data, &b); err != nil {
		return "", fmt.Errorf("invalid %s: %w", filepath.Join(accountDir, AccountFile), err)
	}
	if b.Profile == "" {
		return "", fmt.Errorf("%s names no profile", filepath.Join(accountDir, AccountFile))
	}
	return b.Profile, nil
}

// ConfigDirProfile names the profile a CLAUDE_CONFIG_DIR value activates.
// For an account folder it returns the linked profile and the account name
// (profile is "" when the binding cannot be read); for any other folder it
// returns the folder's base name and an empty account.
func ConfigDirProfile(paths *config.Paths, dir string) (profileName, account string) {
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == filepath.Clean(paths.AccountsDir()) {
		account = filepath.Base(clean)
		profileName, _ = AccountProfile(clean)
		return profileName, account
	}
	return filepath.Base(clean), ""
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

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create account folder: %w", err)
	}

	bound, err := AccountProfile(dir)
	switch {
	case err == nil && bound == profileName:
		// already linked to this profile
	case err == nil && !relink:
		return "", nil, fmt.Errorf("account %q is linked to profile %q; close its Claude sessions, then rerun with --relink", account, bound)
	case err != nil && !errors.Is(err, os.ErrNotExist) && !relink:
		return "", nil, fmt.Errorf("%w; fix the file or rerun with --relink", err)
	default:
		data, err := toml.Marshal(accountBinding{Profile: profileName})
		if err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, AccountFile), data, 0644); err != nil {
			return "", nil, fmt.Errorf("failed to record account binding: %w", err)
		}
	}

	if err := seedAccountState(paths, dir, profileDir); err != nil {
		return "", nil, err
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
// the login state, the binding, Claude's per-folder daemon files, and the
// credentials file Claude uses where there is no Keychain.
func skipAccountEntry(name string) bool {
	return name == claudeStateFile || name == AccountFile || name == claudeCredentialsFile ||
		strings.HasPrefix(name, "daemon")
}

// seedAccountState writes the account's .claude.json once, from the state
// Claude Code used for the profile, without the login. It never overwrites.
func seedAccountState(paths *config.Paths, dir, profileDir string) error {
	dst := filepath.Join(dir, claudeStateFile)
	if _, err := os.Lstat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// A profile activated globally (~/.claude -> profile) keeps its state in
	// ~/.claude.json; one activated by CLAUDE_CONFIG_DIR keeps it inside.
	src := filepath.Join(profileDir, claudeStateFile)
	if global, err := filepath.EvalSymlinks(paths.GlobalClaudeDir); err == nil {
		if real, err := filepath.EvalSymlinks(profileDir); err == nil && global == real {
			src = filepath.Join(filepath.Dir(paths.GlobalClaudeDir), claudeStateFile)
		}
	}
	data, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil // Claude Code creates it on first run
	}
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", src, err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("cannot seed account state from %s: %w", src, err)
	}
	delete(state, "oauthAccount")
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("failed to seed %s: %w", dst, err)
	}
	if _, err := f.Write(append(out, '\n')); err != nil {
		f.Close()
		os.Remove(dst)
		return fmt.Errorf("failed to seed %s: %w", dst, err)
	}
	return f.Close()
}

// pruneAccountLinks removes broken links and links into another profile (left
// from before a relink). Real files are never touched.
func pruneAccountLinks(paths *config.Paths, dir, profileDir string) error {
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
		if _, err := os.Stat(link); err != nil {
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
