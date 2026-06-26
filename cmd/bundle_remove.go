package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
)

var bundleRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a bundle from the hub",
	Args:    cobra.ExactArgs(1),
	RunE:    runBundleRemove,
}

func init() {
	bundleCmd.AddCommand(bundleRemoveCmd)
}

func runBundleRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	bundleDir := paths.BundleDir(name)
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		return fmt.Errorf("bundle not found: %s", name)
	}

	if err := os.RemoveAll(bundleDir); err != nil {
		return fmt.Errorf("failed to remove bundle: %w", err)
	}

	fmt.Printf("Removed bundle '%s'\n", name)
	fmt.Println("Note: profiles that linked this bundle will now show drift (run 'ccp doctor').")
	return nil
}
