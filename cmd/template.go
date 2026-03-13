package cmd

import (
	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl", "t"},
	Short:   "Manage settings templates",
	Long:    `Manage settings.json templates that can be applied to profiles and engines.`,
}

func init() {
	rootCmd.AddCommand(templateCmd)
}
