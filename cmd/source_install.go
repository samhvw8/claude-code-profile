package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	Use:     "install [source] [items...]",
	Aliases: []string{"i", "add"},
	Short:   "Install items from a source or sync all from registry",
	Long: `Install specific items from a source to the hub.

If the source is not already added, it will be added automatically.
If no items are specified, interactive selection is shown.

When called without arguments, syncs all sources from ccp.toml:
- Clones missing sources
- Reinstalls items listed in registry

Examples:
  ccp source install                                  # Sync all from ccp.toml
  ccp source install remorses/playwriter              # Auto-add, interactive selection
  ccp source install owner/repo skills/my-skill
  ccp source install owner/repo --all`,
	Args: cobra.MinimumNArgs(0),
	RunE: runSourceInstall,
}

func init() {
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallAll, "all", "a", false, "Install all available items")
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallInteractive, "interactive", "i", false, "Interactive item selection")
	sourceCmd.AddCommand(sourceInstallCmd)
}

func runSourceInstall(cmd *cobra.Command, args []string) error {
	// No args = sync all from registry
	if len(args) == 0 {
		return runSourceSync()
	}

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
		// Source not found - try to add it first
		fmt.Printf("Source not found, adding: %s\n", sourceID)
		if addErr := addSourceForInstall(sourceID, paths, registry); addErr != nil {
			return fmt.Errorf("failed to add source: %w", addErr)
		}
		// Re-fetch source after adding
		src, err = registry.GetSource(sourceID)
		if err != nil {
			return fmt.Errorf("source not found after add: %s", sourceID)
		}
	}

	installer := source.NewInstaller(paths, registry)
	available := installer.DiscoverItems(src.Path)

	if sourceInstallAll {
		items = available
	}

	// Auto-interactive when no items specified and not --all
	if len(items) == 0 && !sourceInstallAll {
		sourceInstallInteractive = true
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
		fmt.Printf("Install with: ccp install %s <item>\n", sourceID)
		fmt.Printf("Or use: ccp install %s -i (interactive)\n", sourceID)
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

// addSourceForInstall adds a source when it's not found during install
func addSourceForInstall(identifier string, paths *config.Paths, registry *source.Registry) error {
	ctx := context.Background()

	var details *source.PackageDetails
	var provider source.Provider
	var url, ref string

	if isDirectURL(identifier) {
		url = identifier
		provider = source.DetectProvider(url)
		if provider == nil {
			return fmt.Errorf("cannot determine provider for: %s", url)
		}
	} else {
		reg := source.DetectRegistry(identifier)
		if reg == nil {
			reg = source.DefaultRegistry()
		}

		fmt.Printf("Looking up %s in %s...\n", identifier, reg.Name())

		var err error
		details, err = reg.Get(ctx, identifier)
		if err != nil {
			// If skills.sh fails for owner/repo format, try GitHub as fallback
			if reg.Name() == "skills.sh" && strings.Contains(identifier, "/") && !strings.Contains(identifier, "@") {
				githubReg := source.GetRegistryProvider("github")
				if githubReg != nil {
					fmt.Printf("Not found in skills.sh, trying GitHub...\n")
					details, err = githubReg.Get(ctx, identifier)
					if err != nil {
						return fmt.Errorf("package not found in skills.sh or GitHub: %s", identifier)
					}
				}
			}
			if details == nil {
				return fmt.Errorf("package not found: %w", err)
			}
		}

		url = details.DownloadURL
		provider = source.GetProvider(details.ProviderType)
		ref = details.Ref
	}

	sourceID := generateSourceID(identifier, url)

	if registry.HasSource(sourceID) {
		return nil // Already exists
	}

	sourceDir := paths.SourceDir(sourceID)

	fmt.Printf("Adding source: %s\n", sourceID)
	fmt.Printf("  Provider: %s\n", provider.Type())
	fmt.Printf("  URL: %s\n", url)
	if ref != "" {
		fmt.Printf("  Ref: %s\n", ref)
	}

	opts := source.FetchOptions{
		Ref:      ref,
		Progress: true,
	}
	if err := provider.Fetch(ctx, url, sourceDir, opts); err != nil {
		return err
	}

	var commit string
	if gitProvider, ok := provider.(*source.GitProvider); ok {
		commit = gitProvider.GetCommit(sourceDir)
	}

	registryName := "manual"
	if details != nil {
		registryName = details.Registry
	}

	src := source.Source{
		Registry: registryName,
		Provider: provider.Type(),
		URL:      url,
		Path:     sourceDir,
		Ref:      ref,
		Commit:   commit,
	}

	if err := registry.AddSource(sourceID, src); err != nil {
		return err
	}

	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Printf("Added source: %s\n", sourceID)
	return nil
}

// runSourceSync syncs all sources from ccp.toml
// For each source in registry:
// 1. Clone if missing
// 2. Reinstall items from Installed list
func runSourceSync() error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	sources := registry.ListSources()
	if len(sources) == 0 {
		fmt.Println("No sources in registry. Add sources with: ccp install <owner/repo>")
		return nil
	}

	fmt.Printf("Syncing %d sources from registry...\n\n", len(sources))

	installer := source.NewInstaller(paths, registry)
	ctx := context.Background()

	var totalCloned, totalInstalled int

	for _, entry := range sources {
		fmt.Printf("Source: %s\n", entry.ID)

		// Check if source directory exists
		sourceDir := paths.SourceDir(entry.ID)
		needsClone := false

		if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
			needsClone = true
		}

		// Clone if missing
		if needsClone {
			provider := source.GetProvider(entry.Source.Provider)
			if provider == nil {
				fmt.Printf("  ⚠ Unknown provider: %s, skipping\n", entry.Source.Provider)
				continue
			}

			fmt.Printf("  Cloning from %s...\n", entry.Source.URL)
			opts := source.FetchOptions{
				Ref:      entry.Source.Ref,
				Progress: false,
			}
			if err := provider.Fetch(ctx, entry.Source.URL, sourceDir, opts); err != nil {
				fmt.Printf("  ⚠ Clone failed: %v\n", err)
				continue
			}

			// Update path in registry (in case it changed)
			src := entry.Source
			src.Path = sourceDir
			registry.UpdateSource(entry.ID, src)
			totalCloned++
			fmt.Printf("  ✓ Cloned\n")
		} else {
			fmt.Printf("  ✓ Source exists\n")
		}

		// Reinstall items if any are missing from hub
		if len(entry.Source.Installed) > 0 {
			var missing []string
			for _, item := range entry.Source.Installed {
				parts := strings.SplitN(item, "/", 2)
				if len(parts) != 2 {
					continue
				}
				itemPath := paths.HubDir + "/" + item
				if _, err := os.Stat(itemPath); os.IsNotExist(err) {
					missing = append(missing, item)
				}
			}

			if len(missing) > 0 {
				fmt.Printf("  Installing %d missing items...\n", len(missing))

				// Remove items from registry first so Install can add them back
				for _, item := range missing {
					registry.RemoveInstalled(entry.ID, item)
				}

				installed, err := installer.Install(entry.ID, missing)
				if err != nil {
					fmt.Printf("  ⚠ Install failed: %v\n", err)
				} else {
					for _, item := range installed {
						fmt.Printf("    + %s\n", item)
					}
					totalInstalled += len(installed)
				}
			} else {
				fmt.Printf("  ✓ All %d items present\n", len(entry.Source.Installed))
			}
		}
		fmt.Println()
	}

	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Printf("Sync complete: %d sources cloned, %d items installed\n", totalCloned, totalInstalled)
	return nil
}
