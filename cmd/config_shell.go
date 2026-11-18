package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var configShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Output shell configuration for Claude integration",
	Long: `Output shell aliases and functions for Claude Code integration.

Add the output to your shell configuration file (.bashrc, .zshrc, etc.):

  ccp config shell >> ~/.zshrc
  source ~/.zshrc

This configures:
  - ccp-use function: Quick profile switching with mise env reload

No claude alias is needed: Claude Code loads the profile's CLAUDE.md and
rules from CLAUDE_CONFIG_DIR (or the ~/.claude symlink) on its own.

The output is wrapped in a ccp existence check, making it safe for
synced shell configs across machines where ccp may not be installed.`,
	RunE: runConfigShell,
}

func init() {
	configCmd.AddCommand(configShellCmd)
}

func runConfigShell(cmd *cobra.Command, args []string) error {
	// Detect shell
	shell := detectShell()

	fmt.Printf("# ccp shell configuration (%s)\n", shell)
	fmt.Println("# Add this to your shell config file")
	fmt.Println()

	// Wrap in ccp existence check for synced shell configs
	fmt.Println("# Only load if ccp is installed")
	fmt.Println("if command -v ccp &> /dev/null; then")

	// Optional: ccp cd function
	fmt.Println("  # Quick profile switch (optional)")
	fmt.Println("  ccp-use() {")
	fmt.Println(`    ccp use "$@"`)
	fmt.Println("    # Reload mise env if available")
	fmt.Println("    if command -v mise &> /dev/null && [[ -f mise.toml ]]; then")
	fmt.Println("      eval \"$(mise env)\"")
	fmt.Println("    fi")
	fmt.Println("  }")
	fmt.Println("fi")
	fmt.Println()

	return nil
}

func detectShell() string {
	// Check SHELL env var
	shell := os.Getenv("SHELL")
	if shell != "" {
		return filepath.Base(shell)
	}
	return "sh"
}
