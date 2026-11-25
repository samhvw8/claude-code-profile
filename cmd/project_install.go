package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/picker"
	"github.com/samhvw8/claude-code-profile/internal/source"
)

var (
	projectInstallAll         bool
	projectInstallInteractive bool
	projectInstallLink        bool
)

var projectInstallCmd = &cobra.Command{
	Use:     "install [source] [items...]",
	Aliases: []string{"i"},
	Short:   "Copy or link items from a source into the project",
	Long: `Install items from a source (GitHub repo or skills.sh) into the project's .claude/ directory.

If the source is not already added, it will be added automatically.
If no items are specified, interactive selection is shown.

Items are copied (not symlinked), so they become local to the project.
Settings-templates are excluded from project installs.

With --link, items are linked instead: absolute symlinks into the source clone
in ~/.ccp/sources, so 'ccp project update' (or 'ccp source update') updates them.
The source and items go into the project's own ccp.toml (its commit into
ccp.lock), not into ~/.ccp/ccp.toml, and ccp adds both files and the links to
.git/info/exclude: they are for this machine only. A real file or directory in
the way is never replaced.

With no arguments, it replays the project's ccp.toml: clones missing sources,
moves each clone to the project's locked commit, and recreates missing links.

Examples:
  ccp project install remorses/playwriter              # Interactive selection
  ccp project install owner/repo skills/my-skill       # Specific item
  ccp project install owner/repo --all                 # All items
  ccp project install owner/repo --dir /path/to/proj   # Specify project root
  ccp project install owner/repo skills/x --link       # Link, recorded in the project's ccp.toml
  ccp project install                                  # Recreate the links in the project's ccp.toml`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProjectInstallArgs,
	RunE:              runProjectInstall,
}

func init() {
	projectInstallCmd.Flags().BoolVarP(&projectInstallAll, "all", "a", false, "Install all available items")
	projectInstallCmd.Flags().BoolVarP(&projectInstallInteractive, "interactive", "i", false, "Interactive item selection")
	projectInstallCmd.Flags().BoolVarP(&projectInstallLink, "link", "l", false, "Link items from the source clone instead of copying, recorded in the project's ccp.toml")
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
	root := filepath.Dir(claudeDir)

	if len(args) == 0 {
		return runProjectSync(paths, root)
	}
	sourceID := args[0]
	items := args[1:]

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	// A linked install records its source in the project's ccp.toml, so the
	// clone is found and updated from there, never from ~/.ccp/ccp.toml.
	var project *source.Registry
	if projectInstallLink {
		project, err = source.LoadProjectRegistry(root, paths)
		if err != nil {
			return err
		}
		if project.ProjectFileForeign() {
			return fmt.Errorf("%s was not written by ccp: --link keeps its sources in that file, so move it away first", filepath.Join(root, source.ProjectFile))
		}
		if !project.HasSource(sourceID) {
			if sourceID, err = addProjectSource(sourceID, paths, project, registry); err != nil {
				return err
			}
			// Adding the source wrote ccp.toml and ccp.lock already: keep them
			// out of git even if no item gets linked below.
			if err := excludeProjectLinks(root, project); err != nil {
				fmt.Printf("WARN: could not update .git/info/exclude: %v\n", err)
			}
		}
		registry = project
	} else if !registry.HasSource(sourceID) {
		fmt.Printf("Source not found, adding: %s\n", sourceID)
		if addErr := addSourceForInstall(sourceID, "", paths, registry); addErr != nil {
			return fmt.Errorf("failed to add source: %w", addErr)
		}
		resolvedID := generateSourceID(sourceID, sourceID)
		if !registry.HasSource(resolvedID) {
			return fmt.Errorf("source not found after add: %s", sourceID)
		}
		sourceID = resolvedID
	}

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

	if project != nil {
		return linkProjectItems(paths, root, project, sourceID, items)
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

