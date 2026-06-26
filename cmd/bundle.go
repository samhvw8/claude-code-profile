package cmd

import (
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:     "bundle",
	Aliases: []string{"bdl"},
	Short:   "Manage bundles (atomic groups of skills, agents, hooks)",
	Long: `Manage bundles — atomic, non-separable groups of hub items.

A bundle packages coupled items (for example a skill, its hook, and its agent)
that must travel together. Members live inside the bundle and cannot be linked
on their own; the bundle is linked, unlinked, and removed as a single unit.`,
}

func init() {
	rootCmd.AddCommand(bundleCmd)
}
