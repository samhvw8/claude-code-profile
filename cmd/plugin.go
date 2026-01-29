package cmd

import (
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Install plugins from Claude Code marketplaces",
	Long: `Install and manage plugins from Claude Code marketplace repositories.

A marketplace is a GitHub repository containing a .claude-plugin/marketplace.json
file that lists available plugins.

Each plugin can contain agents, commands, skills, and other Claude Code components
that get installed to your hub.

Example marketplaces:
  EveryInc/compound-engineering-plugin`,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}
