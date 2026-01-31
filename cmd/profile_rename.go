package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var profileRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a profile",
	Long: `Rename a profile to a new name.

Examples:
  ccp profile rename default my-profile
  ccp profile rename dev development`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileRename,
}

func init() {
	profileCmd.AddCommand(profileRenameCmd)
}

func runProfileRename(cmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	if oldName == newName {
		return fmt.Errorf("old and new names are the same")
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)
	symMgr := symlink.New()

	// Check source profile exists
	if !mgr.Exists(oldName) {
		return fmt.Errorf("profile not found: %s", oldName)
	}

	// Check destination doesn't exist
	if mgr.Exists(newName) {
		return fmt.Errorf("profile already exists: %s", newName)
	}

	// Check if this is the active profile
	active, err := mgr.GetActive()
	if err != nil {
		return fmt.Errorf("failed to check active profile: %w", err)
	}
	isActive := active != nil && active.Name == oldName

	oldPath := paths.ProfileDir(oldName)
	newPath := paths.ProfileDir(newName)

	// Rename the directory
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename profile directory: %w", err)
	}

	// Update manifest with new name
	manifestPath := filepath.Join(newPath, "profile.yaml")
	manifest, err := profile.LoadManifest(manifestPath)
	if err != nil {
		// Try TOML format
		manifestPath = profile.ManifestPath(newPath)
		manifest, err = profile.LoadManifest(manifestPath)
		if err != nil {
			// Non-fatal - profile still renamed
			fmt.Fprintf(os.Stderr, "Warning: could not update manifest name: %v\n", err)
		}
	}

	if manifest != nil {
		manifest.Name = newName
		manifest.Updated = time.Now()
		if err := manifest.Save(manifestPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save updated manifest: %v\n", err)
		}
	}

	// Update symlink if this was the active profile
	if isActive {
		if err := symMgr.Swap(paths.ClaudeDir, newPath); err != nil {
			return fmt.Errorf("profile renamed but failed to update active symlink: %w", err)
		}
	}

	fmt.Printf("Renamed profile '%s' to '%s'\n", oldName, newName)
	if isActive {
		fmt.Println("Updated active profile symlink")
	}

	return nil
}
