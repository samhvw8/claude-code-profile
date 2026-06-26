package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var bundleListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List bundles in the hub",
	RunE:    runBundleList,
}

func init() {
	bundleCmd.AddCommand(bundleListCmd)
}

func runBundleList(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	bundles, err := hub.ListBundles(paths.BundlesDir())
	if err != nil {
		return fmt.Errorf("failed to list bundles: %w", err)
	}
	if len(bundles) == 0 {
		fmt.Println("No bundles found")
		fmt.Println("\nCreate one with:")
		fmt.Println("  ccp bundle create <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tMEMBERS\tDESCRIPTION")
	for _, b := range bundles {
		version := b.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", b.Name, version, b.Members.Count(), b.Description)
	}
	w.Flush()
	return nil
}
