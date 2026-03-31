package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins (deprecated: use 'source')",
	Long: `DEPRECATED: Use 'ccp source' instead.

  ccp plugin add    →  ccp source add
  ccp plugin list   →  ccp source list
  ccp plugin update →  ccp source update`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		printDeprecationNotice("plugin", "source")
	},
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}

func printDeprecationNotice(old, new string) {
	fmt.Printf("Warning: 'ccp %s' is deprecated. Use 'ccp %s' instead.\n\n", old, new)
}
