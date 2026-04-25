package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/source"
)

var (
	projectInstallAll         bool
	projectInstallInteractive bool
)

var projectInstallCmd = &cobra.Command{
	Use:     "install [source] [items...]",
	Aliases: []string{"i"},
	Short:   "Install items from a source directly into the project",
	Long: `Install items from a source (GitHub repo or skills.sh) into the project's .claude/ directory.

If the source is not already added, it will be added automatically.
If no items are specified, interactive selection is shown.

Items are copied (not symlinked), so they become local to the project.
Settings-templates are excluded from project installs.

Examples:
  ccp project install remorses/playwriter              # Interactive selection
  ccp project install owner/repo skills/my-skill       # Specific item
  ccp project install owner/repo --all                 # All items
  ccp project install owner/repo --dir /path/to/proj   # Specify project root`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectInstallArgs,
	RunE:              runProjectInstall,
}

func init() {
	projectInstallCmd.Flags().BoolVarP(&projectInstallAll, "all", "a", false, "Install all available items")
	projectInstallCmd.Flags().BoolVarP(&projectInstallInteractive, "interactive", "i", false, "Interactive item selection")
	projectCmd.AddCommand(projectInstallCmd)
}

func projectAllowedTypes() map[string]bool {
	m := make(map[string]bool)
	for _, t := range projectHubItemTypes {
		m[string(t)] = true
	}
	return m
}

func runProjectInstall(cmd *cobra.Command, args []string) error {
	sourceID := args[0]
	items := args[1:]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	claudeDir, err := findProjectClaudeDir(projectDirFlag)
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	src, err := registry.GetSource(sourceID)
	if err != nil {
		fmt.Printf("Source not found, adding: %s\n", sourceID)
		if addErr := addSourceForInstall(sourceID, paths, registry); addErr != nil {
			return fmt.Errorf("failed to add source: %w", addErr)
		}
		src, err = registry.GetSource(sourceID)
		if err != nil {
			return fmt.Errorf("source not found after add: %s", sourceID)
		}
	}
	_ = src

	installer := source.NewInstaller(paths, registry)
	available := installer.DiscoverItems(paths.SourceDir(sourceID))

	// Filter out settings-templates from available items
	allowed := projectAllowedTypes()
	var filtered []string
	for _, item := range available {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) == 2 && allowed[parts[0]] {
			filtered = append(filtered, item)
		}
	}
	available = filtered

	if projectInstallAll {
		items = available
	}

	if len(items) == 0 && !projectInstallAll {
		projectInstallInteractive = true
	}

	if projectInstallInteractive && len(items) == 0 {
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

		selected, err := picker.Run("Select items to install into project", pickerItems)
		if err != nil {
			return fmt.Errorf("selection failed: %w", err)
		}
		if len(selected) == 0 {
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
		fmt.Printf("Install with: ccp project install %s <item>\n", sourceID)
		fmt.Printf("Or use: ccp project install %s -i (interactive)\n", sourceID)
		return nil
	}

	installed, err := installer.InstallToDir(sourceID, items, claudeDir, allowed)
	if err != nil {
		return err
	}

	fmt.Printf("Installed %d item(s) into %s:\n", len(installed), claudeDir)
	for _, item := range installed {
		fmt.Printf("  - %s\n", item)
	}

	return nil
}

// completeProjectInstallArgs provides tab completion for project install
func completeProjectInstallArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Complete with known source IDs
		paths, err := config.ResolvePaths()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		registry, err := source.LoadRegistry(paths.RegistryPath())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var ids []string
		for _, entry := range registry.ListSources() {
			if strings.HasPrefix(entry.ID, toComplete) {
				ids = append(ids, entry.ID)
			}
		}
		return ids, cobra.ShellCompDirectiveNoFileComp
	}

	if len(args) == 1 {
		// Complete with discovered items from the source
		paths, err := config.ResolvePaths()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		registry, err := source.LoadRegistry(paths.RegistryPath())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		_, err = registry.GetSource(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		installer := source.NewInstaller(paths, registry)
		available := installer.DiscoverItems(paths.SourceDir(args[0]))

		allowed := projectAllowedTypes()
		var items []string
		for _, item := range available {
			parts := strings.SplitN(item, "/", 2)
			if len(parts) == 2 && allowed[parts[0]] {
				if strings.HasPrefix(item, toComplete) {
					items = append(items, item)
				}
			}
		}
		return items, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}

