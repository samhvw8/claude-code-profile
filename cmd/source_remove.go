package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

var sourceRemoveForce bool

var sourceRemoveCmd = &cobra.Command{
	Use:   "remove <source>",
	Short: "Remove a source",
	Long: `Remove a source and optionally its installed items.

Examples:
  ccp source remove samhoang/skills
  ccp source remove samhoang/skills --force  # Also remove installed items`,
	Args: cobra.ExactArgs(1),
	RunE: runSourceRemove,
}

func init() {
	sourceRemoveCmd.Flags().BoolVarP(&sourceRemoveForce, "force", "f", false, "Also remove installed items from hub")
	sourceCmd.AddCommand(sourceRemoveCmd)
}

func runSourceRemove(cmd *cobra.Command, args []string) error {
	sourceID := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	src, err := registry.GetSource(sourceID)
	if err != nil {
		return err
	}

	if len(src.Installed) > 0 && !sourceRemoveForce {
		fmt.Printf("Source has %d installed items:\n", len(src.Installed))
		for _, item := range src.Installed {
			fmt.Printf("  - %s\n", item)
		}
		fmt.Println()
		fmt.Println("Use --force to also remove installed items, or uninstall them first")
		return nil
	}

	if sourceRemoveForce && len(src.Installed) > 0 {
		installer := source.NewInstaller(paths, registry)
		if err := installer.Uninstall(src.Installed); err != nil {
			return err
		}
		fmt.Printf("Removed %d installed items\n", len(src.Installed))
	}

	if err := os.RemoveAll(src.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove source directory: %w", err)
	}

	if err := registry.RemoveSource(sourceID); err != nil {
		return err
	}

	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Printf("Removed source: %s\n", sourceID)
	return nil
}
