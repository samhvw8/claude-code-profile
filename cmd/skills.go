package cmd

import (
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Discover and install skills from skills.sh",
	Long: `Discover and install skills from the open agent skills ecosystem.

Skills are modular packages that extend Claude Code capabilities with
specialized knowledge, workflows, and tools.

Browse skills at: https://skills.sh/`,
}

func init() {
	rootCmd.AddCommand(skillsCmd)
}
