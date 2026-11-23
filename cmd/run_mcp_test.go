package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// fakeClaude writes a program named claude that records its arguments, one
// per line, and returns its path and the record's path.
func fakeClaude(t *testing.T) (bin, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, out
}

func recordedArgs(t *testing.T, out string) []string {
	t.Helper()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The profile's mcp.json reaches Claude Code on every account, as one
// --mcp-config=<file> argument right after the program, so a subcommand or a
// prompt after it is not taken as a second config file.
func TestRun_PassesProfileMCP(t *testing.T) {
	paths, _ := poolEnv(t)
	bin, out := fakeClaude(t)
	if err := runCcp(t, "run", "a", "--account", "c1", "--", bin, "mcp", "list"); err != nil {
		t.Fatal(err)
	}
	if got := recordedArgs(t, out); strings.Join(got, " ") != "mcp list" {
		t.Errorf("without mcp.json: %q", got)
	}
	file := filepath.Join(paths.ProfileDir("a"), profile.MCPFile)
	writeJSON(t, file, `{"mcpServers":{"shared":{"command":"x"}}}`)
	abs, _ := filepath.Abs(file)
	for _, args := range [][]string{
		{"run", "a", "--account", "c1", "--", bin, "mcp", "list"},
		{"run", "a", "--", bin, "mcp", "list"},
	} {
		if err := runCcp(t, args...); err != nil {
			t.Fatal(err)
		}
		got := recordedArgs(t, out)
		if len(got) != 3 || got[0] != "--mcp-config="+abs || got[1] != "mcp" || got[2] != "list" {
			t.Errorf("%v: %q", args, got)
		}
	}
	// Only claude gets it.
	other := filepath.Join(filepath.Dir(bin), "notclaude")
	os.WriteFile(other, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+out+"\n"), 0o755)
	if err := runCcp(t, "run", "a", "--account", "c1", "--", other, "x"); err != nil {
		t.Fatal(err)
	}
	if got := recordedArgs(t, out); strings.Join(got, " ") != "x" {
		t.Errorf("other program: %q", got)
	}
}

// A server only in the account's own state is reported (read-only); one also
// in mcp.json is not. Neither file is changed.
func TestRun_ReportsAccountOnlyMCP(t *testing.T) {
	paths, _ := poolEnv(t)
	bin, _ := fakeClaude(t)
	writeJSON(t, filepath.Join(paths.ProfileDir("a"), profile.MCPFile), `{"mcpServers":{"shared":{"command":"x"}}}`)
	state := filepath.Join(paths.AccountDir("c1"), ".claude.json")
	body := `{"mcpServers":{"shared":{"command":"y"},"mine":{"command":"z"}}}`
	writeJSON(t, state, body)
	stderr, err := stderrOf(t, func() error { return runCcp(t, "run", "a", "--account", "c1", "--", bin) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "MCP servers only on account c1: mine;") || strings.Contains(stderr, "shared") {
		t.Errorf("stderr: %q", stderr)
	}
	if data, _ := os.ReadFile(state); string(data) != body {
		t.Errorf("account state changed: %s", data)
	}
}

// doctor says what is missing, and --fix creates mcp.json (0600) from the
// profile's own servers without touching the profile's state.
func TestDoctor_MCPNotesAndFix(t *testing.T) {
	paths, _ := poolEnv(t)
	writeJSON(t, profile.ProfileStatePath(paths, paths.ProfileDir("a")), `{"mcpServers":{"s1":{"command":"x","env":{"T":"secret"}}},"oauthAccount":{"x":1}}`)
	notes := strings.Join(mcpNotes(paths, false), "\n")
	if !strings.Contains(notes, "profile a has no mcp.json") {
		t.Errorf("notes: %s", notes)
	}
	notes = strings.Join(mcpNotes(paths, true), "\n")
	file, ok := profile.ProfileMCPPath(paths, "a")
	if !ok || !strings.Contains(notes, "created") {
		t.Fatalf("fix: %v\n%s", ok, notes)
	}
	info, _ := os.Stat(file)
	data, _ := os.ReadFile(file)
	if info.Mode().Perm() != 0o600 || !strings.Contains(string(data), "s1") || strings.Contains(string(data), "oauthAccount") {
		t.Errorf("file %v:\n%s", info.Mode(), data)
	}
	if _, err := profile.CreateProfileMCP(paths, "a"); err == nil {
		t.Error("create over an existing file accepted")
	}
	_ = config.HubBundles
}
