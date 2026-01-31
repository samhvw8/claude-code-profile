package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/source"
)

var (
	sourceAddRef      string
	sourceAddProvider string
	sourceAddInstall  bool
)

var sourceAddCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Add a source from registry or URL",
	Long: `Add a source by downloading/cloning it to the sources directory.

The source can be:
  - A package ID (owner/repo) - looks up skills.sh, falls back to GitHub
  - A GitHub URL (github.com/user/repo)
  - A direct download URL (https://example.com/package.tar.gz)

When using owner/repo format without @ref:
  - First tries skills.sh registry
  - Falls back to GitHub with default branch if not found

Examples:
  ccp source add remorses/playwriter          # Auto-fallback to GitHub
  ccp source add owner/repo --ref=v1.0
  ccp source add https://example.com/tool.tar.gz
  ccp source add owner/repo --install         # Add and install all items`,
	Args: cobra.ExactArgs(1),
	RunE: runSourceAdd,
}

func init() {
	sourceAddCmd.Flags().StringVar(&sourceAddRef, "ref", "", "Git branch/tag or version")
	sourceAddCmd.Flags().StringVar(&sourceAddProvider, "provider", "", "Force provider (git, http)")
	sourceAddCmd.Flags().BoolVarP(&sourceAddInstall, "install", "i", false, "Install all items after adding")
	sourceCmd.AddCommand(sourceAddCmd)
}

func runSourceAdd(cmd *cobra.Command, args []string) error {
	identifier := args[0]
	ctx := context.Background()

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	registry, err := source.LoadRegistry(paths.RegistryPath())
	if err != nil {
		return err
	}

	var details *source.PackageDetails
	var provider source.Provider
	var url, ref string

	if isDirectURL(identifier) {
		url = identifier
		if sourceAddProvider != "" {
			provider = source.GetProvider(sourceAddProvider)
		} else {
			provider = source.DetectProvider(url)
		}
		if provider == nil {
			return fmt.Errorf("cannot determine provider for: %s", url)
		}
		ref = sourceAddRef
	} else {
		reg := source.DetectRegistry(identifier)
		if reg == nil {
			reg = source.DefaultRegistry()
		}

		fmt.Printf("Looking up %s in %s...\n", identifier, reg.Name())

		details, err = reg.Get(ctx, identifier)
		if err != nil {
			// If skills.sh fails for owner/repo format, try GitHub as fallback
			if reg.Name() == "skills.sh" && strings.Contains(identifier, "/") && !strings.Contains(identifier, "@") {
				githubReg := source.GetRegistryProvider("github")
				if githubReg != nil {
					fmt.Printf("Not found in skills.sh, trying GitHub...\n")
					// Add @main to use default branch
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
		if sourceAddRef != "" {
			ref = sourceAddRef
		}
	}

	sourceID := generateSourceID(identifier, url)

	if registry.HasSource(sourceID) {
		fmt.Printf("Source already added: %s\n", sourceID)
		fmt.Println("Use 'ccp source update' to update or 'ccp source install' to install items")
		return nil
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

	installer := source.NewInstaller(paths, registry)
	available := installer.DiscoverItems(sourceDir)

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

	fmt.Println()
	fmt.Printf("Added source: %s\n", sourceID)
	fmt.Printf("Available items: %d\n", len(available))

	for _, item := range available {
		fmt.Printf("  - %s\n", item)
	}

	if sourceAddInstall && len(available) > 0 {
		fmt.Println()
		fmt.Println("Installing all items...")
		installed, err := installer.Install(sourceID, available)
		if err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}
		fmt.Printf("Installed %d items\n", len(installed))
	} else if len(available) > 0 {
		fmt.Println()
		fmt.Println("Install items with:")
		fmt.Printf("  ccp source install %s <item>\n", sourceID)
		fmt.Printf("  ccp source install %s --all\n", sourceID)
	}

	return nil
}

func isDirectURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@")
}

func generateSourceID(identifier, url string) string {
	if strings.Contains(identifier, "/") && !strings.Contains(identifier, "://") {
		return strings.TrimPrefix(identifier, "skills.sh/")
	}

	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "github.com/")
	url = strings.TrimPrefix(url, "gitlab.com/")
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, ".tar.gz")
	url = strings.TrimSuffix(url, ".zip")

	return url
}
