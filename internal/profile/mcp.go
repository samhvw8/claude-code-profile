package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/samhvw8/claude-code-profile/internal/config"
)

// MCPFile is a profile's shared MCP server list, in Claude Code's
// --mcp-config format ({"mcpServers": {...}}). ccp run passes it to every
// Claude Code it starts on the profile, with or without an account, so a
// server listed there reaches every account; on a name clash it wins over the
// account's own .claude.json entry. ccp never writes Claude Code's state
// files: servers added with 'claude mcp add' stay in the account that added
// them, and ccp only reports them.
const MCPFile = "mcp.json"

// ProfileStatePath returns the .claude.json Claude Code uses for a profile:
// ~/.claude.json when ~/.claude resolves to the profile (global activation),
// else <profile>/.claude.json (CLAUDE_CONFIG_DIR).
func ProfileStatePath(paths *config.Paths, profileDir string) string {
	if global, err := filepath.EvalSymlinks(paths.GlobalClaudeDir); err == nil {
		if real, err := filepath.EvalSymlinks(profileDir); err == nil && global == real {
			return filepath.Join(filepath.Dir(paths.GlobalClaudeDir), claudeStateFile)
		}
	}
	return filepath.Join(profileDir, claudeStateFile)
}

// readMCPServers returns the mcpServers object of a JSON file. A missing file
// is an empty map and no error.
func readMCPServers(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	if doc.MCPServers == nil {
		doc.MCPServers = map[string]json.RawMessage{}
	}
	return doc.MCPServers, nil
}

// ProfileMCPPath returns the profile's MCP file and whether it exists.
func ProfileMCPPath(paths *config.Paths, profileName string) (string, bool) {
	p := filepath.Join(paths.ProfileDir(profileName), MCPFile)
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	info, err := os.Stat(p)
	return p, err == nil && !info.IsDir()
}

// AccountOnlyMCP returns the user-scope MCP servers of a Claude account
// (its .claude.json, and .config.json when Claude Code uses that one) that
// the profile's MCP file does not list, sorted. It only reads; nil when the
// profile has no MCP file.
func AccountOnlyMCP(paths *config.Paths, profileName, accountDir string) ([]string, error) {
	file, ok := ProfileMCPPath(paths, profileName)
	if !ok {
		return nil, nil
	}
	shared, err := readMCPServers(file)
	if err != nil {
		return nil, err
	}
	own := map[string]json.RawMessage{}
	for _, name := range []string{claudeStateFile, ".config.json"} {
		path := filepath.Join(accountDir, name)
		if name == ".config.json" && !NonEmptyConfigJSON(path) {
			continue
		}
		m, err := readMCPServers(path)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			own[k] = v
		}
	}
	var only []string
	for _, k := range sortedKeys(own) {
		if _, ok := shared[k]; !ok {
			only = append(only, k)
		}
	}
	return only, nil
}

// CreateProfileMCP writes the profile's MCP file (0600: entries often carry
// tokens in env or headers) from the servers in the profile's own Claude Code
// state. It refuses when the file exists and returns the server names copied.
// Claude Code's state is only read.
func CreateProfileMCP(paths *config.Paths, profileName string) ([]string, error) {
	file, exists := ProfileMCPPath(paths, profileName)
	if exists {
		return nil, fmt.Errorf("%s already exists", file)
	}
	servers, err := readMCPServers(ProfileStatePath(paths, paths.ProfileDir(profileName)))
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		os.Remove(file)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(file)
		return nil, err
	}
	return sortedKeys(servers), nil
}
