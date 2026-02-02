package cmd

import (
	"github.com/spf13/cobra"
)

var (
	findRegistry string
	findLimit    int
)

var findCmd = &cobra.Command{
	Use:     "find <query>",
	Aliases: []string{"search"},
	Short:   "Search for packages in registries",
	Long: `Search skills.sh and other registries for packages.

Examples:
  ccp find debugging
  ccp find --registry=github debugging
  ccp find --limit=5 react`,
	Args: cobra.MinimumNArgs(1),
	RunE: runFind,
}

func init() {
	findCmd.Flags().StringVarP(&findRegistry, "registry", "r", "", "Registry to search (skills.sh, github)")
	findCmd.Flags().IntVarP(&findLimit, "limit", "l", 10, "Maximum results")
	rootCmd.AddCommand(findCmd)
}

// runFind wraps runSourceFind but uses find-specific flags
func runFind(cmd *cobra.Command, args []string) error {
	// Copy find flags to source find flags for the shared function
	sourceFindRegistry = findRegistry
	sourceFindLimit = findLimit
	return runSourceFind(cmd, args)
}
