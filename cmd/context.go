package cmd

import (
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:     "context",
	Aliases: []string{"ctx"},
	Short:   "Manage contexts (prompt/capability layers)",
	Long:    `Contexts define what Claude knows: skills, agents, rules, and commands.`,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}
