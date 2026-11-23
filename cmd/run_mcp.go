package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

// withProfileMCP adds --mcp-config=<profile>/mcp.json right after the program
// when the command is claude and the profile has that file. The = form takes
// exactly one value; the spaced form is variadic and would swallow a
// subcommand or a prompt that follows it.
func withProfileMCP(paths *config.Paths, profileName string, cmdArgs []string) []string {
	// A native install may be started by its version path
	// (~/.local/share/claude/versions/<ver>).
	isClaude := filepath.Base(cmdArgs[0]) == "claude" || strings.Contains(filepath.ToSlash(cmdArgs[0]), "/claude/versions/")
	if profileName == "" || !isClaude {
		return cmdArgs
	}
	file, ok := profile.ProfileMCPPath(paths, profileName)
	if !ok {
		return cmdArgs
	}
	return append([]string{cmdArgs[0], "--mcp-config=" + file}, cmdArgs[1:]...)
}

// reportAccountOnlyMCP prints, read-only, the MCP servers a Claude account
// has in its own state that the profile's MCP file does not list: those stay
// on this account. A read error only warns.
func reportAccountOnlyMCP(paths *config.Paths, profileName, account, accountDir string) {
	only, err := profile.AccountOnlyMCP(paths, profileName, accountDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not compare MCP servers of account %s: %v\n", account, err)
		return
	}
	if len(only) == 0 {
		return
	}
	file, _ := profile.ProfileMCPPath(paths, profileName)
	fmt.Fprintf(os.Stderr, "ccp: MCP servers only on account %s: %s; add them to %s to share them with every account of %s\n",
		account, strings.Join(only, ", "), file, profileName)
}
