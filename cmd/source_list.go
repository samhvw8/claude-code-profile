package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

var sourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sources",
	RunE:  runSourceList,
}

func init() {
	sourceCmd.AddCommand(sourceListCmd)
}

func runSourceList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	entries := registry.ListSources()
	if len(entries) == 0 {
		fmt.Println("No sources installed")
		fmt.Println()
		fmt.Println("Add a source with:")
		fmt.Println("  ccp source add <package>")
		fmt.Println("  ccp source find <query>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SOURCE\tPROVIDER\tINSTALLED\tUPDATED\n")

	for _, entry := range entries {
		updated := entry.Source.Updated.Format("2006-01-02")
		fmt.Fprintf(w, "%s\t%s\t%d items\t%s\n",
			entry.ID,
			entry.Source.Provider,
			len(entry.Source.Installed),
			updated,
		)
	}
	w.Flush()

	return nil
}
