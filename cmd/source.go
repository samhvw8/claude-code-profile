package cmd

import (
	"github.com/spf13/cobra"
)

var sourceCmd = &cobra.Command{
	Use:     "source",
	Aliases: []string{"s", "src"},
	Short:   "Manage sources (skills, plugins, tools)",
	Long: `Manage external sources for skills, agents, commands, and other components.

Sources can be added from:
  - skills.sh registry (default)
  - GitHub repositories
  - Direct URLs (tar.gz, zip)

Examples:
  ccp source find debugging          # Search skills.sh
  ccp source add samhoang/skills     # Add from GitHub
  ccp source install samhoang/skills skills/debugging
  ccp source list                    # Show all sources
  ccp source update                  # Update all sources`,
}

func init() {
	rootCmd.AddCommand(sourceCmd)
}
