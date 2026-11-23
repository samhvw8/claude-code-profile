package profile

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Claude Code 2.1 keeps state in a non-empty .config.json: its servers count
// as the account's own too.
func TestAccountOnlyMCP_ReadsConfigJSON(t *testing.T) {
	paths := accountTestPaths(t, "p")
	profileDir := paths.ProfileDir("p")
	acct := t.TempDir()
	if got, err := AccountOnlyMCP(paths, "p", acct); got != nil || err != nil {
		t.Errorf("no mcp.json: %v %v", got, err)
	}
	os.WriteFile(filepath.Join(profileDir, MCPFile), []byte(`{"mcpServers":{"a":{}}}`), 0o600)
	os.WriteFile(filepath.Join(acct, ".claude.json"), []byte(`{"mcpServers":{"a":{},"b":{}}}`), 0o600)
	os.WriteFile(filepath.Join(acct, ".config.json"), []byte(`{"mcpServers":{"c":{}}}`), 0o600)
	got, err := AccountOnlyMCP(paths, "p", acct)
	if err != nil || !slices.Equal(got, []string{"b", "c"}) {
		t.Errorf("got %v %v", got, err)
	}
	os.WriteFile(filepath.Join(acct, ".claude.json"), []byte("{"), 0o600)
	if _, err := AccountOnlyMCP(paths, "p", acct); err == nil {
		t.Error("invalid state accepted")
	}
}
