package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// accountTestPaths builds a ccp tree and a fake home under t.TempDir.
func accountTestPaths(t *testing.T, profiles ...string) *config.Paths {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	ccp := filepath.Join(root, "ccp")
	paths := &config.Paths{
		CcpDir:          ccp,
		ClaudeDir:       filepath.Join(home, ".claude"),
		GlobalClaudeDir: filepath.Join(home, ".claude"),
		HubDir:          filepath.Join(ccp, "hub"),
		ProfilesDir:     filepath.Join(ccp, "profiles"),
		SharedDir:       filepath.Join(ccp, "profiles", "shared"),
		StoreDir:        filepath.Join(ccp, "store"),
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if err := os.MkdirAll(paths.ProfileDir(p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"work1", true},
		{"Work_2", true},
		{"a-b", true},
		{"9", true},
		{"", false},
		{"-work", false},
		{"_work", false},
		{".hidden", false},
		{"a/b", false},
		{"..", false},
		{"a b", false},
		{"wörk", false},
	}
	for _, tt := range tests {
		err := ValidateAccountName(tt.name)
		if (err == nil) != tt.ok {
			t.Errorf("ValidateAccountName(%q) err = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
	paths := accountTestPaths(t, "p")
	if _, _, err := LinkAccount(paths, "../p", "p", false); err == nil {
		t.Error("LinkAccount accepted an invalid name")
	}
}

func TestLinkAccount_ReturnsExactPath(t *testing.T) {
	paths := accountTestPaths(t, "p")
	dir, _, err := LinkAccount(paths, "work1", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(paths.CcpDir, "accounts", "work1")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || strings.HasSuffix(dir, "/") {
		t.Errorf("dir %q is not absolute and clean", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("account folder not created: %v", err)
	}
	if got, err := AccountProfile(dir); err != nil || got != "p" {
		t.Errorf("AccountProfile = %q, %v; want p", got, err)
	}
}

func TestLinkAccount_UnknownProfile(t *testing.T) {
	paths := accountTestPaths(t)
	if _, _, err := LinkAccount(paths, "work1", "nope", false); err == nil || !strings.Contains(err.Error(), "profile not found") {
		t.Errorf("err = %v, want profile not found", err)
	}
}

func TestLinkAccount_SeedState(t *testing.T) {
	const state = `{"oauthAccount":{"emailAddress":"a@b.c"},"numStartups":3,"projects":{"/x":{"allowedTools":[]}}}`
	tests := []struct {
		name       string
		global     bool // ~/.claude links to the profile
		globalJSON string
		profJSON   string
		wantFrom   string // "global", "profile" or "" for no file
	}{
		{"global symlink uses ~/.claude.json", true, state, `{"numStartups":99}`, "global"},
		{"env activation uses profile file", false, `{"numStartups":99}`, state, "profile"},
		{"global symlink without ~/.claude.json writes nothing", true, "", state, ""},
		{"no source writes nothing", false, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := accountTestPaths(t, "p")
			profDir := paths.ProfileDir("p")
			if tt.global {
				if err := os.Symlink(profDir, paths.GlobalClaudeDir); err != nil {
					t.Fatal(err)
				}
			}
			if tt.globalJSON != "" {
				writeFile(t, filepath.Join(filepath.Dir(paths.GlobalClaudeDir), ".claude.json"), tt.globalJSON)
			}
			if tt.profJSON != "" {
				writeFile(t, filepath.Join(profDir, ".claude.json"), tt.profJSON)
			}
			dir, _, err := LinkAccount(paths, "work1", "p", false)
			if err != nil {
				t.Fatal(err)
			}
			dst := filepath.Join(dir, ".claude.json")
			info, err := os.Lstat(dst)
			if tt.wantFrom == "" {
				if err == nil {
					t.Fatalf("seeded %s with no source", dst)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Fatal(".claude.json is a link; it must be the account's own file")
			}
			if info.Mode().Perm() != 0600 {
				t.Errorf("mode = %v, want 0600", info.Mode().Perm())
			}
			got := readJSON(t, dst)
			if _, ok := got["oauthAccount"]; ok {
				t.Error("oauthAccount was copied")
			}
			if got["numStartups"] != float64(3) {
				t.Errorf("numStartups = %v, want 3 (from the %s source)", got["numStartups"], tt.wantFrom)
			}
			if _, ok := got["projects"]; !ok {
				t.Error("projects key dropped")
			}
		})
	}
}

func TestLinkAccount_NeverOverwritesState(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), `{"numStartups":1}`)
	existing := `{"oauthAccount":{"emailAddress":"work1@corp"}}`
	writeFile(t, filepath.Join(paths.AccountDir("work1"), ".claude.json"), existing)

	dir, _, err := LinkAccount(paths, "work1", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if string(data) != existing {
		t.Errorf(".claude.json overwritten: %s", data)
	}
}

func TestLinkAccount_InvalidSourceJSONFails(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), "{not json")
	if _, _, err := LinkAccount(paths, "work1", "p", false); err == nil {
		t.Error("seeding from invalid JSON succeeded")
	}
}

func TestLinkAccount_Links(t *testing.T) {
	paths := accountTestPaths(t, "p")
	prof := paths.ProfileDir("p")
	writeFile(t, filepath.Join(prof, "settings.json"), "{}")
	writeFile(t, filepath.Join(prof, "CLAUDE.md"), "x")
	writeFile(t, filepath.Join(prof, ".hidden"), "x")
	writeFile(t, filepath.Join(prof, "skills", "s", "SKILL.md"), "x")
	writeFile(t, filepath.Join(prof, ".claude.json"), "{}")
	writeFile(t, filepath.Join(prof, ".ccp-account.toml"), "profile = \"other\"\n")
	writeFile(t, filepath.Join(prof, ".credentials.json"), "{}")
	writeFile(t, filepath.Join(prof, "daemon.sock"), "")
	writeFile(t, filepath.Join(prof, "daemon"), "")

	dir, _, err := LinkAccount(paths, "work1", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.json", "CLAUDE.md", ".hidden", "skills"} {
		target, err := os.Readlink(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: not a link: %v", name, err)
			continue
		}
		if want := filepath.Join(prof, name); target != want {
			t.Errorf("%s -> %q, want %q", name, target, want)
		}
	}
	for _, name := range []string{".credentials.json", "daemon.sock", "daemon"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was linked; it must be skipped", name)
		}
	}
	for _, name := range []string{".claude.json", ".ccp-account.toml"} {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s must be the account's own file (err %v)", name, err)
		}
	}
	if got, _ := AccountProfile(dir); got != "p" {
		t.Errorf("binding = %q, the profile's own .ccp-account.toml leaked in", got)
	}

	// Idempotent
	if _, warnings, err := LinkAccount(paths, "work1", "p", false); err != nil || len(warnings) != 0 {
		t.Errorf("second run: warnings %v, err %v", warnings, err)
	}
}

func TestLinkAccount_RemovesBrokenLinks(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), "gone.md"), "x")
	dir, _, err := LinkAccount(paths, "work1", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(paths.ProfileDir("p"), "gone.md"))
	if err := os.Symlink(filepath.Join(dir, "nowhere"), filepath.Join(dir, "foreign")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LinkAccount(paths, "work1", "p", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gone.md", "foreign"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("broken link %s kept", name)
		}
	}
}

func TestLinkAccount_KeepsRealFiles(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), "settings.json"), "{}")
	real := filepath.Join(paths.AccountDir("work1"), "settings.json")
	writeFile(t, real, "mine")

	_, warnings, err := LinkAccount(paths, "work1", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "settings.json") {
		t.Errorf("warnings = %v, want one about settings.json", warnings)
	}
	data, _ := os.ReadFile(real)
	info, _ := os.Lstat(real)
	if string(data) != "mine" || info.Mode()&os.ModeSymlink != 0 {
		t.Error("real file was replaced")
	}
}

func TestLinkAccount_ProfileMismatch(t *testing.T) {
	paths := accountTestPaths(t, "a", "b")
	writeFile(t, filepath.Join(paths.ProfileDir("a"), "only-a.md"), "x")
	writeFile(t, filepath.Join(paths.ProfileDir("a"), "CLAUDE.md"), "a")
	writeFile(t, filepath.Join(paths.ProfileDir("b"), "CLAUDE.md"), "b")

	dir, _, err := LinkAccount(paths, "work1", "a", false)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = LinkAccount(paths, "work1", "b", false)
	if err == nil {
		t.Fatal("linking to another profile without relink succeeded")
	}
	for _, want := range []string{`account "work1" is linked to profile "a"`, "--relink"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to contain %q", err, want)
		}
	}
	if target, _ := os.Readlink(filepath.Join(dir, "CLAUDE.md")); target != filepath.Join(paths.ProfileDir("a"), "CLAUDE.md") {
		t.Errorf("refused link still changed CLAUDE.md -> %q", target)
	}

	if _, _, err := LinkAccount(paths, "work1", "b", true); err != nil {
		t.Fatalf("relink: %v", err)
	}
	if got, _ := AccountProfile(dir); got != "b" {
		t.Errorf("binding = %q after relink, want b", got)
	}
	if target, _ := os.Readlink(filepath.Join(dir, "CLAUDE.md")); target != filepath.Join(paths.ProfileDir("b"), "CLAUDE.md") {
		t.Errorf("CLAUDE.md -> %q after relink, want profile b", target)
	}
	if _, err := os.Lstat(filepath.Join(dir, "only-a.md")); err == nil {
		t.Error("link into profile a kept after relink to b")
	}
}

func TestConfigDirProfile(t *testing.T) {
	paths := accountTestPaths(t, "dev")
	dir, _, err := LinkAccount(paths, "work1", "dev", false)
	if err != nil {
		t.Fatal(err)
	}
	unbound := paths.AccountDir("stray")
	os.MkdirAll(unbound, 0755)

	tests := []struct {
		dir, profile, account string
	}{
		{dir, "dev", "work1"},
		{dir + "/", "dev", "work1"},
		{unbound, "", "stray"},
		{paths.ProfileDir("dev"), "dev", ""},
		{"/somewhere/else", "else", ""},
	}
	for _, tt := range tests {
		p, a := ConfigDirProfile(paths, tt.dir)
		if p != tt.profile || a != tt.account {
			t.Errorf("ConfigDirProfile(%q) = %q, %q; want %q, %q", tt.dir, p, a, tt.profile, tt.account)
		}
	}
}

func TestLinkAccount_NeverLinksStateCopies(t *testing.T) {
	paths := accountTestPaths(t, "p")
	prof := paths.ProfileDir("p")
	writeFile(t, filepath.Join(prof, "backups", ".claude.json.backup.1700000000"), `{"oauthAccount":{}}`)
	writeFile(t, filepath.Join(prof, ".claude.json.backup"), "{}")
	writeFile(t, filepath.Join(prof, ".claude.json.lock"), "")
	writeFile(t, filepath.Join(prof, "settings.json"), "{}")

	// An account linked by an older version: backups/ and a backup file are links.
	dir := paths.AccountDir("p")
	os.MkdirAll(dir, 0755)
	os.Symlink(filepath.Join(prof, "backups"), filepath.Join(dir, "backups"))
	os.Symlink(filepath.Join(prof, ".claude.json.backup"), filepath.Join(dir, ".claude.json.backup"))

	if _, _, err := LinkAccount(paths, "p", "p", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"backups", ".claude.json.backup", ".claude.json.lock"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s is linked into the account", name)
		}
	}
	if _, err := os.Readlink(filepath.Join(dir, "settings.json")); err != nil {
		t.Errorf("settings.json not linked: %v", err)
	}

	// The account's own backups (written by Claude) stay.
	writeFile(t, filepath.Join(dir, "backups", "own"), "x")
	if _, _, err := LinkAccount(paths, "p", "p", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", "own")); err != nil {
		t.Errorf("account's own backups/ removed: %v", err)
	}
}

func TestLinkAccount_CorruptSeedLeavesNoFolder(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), "{broken")
	if _, _, err := LinkAccount(paths, "work1", "p", false); err == nil {
		t.Fatal("corrupt state accepted")
	}
	if _, err := os.Stat(paths.AccountDir("work1")); err == nil {
		t.Error("account folder created before the seed was checked")
	}
}

func TestLinkAccount_MismatchLeavesNoChange(t *testing.T) {
	paths := accountTestPaths(t, "a", "b")
	if _, _, err := LinkAccount(paths, "w", "a", false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(paths.AccountDir("w"), AccountFile))
	if _, _, err := LinkAccount(paths, "w", "b", false); err == nil {
		t.Fatal("mismatch accepted")
	}
	if after, _ := os.ReadFile(filepath.Join(paths.AccountDir("w"), AccountFile)); string(after) != string(before) {
		t.Error("binding changed by a refused link")
	}
}

func TestConfigDirProfile_Normalizes(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	paths := &config.Paths{
		CcpDir:          "ccp", // relative CCP_DIR
		GlobalClaudeDir: filepath.Join(root, "home", ".claude"),
		ProfilesDir:     filepath.Join("ccp", "profiles"),
	}
	os.MkdirAll(paths.ProfileDir("dev"), 0755)
	if _, _, err := LinkAccount(paths, "work1", "dev", false); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, "ccp", "accounts", "work1")
	for _, dir := range []string{abs, abs + "/", "ccp/accounts/work1", "./ccp/accounts/work1/"} {
		if p, a := ConfigDirProfile(paths, dir); p != "dev" || a != "work1" {
			t.Errorf("ConfigDirProfile(%q) = %q, %q; want dev, work1", dir, p, a)
		}
	}
	if !SamePath(filepath.Join(root, "ccp", "profiles", "dev")+"/", paths.ProfileDir("dev")) {
		t.Error("SamePath: trailing slash vs relative path")
	}
	if SamePath("/a/b", "/a/c") {
		t.Error("SamePath: different paths")
	}
}

func TestLinkAccount_NeverLinksPerFolderState(t *testing.T) {
	paths := accountTestPaths(t, "p")
	prof := paths.ProfileDir("p")
	own := []string{".config.json", ".storage-write.lock", ".storage-write", "history.lock", "jobs"}
	for _, name := range own {
		writeFile(t, filepath.Join(prof, name), "{}")
	}
	writeFile(t, filepath.Join(prof, "settings.json"), "{}")
	dir := paths.AccountDir("p")
	os.MkdirAll(dir, 0755)
	for _, name := range own { // linked by an older version
		os.Symlink(filepath.Join(prof, name), filepath.Join(dir, name))
	}

	if _, _, err := LinkAccount(paths, "p", "p", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range own {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s is linked into the account", name)
		}
	}
	if _, err := os.Readlink(filepath.Join(dir, "settings.json")); err != nil {
		t.Errorf("settings.json not linked: %v", err)
	}
}

func TestLinkAccount_RejectsBadProfileNames(t *testing.T) {
	paths := accountTestPaths(t, "p", "shared")
	for _, name := range []string{"", "shared", ".", "..", "a/b", "../p", "p/"} {
		if _, _, err := LinkAccount(paths, "w", name, false); err == nil {
			t.Errorf("LinkAccount accepted profile %q", name)
		}
	}
	if _, err := os.Stat(paths.AccountsDir()); err == nil {
		t.Error("a rejected name created the accounts folder")
	}
}

func TestLinkAccount_NullState(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), "null")
	dir, _, err := LinkAccount(paths, "w", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".claude.json")); strings.TrimSpace(string(data)) != "{}" {
		t.Errorf("seed from null = %q, want {}", data)
	}
}

func TestLinkAccount_ReplacesLinkedLoginFiles(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), `{"oauthAccount":{"e":"x"},"n":2}`)
	other := filepath.Join(paths.CcpDir, "elsewhere")
	writeFile(t, filepath.Join(other, ".claude.json"), `{"oauthAccount":{"e":"other"}}`)
	writeFile(t, filepath.Join(other, ".credentials.json"), "tok")
	dir := paths.AccountDir("w")
	os.MkdirAll(dir, 0755)
	os.Symlink(filepath.Join(other, ".claude.json"), filepath.Join(dir, ".claude.json"))
	os.Symlink(filepath.Join(other, ".credentials.json"), filepath.Join(dir, ".credentials.json"))

	_, warnings, err := LinkAccount(paths, "w", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want one per removed link", warnings)
	}
	info, err := os.Lstat(filepath.Join(dir, ".claude.json"))
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
		t.Fatalf(".claude.json: %v %v", info, err)
	}
	var got map[string]any
	data, _ := os.ReadFile(filepath.Join(dir, ".claude.json"))
	json.Unmarshal(data, &got)
	if got["n"] != float64(2) || got["oauthAccount"] != nil {
		t.Errorf("seed = %s", data)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".credentials.json")); err == nil {
		t.Error(".credentials.json link kept")
	}
	if data, _ := os.ReadFile(filepath.Join(other, ".claude.json")); !strings.Contains(string(data), "other") {
		t.Error("the link target was changed")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".claude.json.ccp-seed-*")); len(leftovers) != 0 {
		t.Errorf("seed temp files left: %v", leftovers)
	}
}

func TestGetCurrent(t *testing.T) {
	paths := accountTestPaths(t, "a", "b")
	for _, p := range []string{"a", "b"} {
		writeFile(t, filepath.Join(paths.ProfileDir(p), "profile.toml"), "version = 3\nname = \""+p+"\"\n")
	}
	dir, _, err := LinkAccount(paths, "w", "b", false)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ claudeDir, want string }{
		{paths.ProfileDir("a"), "a"},
		{dir + "/", "b"},
		{"/somewhere/else", ""},
	}
	for _, tt := range tests {
		p2 := *paths
		p2.ClaudeDir = tt.claudeDir
		got, _ := NewManager(&p2).GetCurrent()
		name := ""
		if got != nil {
			name = got.Name
		}
		if name != tt.want {
			t.Errorf("GetCurrent with ClaudeDir %q = %q, want %q", tt.claudeDir, name, tt.want)
		}
	}
}

func TestInsideCcp(t *testing.T) {
	paths := accountTestPaths(t, "p")
	for dir, want := range map[string]bool{
		paths.CcpDir:                  true,
		paths.ProfileDir("p"):         true,
		paths.AccountDir("w") + "/":   true,
		paths.GlobalClaudeDir:         false,
		paths.CcpDir + "-sibling/foo": false,
	} {
		if got := InsideCcp(paths, dir); got != want {
			t.Errorf("InsideCcp(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestLinkAccount_WarnsAboutConfigJSON(t *testing.T) {
	paths := accountTestPaths(t, "p")
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".claude.json"), `{"n":1}`)
	writeFile(t, filepath.Join(paths.ProfileDir("p"), ".config.json"), `{"oauthAccount":{"e":"x"}}`)
	dir, warnings, err := LinkAccount(paths, "w", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), ".config.json") {
		t.Errorf("warnings = %v", warnings)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if strings.Contains(string(data), "oauthAccount") || !strings.Contains(string(data), "\"n\"") {
		t.Errorf("seed = %s (must come from .claude.json only)", data)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".config.json")); err == nil {
		t.Error(".config.json linked")
	}
}

func TestInsideCcp_SymlinkedProfilesDir(t *testing.T) {
	root := t.TempDir()
	dotfiles := filepath.Join(root, "dotfiles", "profiles")
	os.MkdirAll(filepath.Join(dotfiles, "p"), 0755)
	ccp := filepath.Join(root, "ccp")
	os.MkdirAll(ccp, 0755)
	if err := os.Symlink(dotfiles, filepath.Join(ccp, "profiles")); err != nil {
		t.Fatal(err)
	}
	paths := &config.Paths{CcpDir: ccp, ProfilesDir: filepath.Join(ccp, "profiles")}
	for _, dir := range []string{filepath.Join(ccp, "profiles", "p"), filepath.Join(dotfiles, "p")} {
		if !InsideCcp(paths, dir) {
			t.Errorf("InsideCcp(%q) = false, want true", dir)
		}
	}
	if InsideCcp(paths, filepath.Join(root, "dotfiles", "other")) {
		t.Error("a folder beside the linked profiles counted as inside")
	}
}

func TestAccountProfile_RejectsInvalidBoundNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"shared", "../hub", "a/b", ".."} {
		os.WriteFile(filepath.Join(dir, AccountFile), []byte("profile = \""+name+"\"\n"), 0644)
		if got, err := AccountProfile(dir); err == nil {
			t.Errorf("AccountProfile accepted %q (got %q)", name, got)
		}
	}
	paths := accountTestPaths(t, "p")
	os.MkdirAll(paths.AccountDir("bad"), 0755)
	os.WriteFile(filepath.Join(paths.AccountDir("bad"), AccountFile), []byte("profile = \"shared\"\n"), 0644)
	accounts, _ := ListAccounts(paths)
	if len(accounts) != 1 || accounts[0].Err == nil {
		t.Errorf("ListAccounts = %+v, want the bad binding as an error", accounts)
	}
}

func TestSetAccountMigratedFrom(t *testing.T) {
	paths := accountTestPaths(t, "p")
	dir, _, err := LinkAccount(paths, "w", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetAccountMigratedFrom(dir, "Claude Code-credentials"); err != nil {
		t.Fatal(err)
	}
	if got, _ := AccountMigratedFrom(dir); got != "Claude Code-credentials" {
		t.Errorf("recorded %q", got)
	}
	if got, _ := AccountProfile(dir); got != "p" {
		t.Errorf("binding lost: %q", got)
	}
	// A relink to the same profile keeps the record.
	if _, _, err := LinkAccount(paths, "w", "p", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := AccountMigratedFrom(dir); got != "Claude Code-credentials" {
		t.Errorf("record lost on relink: %q", got)
	}
}
