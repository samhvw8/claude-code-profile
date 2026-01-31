package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/source"
)

var (
	sourceInstallAll         bool
	sourceInstallInteractive bool
)

var sourceInstallCmd = &cobra.Command{
	Use:   "install <source> [items...]",
	Short: "Install items from a source",
	Long: `Install specific items from a source to the hub.

Examples:
  ccp source install samhoang/skills skills/debugging
  ccp source install samhoang/skills skills/debugging commands/debug
  ccp source install samhoang/skills --all
  ccp source install samhoang/skills -i  # Interactive selection`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSourceInstall,
}

func init() {
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallAll, "all", "a", false, "Install all available items")
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallInteractive, "interactive", "i", false, "Interactive item selection")
	sourceCmd.AddCommand(sourceInstallCmd)
}

func runSourceInstall(cmd *cobra.Command, args []string) error {
	sourceID := args[0]
	items := args[1:]

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
		return fmt.Errorf("source not found: %s\n  Add it first with: ccp source add %s", sourceID, sourceID)
	}

	installer := source.NewInstaller(paths, registry)
	available := installer.DiscoverItems(src.Path)

	if sourceInstallAll {
		items = available
	}

	// Interactive selection
	if sourceInstallInteractive && len(items) == 0 {
		if len(available) == 0 {
			return fmt.Errorf("no items found in source")
		}

		var pickerItems []picker.Item
		for _, item := range available {
			pickerItems = append(pickerItems, picker.Item{
				ID:    item,
				Label: item,
			})
		}

		selected, err := picker.Run("Select items to install", pickerItems)
		if err != nil {
			return fmt.Errorf("selection failed: %w", err)
		}
		if selected == nil || len(selected) == 0 {
			fmt.Println("Cancelled")
			return nil
		}
		items = selected
	}

	if len(items) == 0 {
		if len(available) == 0 {
			return fmt.Errorf("no items found in source")
		}
		fmt.Println("Available items:")
		for _, item := range available {
			fmt.Printf("  - %s\n", item)
		}
		fmt.Println()
		fmt.Printf("Install with: ccp source install %s <item>\n", sourceID)
		fmt.Printf("Or use: ccp source install %s -i (interactive)\n", sourceID)
		return nil
	}

	installed, err := installer.Install(sourceID, items)
	if err != nil {
		return err
	}

	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Printf("Installed %d items from %s:\n", len(installed), sourceID)
	for _, item := range installed {
		fmt.Printf("  - %s\n", item)
	}

	fmt.Println()
	fmt.Println("Link to profile with:")
	for _, item := range installed {
		fmt.Printf("  ccp link <profile> %s\n", item)
	}

	return nil
}
