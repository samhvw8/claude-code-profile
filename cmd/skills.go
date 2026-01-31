package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Discover and install skills (deprecated: use 'source')",
	Long: `DEPRECATED: Use 'ccp source' instead.

  ccp skills find    →  ccp source find
  ccp skills add     →  ccp source add
  ccp skills update  →  ccp source update

Browse skills at: https://skills.sh/`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		printDeprecationNotice("skills", "source")
	},
}

func init() {
	rootCmd.AddCommand(skillsCmd)
}

func printDeprecationNotice(old, new string) {
	fmt.Printf("Warning: 'ccp %s' is deprecated. Use 'ccp %s' instead.\n\n", old, new)
}
