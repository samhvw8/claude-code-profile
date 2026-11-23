package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// WithEnv returns a copy of base with each KEY=VALUE entry set, replacing any
// existing entry for that key.
func WithEnv(base []string, kv ...string) []string {
	out := make([]string, len(base))
	copy(out, base)
	for _, entry := range kv {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		prefix := key + "="
		replaced := false
		for i, existing := range out {
			if strings.HasPrefix(existing, prefix) {
				out[i] = entry
				replaced = true
			}
		}
		if !replaced {
			out = append(out, entry)
		}
	}
	return out
}

const agySettingsFile = "settings.json"

// AgyAppDataDirName returns the relative --app_data_dir name under ~/.gemini
// for an agy account.
func AgyAppDataDirName(account string) string {
	return "antigravity-cli-ccp-" + account
}

// AgyAccountDataDir returns the absolute agy data directory
// (~/.gemini/antigravity-cli-ccp-<account>) for an agy account.
func AgyAccountDataDir(account string) (string, error) {
	if err := ValidateAccountName(account); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", AgyAppDataDirName(account)), nil
}

// AgyMainDataDir returns agy's main data directory (~/.gemini/antigravity-cli).
func AgyMainDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "antigravity-cli"), nil
}

// AgyTokenPathIn returns the file-mode OAuth token path for account under home.
func AgyTokenPathIn(home, account string) string {
	return filepath.Join(home, ".gemini", AgyAppDataDirName(account), "antigravity-oauth-token")
}

// AgyTokenPath returns the file-mode OAuth token path for an agy account.
func AgyTokenPath(account string) (string, error) {
	dir, err := AgyAccountDataDir(account)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "antigravity-oauth-token"), nil
}

// IsAgyAccountDir reports whether accountDir belongs to an agy account,
// including when its binding file is missing a valid profile or is corrupt TOML
// while an agy data directory exists without Claude state.
func IsAgyAccountDir(account, accountDir string) bool {
	b, err := readBinding(accountDir)
	if b.Harness == HarnessAgy {
		return true
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if dataDir, dErr := AgyAccountDataDir(account); dErr == nil {
			if info, sErr := os.Stat(dataDir); sErr == nil && info.IsDir() {
				if _, cErr := os.Lstat(filepath.Join(accountDir, claudeStateFile)); errors.Is(cErr, os.ErrNotExist) {
					return true
				}
			}
		}
	}
	return false
}

// RelinkAgyAccount ensures an agy account is bound to profileName, rewriting
// only .ccp-account.toml (never creating symlinks or .claude.json).
func RelinkAgyAccount(paths *config.Paths, account, profileName string, relink bool) (string, error) {
	if err := ValidateAccountName(account); err != nil {
		return "", err
	}
	if err := validateBoundProfile(profileName); err != nil {
		return "", err
	}
	profileDir, err := filepath.Abs(paths.ProfileDir(profileName))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(profileDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("profile not found: %s", profileName)
	}
	dir := absClean(paths.AccountDir(account))
	existing, err := readBinding(dir)
	return relinkAgyAccountDir(dir, account, profileName, existing, err, relink)
}

func relinkAgyAccountDir(dir, account, profileName string, existing accountBinding, err error, relink bool) (string, error) {
	switch {
	case err == nil && existing.Profile == profileName:
		return dir, nil
	case err != nil && !relink:
		return "", fmt.Errorf("%w; fix the file or rerun with --relink", err)
	case !relink:
		return "", fmt.Errorf("account %q is linked to profile %q; close its agy sessions, then rerun with --relink", account, existing.Profile)
	}
	if err := writeAccountBinding(dir, rebound(existing, profileName, HarnessAgy)); err != nil {
		return "", fmt.Errorf("failed to record account binding: %w", err)
	}
	return dir, nil
}

// AgyLaunch returns the CLI flag and environment variable required to run agy
// isolated to account's file-based token store. Callers must always apply both
// together: without SSH_CLIENT agy uses the shared Keychain, and without
// --app_data_dir a keyring refresh can purge a file token (agy issue #1514).
func AgyLaunch(account string) (flagArg, envEntry string) {
	return "--app_data_dir=" + AgyAppDataDirName(account), "SSH_CLIENT=ccp 0 0"
}

// CreateAgyAccount creates ~/.ccp/accounts/<account>/ (with harness = "agy")
// and ~/.gemini/antigravity-cli-ccp-<account>/, plus merged settings if the
// main agy settings.json exists. Never creates symlinks.
func CreateAgyAccount(paths *config.Paths, account, profileName string) (string, error) {
	if err := ValidateAccountName(account); err != nil {
		return "", err
	}
	if err := validateBoundProfile(profileName); err != nil {
		return "", err
	}
	profileDir, err := filepath.Abs(paths.ProfileDir(profileName))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(profileDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("profile not found: %s", profileName)
	}
	dir := absClean(paths.AccountDir(account))
	if _, err := os.Stat(dir); err == nil {
		bound, _ := AccountProfile(dir)
		return "", fmt.Errorf("account %q already exists (profile %q)", account, bound)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	dataDir, err := AgyAccountDataDir(account)
	if err != nil {
		return "", err
	}
	// Validate and compute merged settings before creating any folder.
	merged, hasSettings, err := computeAgySettings(account)
	if err != nil {
		return "", err
	}

	_, dataDirStatErr := os.Stat(dataDir)
	dataDirExisted := dataDirStatErr == nil
	committed := false
	createdDir := false
	createdDataDir := false
	defer func() {
		if committed {
			return
		}
		if createdDir {
			os.RemoveAll(dir)
		}
		if createdDataDir {
			os.RemoveAll(dataDir)
		}
	}()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create account folder: %w", err)
	}
	createdDir = true
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create agy data folder: %w", err)
	}
	createdDataDir = !dataDirExisted
	if err := writeAccountBinding(dir, accountBinding{Profile: profileName, Harness: HarnessAgy}); err != nil {
		return "", fmt.Errorf("failed to record account binding: %w", err)
	}
	if hasSettings {
		dst := filepath.Join(dataDir, agySettingsFile)
		if err := writeFileAtomicMode(dst, merged, 0600); err != nil {
			return "", fmt.Errorf("failed to write agy settings: %w", err)
		}
	}
	committed = true
	return dir, nil
}

// SyncAgyAccountSettings refreshes ~/.gemini/antigravity-cli-ccp-<account>/settings.json
// from the main ~/.gemini/antigravity-cli/settings.json while preserving the
// account file's extra items in permissions.{allow,deny,ask} and
// trustedWorkspaces. If main does not exist, the account file is left alone.
func SyncAgyAccountSettings(account string) error {
	if err := ValidateAccountName(account); err != nil {
		return err
	}
	merged, hasSettings, err := computeAgySettings(account)
	if err != nil {
		return err
	}
	if !hasSettings {
		return nil
	}
	dataDir, err := AgyAccountDataDir(account)
	if err != nil {
		return err
	}
	return writeFileAtomicMode(filepath.Join(dataDir, agySettingsFile), merged, 0600)
}

func computeAgySettings(account string) ([]byte, bool, error) {
	mainDir, err := AgyMainDataDir()
	if err != nil {
		return nil, false, err
	}
	mainPath := filepath.Join(mainDir, agySettingsFile)
	mainData, err := os.ReadFile(mainPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read %s: %w", mainPath, err)
	}

	acctDir, err := AgyAccountDataDir(account)
	if err != nil {
		return nil, false, err
	}
	acctPath := filepath.Join(acctDir, agySettingsFile)
	acctData, err := os.ReadFile(acctPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("failed to read %s: %w", acctPath, err)
	}

	merged, err := MergeAgySettingsData(mainData, acctData)
	if err != nil {
		return nil, false, fmt.Errorf("failed to merge agy settings: %w", err)
	}
	return merged, true, nil
}

// MergeAgySettingsData merges main settings.json with an account settings.json.
// For permissions.allow, permissions.deny, permissions.ask, and trustedWorkspaces,
// result = main's list + the account's items main lacks (order kept, no duplicates).
// Every other key comes from main.
func MergeAgySettingsData(mainData, acctData []byte) ([]byte, error) {
	var mainMap map[string]any
	if err := json.Unmarshal(mainData, &mainMap); err != nil {
		return nil, fmt.Errorf("invalid main settings JSON: %w", err)
	}
	if mainMap == nil {
		mainMap = map[string]any{}
	}

	var acctMap map[string]any
	if len(bytes.TrimSpace(acctData)) > 0 {
		if err := json.Unmarshal(acctData, &acctMap); err != nil {
			return nil, fmt.Errorf("invalid account settings JSON: %w", err)
		}
	}
	if acctMap == nil {
		acctMap = map[string]any{}
	}

	result := make(map[string]any, len(mainMap))
	maps.Copy(result, mainMap)

	if merged, ok := mergeSettingsList(mainMap["trustedWorkspaces"], acctMap["trustedWorkspaces"]); ok {
		result["trustedWorkspaces"] = merged
	}

	mainPerm, mainHasPerm := mainMap["permissions"].(map[string]any)
	acctPerm, acctHasPerm := acctMap["permissions"].(map[string]any)
	if mainHasPerm || acctHasPerm {
		permOut := map[string]any{}
		if mainHasPerm {
			maps.Copy(permOut, mainPerm)
		}
		for _, key := range []string{"allow", "deny", "ask"} {
			var mv, av any
			if mainHasPerm {
				mv = mainPerm[key]
			}
			if acctHasPerm {
				av = acctPerm[key]
			}
			if merged, ok := mergeSettingsList(mv, av); ok {
				permOut[key] = merged
			}
		}
		if mainHasPerm || len(permOut) > 0 {
			result["permissions"] = permOut
		}
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func mergeSettingsList(mainVal, acctVal any) ([]any, bool) {
	mainSlice, mainIsList := mainVal.([]any)
	acctSlice, acctIsList := acctVal.([]any)
	if !mainIsList && len(acctSlice) == 0 {
		return nil, false
	}
	if !mainIsList && !acctIsList {
		return nil, false
	}

	out := make([]any, 0, len(mainSlice)+len(acctSlice))
	seen := make(map[string]bool, len(mainSlice)+len(acctSlice))
	appendUnique := func(items []any) {
		for _, item := range items {
			raw, err := json.Marshal(item)
			if err != nil {
				continue
			}
			key := string(raw)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	if mainIsList {
		appendUnique(mainSlice)
	}
	if acctIsList {
		appendUnique(acctSlice)
	}
	return out, true
}
