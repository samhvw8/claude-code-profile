package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/picker"
	"github.com/samhvw8/claude-code-profile/internal/profile"
	"github.com/samhvw8/claude-code-profile/internal/source"
)

var (
	sourceInstallAll         bool
	sourceInstallInteractive bool
	sourceInstallLink        string
	sourceInstallClone       bool
)

// activeProfileMarker is the --link value that means the active profile.
// --link always takes a value: an optional one would read "--link work" as a
// bare --link followed by an item named "work".
const activeProfileMarker = "."

// addInstallLinkFlag registers --link and --clone on an install command.
func addInstallLinkFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&sourceInstallLink, "link", "l", "", "Also link installed items to this profile ('.' for the active profile)")
	cmd.Flags().BoolVar(&sourceInstallClone, "clone", false, "Copy items into the hub as local items you can edit (converts an existing shadow)")
}

var sourceInstallCmd = &cobra.Command{
	Use:     "install [source] [items...]",
	Aliases: []string{"i", "add"},
	Short:   "Install items from a source or sync all from registry",
	Long: `Install specific items from a source to the hub.

If the source is not already added, it will be added automatically.
If no items are specified, interactive selection is shown.

An installed item is a shadow: a symlink into the source's clone, so
'ccp source update' updates it in place. To edit an item, install it with
--clone: ccp copies it into the hub as a local item and stops tracking it.

When called without arguments, syncs all sources from ccp.toml:
- Clones missing sources at the commit ccp.toml records
- Recreates missing shadow links

Examples:
  ccp source install                                  # Sync all from ccp.toml
  ccp source install remorses/playwriter              # Auto-add, interactive selection
  ccp source install owner/repo skills/my-skill
  ccp source install owner/repo --all
  ccp source install owner/repo skills/my-skill --link .        # ...and link to the active profile
  ccp source install owner/repo skills/my-skill --link work     # ...and link to 'work'
  ccp source install owner/repo skills/my-skill --clone         # local copy to edit`,
	Args: cobra.MinimumNArgs(0),
	RunE: runSourceInstall,
}

func init() {
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallAll, "all", "a", false, "Install all available items")
	sourceInstallCmd.Flags().BoolVarP(&sourceInstallInteractive, "interactive", "i", false, "Interactive item selection")
	addInstallLinkFlag(sourceInstallCmd)
	sourceCmd.AddCommand(sourceInstallCmd)
}

func runSourceInstall(cmd *cobra.Command, args []string) error {
	// No args = sync all from registry
	if len(args) == 0 {
		if sourceInstallLink != "" {
			return fmt.Errorf("--link needs a package to install; it does not apply to syncing ccp.toml")
		}
		if sourceInstallClone {
			return fmt.Errorf("--clone needs a package and items; it does not apply to syncing ccp.toml")
		}
		return runSourceSync()
	}

	sourceID := args[0]
	items := args[1:]

	// A GitHub/GitLab blob|tree URL points at a specific skill inside a repo,
	// e.g. https://github.com/owner/repo/blob/main/SKILL.md. Split it into the
	// repo (to add as the source) and the in-repo path (the skill to install).
	addIdentifier := sourceID
	var urlRef, skillPath string
	if repoURL, ref, sub := source.ParseGitWebURL(sourceID); sub != "" {
		addIdentifier = repoURL
		urlRef = ref
		skillPath = sub
		sourceID = generateSourceID(repoURL, repoURL)
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	// Resolve --link up front so a bad profile name fails before anything is installed
	linkProfile, err := resolveLinkProfile(paths)
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
		fmt.Printf("Source not found, adding: %s\n", addIdentifier)
		if addErr := addSourceForInstall(addIdentifier, urlRef, paths, registry); addErr != nil {
			return fmt.Errorf("failed to add source: %w", addErr)
		}
		// Re-fetch using the generated ID (URL gets normalized to owner/repo)
		resolvedID := generateSourceID(addIdentifier, addIdentifier)
		src, err = registry.GetSource(resolvedID)
		if err != nil {
			return fmt.Errorf("source not found after add: %s", addIdentifier)
		}
		sourceID = resolvedID
	}

	installer := source.NewInstaller(paths, registry)
	mode := source.Shadow
	if sourceInstallClone {
		mode = source.Clone
	}

	// URL pointed at a specific skill: install exactly that, copying everything
	// at the SKILL.md's level (handles both root-level and skills/<name>/).
	if skillPath != "" {
		relDir := skillPath
		if strings.EqualFold(path.Base(relDir), "SKILL.md") {
			relDir = path.Dir(relDir)
		}
		item, instErr := installer.InstallPath(sourceID, relDir, mode)
		if instErr != nil {
			return instErr
		}
		if err := registry.Save(); err != nil {
			return err
		}
		fmt.Printf("Installed %s from %s%s\n", item, sourceID, cloneNote())
		return linkInstalled(paths, linkProfile, []string{item})
	}

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

	installed, err := installer.Install(sourceID, items, mode)
	// Items placed before a failure are in the hub: record them either way.
	if saveErr := registry.Save(); saveErr != nil {
		return saveErr
	}
	if err != nil {
		if len(installed) > 0 {
			fmt.Printf("Installed %d items before the failure: %s\n", len(installed), strings.Join(installed, ", "))
		}
		return err
	}

	fmt.Printf("Installed %d items from %s%s:\n", len(installed), sourceID, cloneNote())
	for _, item := range installed {
		fmt.Printf("  - %s\n", item)
	}

	return linkInstalled(paths, linkProfile, installed)
}

// cloneNote marks install output for --clone, whose items are local items.
func cloneNote() string {
	if sourceInstallClone {
		return " as local items"
	}
	return ""
}

// resolveLinkProfile returns the profile named by --link ("" when not given),
// resolving "." to the active profile and checking that it exists.
func resolveLinkProfile(paths *config.Paths) (string, error) {
	if sourceInstallLink == "" {
		return "", nil
	}
	mgr := profile.NewManager(paths)
	if sourceInstallLink == activeProfileMarker {
		active, err := mgr.GetActive()
		if err != nil {
			return "", fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return "", fmt.Errorf("no active profile: use --link <profile>")
		}
		return active.Name, nil
	}
	if !mgr.Exists(sourceInstallLink) {
		return "", fmt.Errorf("--link: profile not found: %s", sourceInstallLink)
	}
	return sourceInstallLink, nil
}

// linkInstalled links freshly installed items to profileName, or prints how to
// do it when --link was not given.
func linkInstalled(paths *config.Paths, profileName string, items []string) error {
	if profileName == "" {
		fmt.Println()
		fmt.Println("Link to profile with:")
		for _, item := range items {
			fmt.Printf("  ccp link <profile> %s\n", item)
		}
		fmt.Println("(or pass --link <profile> to install and link in one step; '.' is the active profile)")
		return nil
	}

	mgr := profile.NewManager(paths)
	for _, item := range items {
		itemType, itemName, ok := strings.Cut(item, "/")
		if !ok {
			return fmt.Errorf("cannot link %s: expected type/name", item)
		}
		if err := mgr.LinkHubItem(profileName, config.HubItemType(itemType), itemName); err != nil {
			return fmt.Errorf("failed to link %s to profile %s: %w", item, profileName, err)
		}
		fmt.Printf("Linked %s to profile %s\n", item, profileName)
	}
	return nil
}

// addSourceForInstall adds a source when it's not found during install.
// refOverride, when non-empty, forces the git ref (e.g. the branch parsed from
// a blob URL); otherwise the ref comes from the registry lookup.
func addSourceForInstall(identifier, refOverride string, paths *config.Paths, registry *source.Registry) error {
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
		ref = refOverride
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
		if refOverride != "" {
			ref = refOverride
		}
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
// 2. Check out the commit ccp.toml records
// 3. Recreate missing items from the Installed list
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

		// ccp.toml's commit is a lock: move the clone to it, so every machine
		// that syncs the same ccp.toml gets the same item content.
		if err := checkoutLockedCommit(ctx, installer, entry, sourceDir); err != nil {
			fmt.Printf("  ⚠ %v\n", err)
		}

		// Reinstall items if any are missing from hub
		if len(entry.Source.Installed) > 0 {
			var missing []string
			for _, item := range entry.Source.Installed {
				parts := strings.SplitN(item, "/", 2)
				if len(parts) != 2 {
					continue
				}
				// Stat follows a shadow link: a link whose target is gone is missing.
				itemPath := paths.HubDir + "/" + item
				if _, err := os.Stat(itemPath); os.IsNotExist(err) {
					missing = append(missing, item)
				}
			}

			if len(missing) > 0 {
				fmt.Printf("  Installing %d missing items...\n", len(missing))

				// One at a time, so one bad item does not stop the rest. A failed
				// item stays in ccp.toml: it is wanted, and a later sync retries it.
				for _, item := range missing {
					if _, err := installer.Install(entry.ID, []string{item}, source.Shadow); err != nil {
						fmt.Printf("    ⚠ %s: %v\n", item, err)
						continue
					}
					fmt.Printf("    + %s\n", item)
					totalInstalled++
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

// checkoutLockedCommit moves a git source clone to the commit ccp.toml records.
// A clone with local edits is left alone: the checkout would discard them. A
// failure (offline, rate-limited, a commit force-pushed away) only warns: the
// lock in ccp.toml is never rewritten here, so a transient error cannot lose it.
func checkoutLockedCommit(ctx context.Context, installer *source.Installer, entry source.SourceEntry, sourceDir string) error {
	git, ok := source.GetProvider(entry.Source.Provider).(*source.GitProvider)
	if !ok || entry.Source.Commit == "" {
		return nil
	}
	if !source.ValidCommit(entry.Source.Commit) {
		return fmt.Errorf("ccp.toml commit %q is not a full commit id: ignored", entry.Source.Commit)
	}
	head := git.GetCommit(sourceDir)
	if head == "" || head == entry.Source.Commit {
		return nil
	}
	edits, err := installer.LocalEdits(entry.ID)
	if err != nil {
		return err
	}
	if len(edits) > 0 {
		return fmt.Errorf("not moved to locked commit %s: local edits in %s", shortenSHA(entry.Source.Commit), strings.Join(edits, ", "))
	}
	if err := git.CheckoutCommit(ctx, sourceDir, entry.Source.Commit); err != nil {
		return fmt.Errorf("could not move to locked commit %s, still at %s: %v", shortenSHA(entry.Source.Commit), shortenSHA(head), err)
	}
	fmt.Printf("  ✓ At locked commit %s\n", shortenSHA(entry.Source.Commit))
	return nil
}
