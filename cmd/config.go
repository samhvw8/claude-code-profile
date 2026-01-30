package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration utilities",
	Long:  `Configuration utilities for ccp and Claude Code integration.`,
}

func init() {
	rootCmd.AddCommand(configCmd)
}
