package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
)

var bundleShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a bundle's members",
	Args:  cobra.ExactArgs(1),
	RunE:  runBundleShow,
}

func init() {
	bundleCmd.AddCommand(bundleShowCmd)
}

func runBundleShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	bundle, err := hub.LoadBundle(paths.BundlesDir(), name)
	if err != nil {
		return fmt.Errorf("bundle not found: %s", name)
	}

	fmt.Printf("Bundle: %s\n", bundle.Name)
	if bundle.Version != "" {
		fmt.Printf("Version: %s\n", bundle.Version)
	}
	if bundle.Description != "" {
		fmt.Printf("Description: %s\n", bundle.Description)
	}
	fmt.Printf("\nMembers (%d):\n", bundle.Members.Count())

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, m := range bundle.Members.AllComponents() {
		fmt.Fprintf(w, "  %s\t%s\n", m.Type, m.Name)
	}
	w.Flush()
	return nil
}
