package cmd

import (
	"github.com/spf13/cobra"
)

var hlCmd = &cobra.Command{
	Use:               "hl [profile]",
	Short:             "Add hub items to a profile (shorthand for 'hub link')",
	Long:              hubLinkCmd.Long,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeHubLinkArgs,
	RunE:              runHubLink,
}

func init() {
	rootCmd.AddCommand(hlCmd)
}
