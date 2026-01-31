package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var hubRenameCmd = &cobra.Command{
	Use:   "rename <type>/<old-name> <new-name>",
	Short: "Rename a hub item and update all profile symlinks",
	Long: `Rename a hub item and automatically update all profile symlinks.

Examples:
  ccp hub rename skills/old-skill.md new-skill.md
  ccp hub rename agents/old-agent new-agent`,
	Args: cobra.ExactArgs(2),
	RunE: runHubRename,
}

func init() {
	hubCmd.AddCommand(hubRenameCmd)
}

func runHubRename(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Parse type/old-name
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: use <type>/<old-name>")
	}

	itemType := config.HubItemType(parts[0])
	oldName := parts[1]
	newName := args[1]

	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s", parts[0])
	}

	oldPath := paths.HubItemPath(itemType, oldName)
	newPath := paths.HubItemPath(itemType, newName)

	// Check old exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return fmt.Errorf("item not found: %s/%s", itemType, oldName)
	}

	// Check new doesn't exist
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("item already exists: %s/%s", itemType, newName)
	}

	// Find profiles using this item
	profilesToUpdate, err := findProfilesUsingItemByName(paths, itemType, oldName)
	if err != nil {
		return fmt.Errorf("failed to find profiles: %w", err)
	}

	// Rename in hub
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}

	// Update profile symlinks and manifests
	symMgr := symlink.New()
	for _, profileName := range profilesToUpdate {
		profileDir := paths.ProfileDir(profileName)
		manifestPath := profile.ManifestPath(profileDir)

		// Update symlink
		oldLink := filepath.Join(profileDir, string(itemType), oldName)
		newLink := filepath.Join(profileDir, string(itemType), newName)

		os.Remove(oldLink)
		if err := symMgr.Create(newLink, newPath); err != nil {
			fmt.Printf("Warning: failed to update symlink in profile %s: %v\n", profileName, err)
			continue
		}

		// Update manifest
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		items := manifest.GetHubItems(itemType)
		for i, item := range items {
			if item == oldName {
				items[i] = newName
			}
		}
		manifest.SetHubItems(itemType, items)

		if err := manifest.Save(manifestPath); err != nil {
			fmt.Printf("Warning: failed to update manifest in profile %s: %v\n", profileName, err)
		}
	}

	fmt.Printf("Renamed %s/%s -> %s/%s\n", itemType, oldName, itemType, newName)
	if len(profilesToUpdate) > 0 {
		fmt.Printf("Updated profiles: %s\n", strings.Join(profilesToUpdate, ", "))
	}

	return nil
}
