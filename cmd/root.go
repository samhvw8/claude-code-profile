package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "ccp",
	Short: "Claude Code Profile manager",
	Long: `ccp (Claude Code Profile) manages a central hub of reusable components
and multiple profiles for Claude Code. Each profile is a complete Claude Code
configuration directory, activated via CLAUDE_CONFIG_DIR or symlink.`,
	Version: Version,
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
