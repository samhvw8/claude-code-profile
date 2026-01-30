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
  - claude alias: Loads profile's CLAUDE.md and rules via --add-dir
  - Automatic CLAUDE_CONFIG_DIR from ccp which --path`,
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

	// Claude alias with --add-dir for loading profile CLAUDE.md/rules
	fmt.Println("# Claude alias - loads profile's CLAUDE.md and rules")
	fmt.Println(`alias claude='CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1 command claude --add-dir "${CLAUDE_CONFIG_DIR:-$(ccp which --path 2>/dev/null)}"'`)
	fmt.Println()

	// Optional: ccp cd function
	fmt.Println("# Quick profile switch (optional)")
	fmt.Println("ccp-use() {")
	fmt.Println(`  ccp use "$@"`)
	fmt.Println("  # Reload mise env if available")
	fmt.Println("  if command -v mise &> /dev/null && [[ -f mise.toml ]]; then")
	fmt.Println("    eval \"$(mise env)\"")
	fmt.Println("  fi")
	fmt.Println("}")
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
