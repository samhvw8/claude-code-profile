package cmd

import (
	"github.com/spf13/cobra"
)

var engineCmd = &cobra.Command{
	Use:     "engine",
	Aliases: []string{"e"},
	Short:   "Manage engines (runtime config layers)",
	Long:    `Engines define Claude runtime behavior: settings, hooks, data sharing, and permissions.`,
}

func init() {
	rootCmd.AddCommand(engineCmd)
}
