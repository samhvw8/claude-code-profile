package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAgyLaunch_ReturnsBothFlagAndSSH(t *testing.T) {
	if got := AgyAppDataDirName("work1"); got != "antigravity-cli-ccp-work1" {
		t.Errorf("AgyAppDataDirName = %q", got)
	}
	flagArg, envEntry := AgyLaunch("work1")
	if flagArg != "--app_data_dir=antigravity-cli-ccp-work1" {
		t.Errorf("flagArg = %q", flagArg)
	}
	if envEntry != "SSH_CLIENT=ccp 0 0" {
		t.Errorf("envEntry = %q", envEntry)
	}
}

func TestCreateAgyAccount_CreatesBothFoldersAndMergedSettings(t *testing.T) {
	paths := accountTestPaths(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	t.Setenv("HOME", home)

	mainDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(mainDir, "settings.json"), `{
  "theme": "dark",
  "permissions": {
    "allow": ["Read", "Bash(git:*)"],
    "defaultMode": "plan"
  },
  "trustedWorkspaces": ["/proj/main"]
}
`)
	// Profile has files that must NOT be symlinked into an agy account folder.
	writeFile(t, filepath.Join(paths.ProfileDir("work"), "CLAUDE.md"), "hi")
	writeFile(t, filepath.Join(paths.ProfileDir("work"), "settings.json"), "{}")

	dir, err := CreateAgyAccount(paths, "agy1", "work")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(paths.CcpDir, "accounts", "agy1") {
		t.Errorf("dir = %q", dir)
	}
	bound, harness, err := AccountInfo(dir)
	if err != nil || bound != "work" || harness != HarnessAgy {
		t.Fatalf("AccountInfo = %q, %q, %v", bound, harness, err)
	}
	// No symlinks and no .claude.json inside ~/.ccp/accounts/agy1/.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || e.Name() == ".claude.json" || e.Name() == "CLAUDE.md" {
			t.Errorf("unexpected entry %q (type %v) in agy account dir", e.Name(), e.Type())
		}
	}

	// Merged settings.json exists with mode 0600 inside ~/.gemini/antigravity-cli-ccp-agy1/.
	dataDir, err := AgyAccountDataDir("agy1")
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dataDir, "settings.json")
	info, err := os.Lstat(settingsPath)
	if err != nil {
		t.Fatalf("settings.json missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
		t.Errorf("settings.json mode = %v, symlink=%v", info.Mode().Perm(), info.Mode()&os.ModeSymlink != 0)
	}
	got := readJSON(t, settingsPath)
	if got["theme"] != "dark" {
		t.Errorf("theme = %v", got["theme"])
	}

	accounts, err := ListAccounts(paths)
	if err != nil || len(accounts) != 1 || accounts[0].Harness != HarnessAgy || accounts[0].Profile != "work" {
		t.Errorf("ListAccounts = %+v, %v", accounts, err)
	}
}

func TestCreateAgyAccount_CorruptMainLeavesNoFolder(t *testing.T) {
	paths := accountTestPaths(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), "{broken")

	if _, err := CreateAgyAccount(paths, "agy1", "work"); err == nil {
		t.Fatal("corrupt main settings accepted")
	}
	if _, err := os.Stat(paths.AccountDir("agy1")); err == nil {
		t.Error("account folder created despite corrupt main settings")
	}
}

func TestLinkAccount_SkipsAgySymlinksAndRelinksBindingOnly(t *testing.T) {
	paths := accountTestPaths(t, "a", "b")
	home := filepath.Dir(paths.GlobalClaudeDir)
	t.Setenv("HOME", home)

	writeFile(t, filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), "a")
	writeFile(t, filepath.Join(paths.ProfileDir("b"), "CLAUDE.md"), "b")

	dir, err := CreateAgyAccount(paths, "agy1", "a")
	if err != nil {
		t.Fatal(err)
	}

	// Same profile: LinkAccount and RefreshAccounts skip without creating symlinks.
	if gotDir, warnings, err := LinkAccount(paths, "agy1", "a", false); err != nil || len(warnings) != 0 || gotDir != dir {
		t.Fatalf("LinkAccount same profile: %q %v %v", gotDir, warnings, err)
	}
	RefreshAccounts(paths, "a")
	if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md symlink created in agy account")
	}

	// Mismatched profile without --relink: refused.
	if _, _, err := LinkAccount(paths, "agy1", "b", false); err == nil || !strings.Contains(err.Error(), "--relink") {
		t.Errorf("mismatch without relink: %v", err)
	}

	// Relink / rename rewrites only the binding and creates no symlinks.
	if warnings, err := RelinkAccounts(paths, "a", "b"); err != nil || len(warnings) != 0 {
		t.Fatalf("RelinkAccounts: %v %v", warnings, err)
	}
	bound, harness, err := AccountInfo(dir)
	if err != nil || bound != "b" || harness != HarnessAgy {
		t.Errorf("after relink: bound=%q harness=%q err=%v", bound, harness, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md symlink created after relink")
	}

	// pruneAccountLinks also skips agy accounts even if a hand-made symlink is present.
	if err := os.Symlink(filepath.Join(paths.ProfileDir("b"), "CLAUDE.md"), filepath.Join(dir, "keep-me")); err != nil {
		t.Fatal(err)
	}
	if err := pruneAccountLinks(paths, dir, paths.ProfileDir("b")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "keep-me")); err != nil {
		t.Errorf("pruneAccountLinks touched agy account: %v", err)
	}
}

func TestMergeAgySettingsData(t *testing.T) {
	mainJSON := `{
  "theme": "main-theme",
  "model": "gemini-4",
  "trustedWorkspaces": ["/ws/a", "/ws/b", "/ws/a"],
  "permissions": {
    "defaultMode": "plan",
    "allow": ["Read", "Grep", "Read"],
    "deny": ["Bash(rm:*)"],
    "ask": ["Write"]
  }
}`
	acctJSON := `{
  "theme": "acct-theme",
  "accountOnlyKey": true,
  "trustedWorkspaces": ["/ws/b", "/ws/c", "/ws/c"],
  "permissions": {
    "defaultMode": "bypass",
    "extraPermKey": "drop-me",
    "allow": ["Grep", "Edit", "Edit"],
    "deny": ["Bash(rm:*)", "WebFetch"],
    "ask": ["Bash(git:*)"]
  }
}`
	out, err := MergeAgySettingsData([]byte(mainJSON), []byte(acctJSON))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "main-theme" || got["model"] != "gemini-4" {
		t.Errorf("scalar keys not from main: %v", got)
	}
	if _, ok := got["accountOnlyKey"]; ok {
		t.Error("accountOnlyKey leaked into merged settings")
	}
	wantWS := []any{"/ws/a", "/ws/b", "/ws/c"}
	if !slices.Equal(got["trustedWorkspaces"].([]any), wantWS) {
		t.Errorf("trustedWorkspaces = %v, want %v", got["trustedWorkspaces"], wantWS)
	}
	perm := got["permissions"].(map[string]any)
	if perm["defaultMode"] != "plan" {
		t.Errorf("permissions.defaultMode = %v, want plan", perm["defaultMode"])
	}
	if _, ok := perm["extraPermKey"]; ok {
		t.Error("permissions.extraPermKey leaked from account")
	}
	if !slices.Equal(perm["allow"].([]any), []any{"Read", "Grep", "Edit"}) {
		t.Errorf("permissions.allow = %v", perm["allow"])
	}
	if !slices.Equal(perm["deny"].([]any), []any{"Bash(rm:*)", "WebFetch"}) {
		t.Errorf("permissions.deny = %v", perm["deny"])
	}
	if !slices.Equal(perm["ask"].([]any), []any{"Write", "Bash(git:*)"}) {
		t.Errorf("permissions.ask = %v", perm["ask"])
	}
}

func TestMergeAgySettingsData_MainLacksListsAccountHasThem(t *testing.T) {
	mainJSON := `{"theme": "dark"}`
	acctJSON := `{"trustedWorkspaces": ["/x"], "permissions": {"allow": ["Read"], "other": 1}}`
	out, err := MergeAgySettingsData([]byte(mainJSON), []byte(acctJSON))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["trustedWorkspaces"].([]any), []any{"/x"}) {
		t.Errorf("trustedWorkspaces = %v", got["trustedWorkspaces"])
	}
	perm := got["permissions"].(map[string]any)
	if !slices.Equal(perm["allow"].([]any), []any{"Read"}) {
		t.Errorf("permissions.allow = %v", perm["allow"])
	}
	if _, ok := perm["other"]; ok {
		t.Error("non-list key permissions.other leaked from account")
	}
}

func TestSyncAgyAccountSettings_MissingMainLeavesAccountAloneAndNeverWritesMain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataDir, err := AgyAccountDataDir("agy1")
	if err != nil {
		t.Fatal(err)
	}
	acctPath := filepath.Join(dataDir, "settings.json")
	orig := `{"trustedWorkspaces":["/mine"]}`
	writeFile(t, acctPath, orig)

	// Missing main: account file left untouched, main never created.
	if err := SyncAgyAccountSettings("agy1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(acctPath)
	if err != nil || string(data) != orig {
		t.Errorf("account settings changed without main: %q %v", data, err)
	}
	mainPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	if _, err := os.Stat(mainPath); err == nil {
		t.Fatal("main settings.json was created")
	}

	// Present main: merged and written atomically (replacing a symlink), main untouched.
	mainContent := `{"theme":"dark","trustedWorkspaces":["/main"]}`
	writeFile(t, mainPath, mainContent)
	if err := os.Remove(acctPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(mainPath, acctPath); err != nil {
		t.Fatal(err)
	}
	if err := SyncAgyAccountSettings("agy1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(acctPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
		t.Errorf("synced settings is symlink=%v mode=%v err=%v", info.Mode()&os.ModeSymlink != 0, info.Mode().Perm(), err)
	}
	gotMain, _ := os.ReadFile(mainPath)
	if string(gotMain) != mainContent {
		t.Errorf("main settings modified: %q", gotMain)
	}
}

func TestWithEnv(t *testing.T) {
	base := []string{"PATH=/bin", "SSH_CLIENT=old", "KEEP=1"}
	got := WithEnv(base, "SSH_CLIENT=ccp 0 0", "CLAUDE_CONFIG_DIR=/p")
	want := []string{"PATH=/bin", "SSH_CLIENT=ccp 0 0", "KEEP=1", "CLAUDE_CONFIG_DIR=/p"}
	if !slices.Equal(got, want) {
		t.Errorf("WithEnv = %v, want %v", got, want)
	}
}

func TestLinkAccount_CorruptAgyBindingRelinkPreservesHarnessAndNoSymlinks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"missing profile", "harness = \"agy\"\n"},
		{"invalid profile", "profile = \"shared\"\nharness = \"agy\"\n"},
		{"corrupt toml with agy dataDir", "profile = [\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := accountTestPaths(t, "a", "b")
			home := filepath.Dir(paths.GlobalClaudeDir)
			t.Setenv("HOME", home)
			writeFile(t, filepath.Join(paths.ProfileDir("b"), "CLAUDE.md"), "b")

			dir, err := CreateAgyAccount(paths, "agy1", "a")
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, AccountFile), tc.content)

			if _, _, err := LinkAccount(paths, "agy1", "b", false); err == nil {
				t.Fatal("expected error without --relink")
			}
			if _, _, err := LinkAccount(paths, "agy1", "b", true); err != nil {
				t.Fatalf("LinkAccount relink: %v", err)
			}
			bound, harness, err := AccountInfo(dir)
			if err != nil || bound != "b" || harness != HarnessAgy {
				t.Errorf("after relink: bound=%q harness=%q err=%v", bound, harness, err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); err == nil {
				t.Error("CLAUDE.md symlink created in agy account on relink")
			}
			if _, err := os.Lstat(filepath.Join(dir, claudeStateFile)); err == nil {
				t.Error(".claude.json created in agy account on relink")
			}
		})
	}
}

func TestCreateAgyAccount_RollsBackOnFailure(t *testing.T) {
	paths := accountTestPaths(t, "work")
	home := filepath.Dir(paths.GlobalClaudeDir)
	t.Setenv("HOME", home)
	// Make ~/.gemini a regular file so MkdirAll(dataDir) fails after dir creation.
	writeFile(t, filepath.Join(home, ".gemini"), "not-a-dir")

	if _, err := CreateAgyAccount(paths, "agy1", "work"); err == nil {
		t.Fatal("expected error when ~/.gemini is a file")
	}
	if _, err := os.Stat(paths.AccountDir("agy1")); err == nil {
		t.Error("partial account folder left behind after failure")
	}
}

func TestSyncAndDataDir_RejectInvalidAccountName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := AgyAccountDataDir("../escape"); err == nil {
		t.Error("AgyAccountDataDir accepted invalid account name")
	}
	if err := SyncAgyAccountSettings("../escape"); err == nil {
		t.Error("SyncAgyAccountSettings accepted invalid account name")
	}
}

func TestMergeAgySettingsData_WhitespaceAccountFile(t *testing.T) {
	out, err := MergeAgySettingsData([]byte(`{"theme":"dark"}`), []byte("  \n\t "))
	if err != nil {
		t.Fatalf("whitespace account settings failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil || got["theme"] != "dark" {
		t.Errorf("got = %v, err = %v", got, err)
	}
}
