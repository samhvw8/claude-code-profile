package cmd

import (
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:     "profile",
	Aliases: []string{"p"},
	Short:   "Manage profiles",
	Long:    `Create, list, check, fix, and delete profiles.`,
}

func init() {
	rootCmd.AddCommand(profileCmd)
}
