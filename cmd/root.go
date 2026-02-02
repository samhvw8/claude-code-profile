package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "ccp",
	Short: "Claude Code Profile manager",
	Long: `ccp (Claude Code Profile) manages a central hub of reusable components
and multiple profiles for Claude Code. Each profile is a complete Claude Code
configuration directory, activated via CLAUDE_CONFIG_DIR or symlink.`,
	Version: Version,
	Run:     runRoot,
}

func runRoot(cmd *cobra.Command, args []string) {
	// Check if ccp is initialized
	paths, err := config.ResolvePaths()
	if err != nil {
		cmd.Help()
		return
	}

	if !paths.IsInitialized() {
		fmt.Println("ccp - Claude Code Profile Manager")
		fmt.Println()
		fmt.Println("Not initialized. Get started with:")
		fmt.Println()
		fmt.Println("  ccp init       Initialize ccp (migrates existing ~/.claude)")
		fmt.Println("  ccp --help     Show all commands")
		fmt.Println()
		fmt.Println("Learn more: https://github.com/samhvw8/claude-code-profile")
		return
	}

	// If initialized, show help
	cmd.Help()
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}
