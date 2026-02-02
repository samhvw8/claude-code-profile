package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/source"
)

var (
	sourceFindRegistry string
	sourceFindLimit    int
)

var sourceFindCmd = &cobra.Command{
	Use:     "find <query>",
	Aliases: []string{"f", "search"},
	Short:   "Search for packages in registries",
	Long: `Search skills.sh and other registries for packages.

Examples:
  ccp source find debugging
  ccp source find --registry=github debugging
  ccp source find --limit=5 react`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSourceFind,
}

func init() {
	sourceFindCmd.Flags().StringVarP(&sourceFindRegistry, "registry", "r", "", "Registry to search (skills.sh, github)")
	sourceFindCmd.Flags().IntVarP(&sourceFindLimit, "limit", "l", 10, "Maximum results")
	sourceCmd.AddCommand(sourceFindCmd)
}

func runSourceFind(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	var reg source.RegistryProvider
	if sourceFindRegistry != "" {
		reg = source.GetRegistryProvider(sourceFindRegistry)
		if reg == nil {
			return fmt.Errorf("unknown registry: %s", sourceFindRegistry)
		}
	} else {
		reg = source.DefaultRegistry()
	}

	opts := source.SearchOptions{Limit: sourceFindLimit}
	packages, err := reg.Search(ctx, query, opts)
	if err != nil {
		return err
	}

	if len(packages) == 0 {
		fmt.Println("No packages found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Show different columns based on registry
	hasSkillName := false
	for _, pkg := range packages {
		if pkg.Name != "" && pkg.Name != pkg.ID {
			hasSkillName = true
			break
		}
	}

	if hasSkillName {
		fmt.Fprintf(w, "PACKAGE\tSKILL\tDESCRIPTION\n")
		for _, pkg := range packages {
			desc := pkg.Description
			if len(desc) > 40 {
				desc = desc[:37] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", pkg.ID, pkg.Name, desc)
		}
	} else {
		fmt.Fprintf(w, "PACKAGE\tDESCRIPTION\tVERSION\n")
		for _, pkg := range packages {
			desc := pkg.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", pkg.ID, desc, pkg.Version)
		}
	}
	w.Flush()

	fmt.Println()
	fmt.Println("Install with: ccp install <package>")
	if hasSkillName {
		fmt.Println("  (skill name is informational - use PACKAGE column)")
	}

	return nil
}
