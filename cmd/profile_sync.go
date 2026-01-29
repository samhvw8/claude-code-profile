package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var profileSyncCmd = &cobra.Command{
	Use:   "sync [name]",
	Short: "Regenerate profile symlinks and settings.json",
	Long: `Sync a profile by regenerating hub item symlinks and settings.json based on profile.yaml.

This command is useful when:
- Hub items have been added/removed and you want to update the profile
- Settings.json hooks are out of sync with the hub
- Symlinks are broken or missing

If no profile name is given, syncs the active profile.

Examples:
  ccp profile sync           # Sync active profile
  ccp profile sync default   # Sync the 'default' profile
  ccp profile sync --all     # Sync all profiles`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProfileSync,
}

var syncAll bool

func init() {
	profileSyncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all profiles")
	profileCmd.AddCommand(profileSyncCmd)
}

func runProfileSync(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	if syncAll {
		// Sync all profiles
		profiles, err := mgr.List()
		if err != nil {
			return fmt.Errorf("failed to list profiles: %w", err)
		}

		for _, p := range profiles {
			fmt.Printf("Syncing profile: %s\n", p.Name)
			if err := syncProfile(paths, p); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			} else {
				fmt.Println("  Done")
			}
		}
		return nil
	}

	// Get target profile
	var profileName string
	if len(args) > 0 {
		profileName = args[0]
	} else {
		// Use active profile
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no profile name specified")
		}
		profileName = active.Name
	}

	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	fmt.Printf("Syncing profile: %s\n", p.Name)
	if err := syncProfile(paths, p); err != nil {
		return err
	}
	fmt.Println("Done")

	return nil
}

func syncProfile(paths *config.Paths, p *profile.Profile) error {
	symMgr := symlink.New()

	// Sync hub item symlinks
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(p.Path, string(itemType))

		// Ensure directory exists
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", itemType, err)
		}

		// Get items from manifest
		manifestItems := make(map[string]bool)
		for _, name := range p.Manifest.GetHubItems(itemType) {
			manifestItems[name] = true
		}

		// Remove symlinks not in manifest
		entries, err := os.ReadDir(itemDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read %s directory: %w", itemType, err)
		}

		for _, entry := range entries {
			if !manifestItems[entry.Name()] {
				linkPath := filepath.Join(itemDir, entry.Name())
				isLink, _ := symMgr.IsSymlink(linkPath)
				if isLink {
					fmt.Printf("  Removing unlinked %s: %s\n", itemType, entry.Name())
					os.Remove(linkPath)
				}
			}
		}

		// Create missing symlinks
		for _, itemName := range p.Manifest.GetHubItems(itemType) {
			hubItemPath := paths.HubItemPath(itemType, itemName)
			profileItemPath := filepath.Join(itemDir, itemName)

			// Check if hub item exists
			if _, err := os.Stat(hubItemPath); err != nil {
				fmt.Printf("  Warning: hub item not found: %s/%s\n", itemType, itemName)
				continue
			}

			// Check if symlink already exists and is correct
			isLink, _ := symMgr.IsSymlink(profileItemPath)
			if isLink {
				target, err := symMgr.ReadLink(profileItemPath)
				if err == nil && target == hubItemPath {
					continue // Already correct
				}
				// Wrong target, remove and recreate
				os.Remove(profileItemPath)
			}

			fmt.Printf("  Linking %s: %s\n", itemType, itemName)
			if err := symMgr.Create(profileItemPath, hubItemPath); err != nil {
				fmt.Printf("  Warning: failed to create symlink for %s/%s: %v\n", itemType, itemName, err)
			}
		}
	}

	// Regenerate settings.json
	if len(p.Manifest.Hub.Hooks) > 0 || len(p.Manifest.Hub.SettingFragments) > 0 {
		fmt.Println("  Regenerating settings.json...")
		if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
			return fmt.Errorf("failed to regenerate settings.json: %w", err)
		}
		if len(p.Manifest.Hub.Hooks) > 0 {
			fmt.Printf("  Configured %d hooks\n", len(p.Manifest.Hub.Hooks))
		}
		if len(p.Manifest.Hub.SettingFragments) > 0 {
			fmt.Printf("  Merged %d setting fragments\n", len(p.Manifest.Hub.SettingFragments))
		}
	} else if len(p.Manifest.Hooks) > 0 {
		// Legacy: Sync hooks from old-style manifest.Hooks
		fmt.Println("  Syncing legacy hooks...")
		settingsMgr := profile.NewSettingsManager(paths)
		if err := settingsMgr.SyncHooksFromManifest(p.Path, p.Manifest); err != nil {
			return fmt.Errorf("failed to sync settings: %w", err)
		}
		fmt.Printf("  Configured %d hooks\n", len(p.Manifest.Hooks))
	}

	return nil
}
