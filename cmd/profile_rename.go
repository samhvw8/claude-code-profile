package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/profile"
	"github.com/samhvw8/claude-code-profile/internal/symlink"
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

	oldPath := paths.ProfileDir(oldName)
	newPath := paths.ProfileDir(newName)

	// A CLAUDE_CONFIG_DIR naming the profile folder itself would point at a
	// folder that no longer exists; ccp cannot rewrite that variable.
	if os.Getenv("CLAUDE_CONFIG_DIR") != "" && profile.SamePath(paths.ClaudeDir, oldPath) {
		return fmt.Errorf("profile '%s' is in use through CLAUDE_CONFIG_DIR in this shell; leave that session (and update the variable wherever it is set) before renaming", oldName)
	}

	// The global symlink follows the rename. It is always ~/.claude, whatever
	// CLAUDE_CONFIG_DIR says: that may be a real profile or account folder.
	globalPaths := *paths
	globalPaths.ClaudeDir = paths.GlobalClaudeDir
	isActive := false
	if globalPaths.ClaudeDirIsSymlink() {
		active, err := profile.NewManager(&globalPaths).GetActive()
		if err != nil {
			return fmt.Errorf("failed to check active profile: %w", err)
		}
		isActive = active != nil && active.Name == oldName
	}

	// Rename the directory
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename profile directory: %w", err)
	}
	if err := profile.MoveSettingsSnapshot(paths, oldPath, newPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not move the settings snapshot: %v\n", err)
	}

	// Update manifest with new name
	manifestPath := profile.ManifestPath(newPath)
	manifest, err := profile.LoadManifest(manifestPath)
	if err != nil {
		// Non-fatal - profile still renamed
		fmt.Fprintf(os.Stderr, "Warning: could not update manifest name: %v\n", err)
	}

	if manifest != nil {
		manifest.Name = newName
		if err := manifest.Save(manifestPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save updated manifest: %v\n", err)
		}
	}

	// Update symlink if this was the active profile
	if isActive {
		if err := symMgr.Swap(paths.GlobalClaudeDir, newPath); err != nil {
			return fmt.Errorf("profile renamed but failed to update active symlink: %w", err)
		}
	}

	// Accounts linked to the old name move with it; their links into the old
	// folder are broken now and are replaced.
	warnings, err := profile.RelinkAccounts(paths, oldName, newName)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	if err != nil {
		return fmt.Errorf("profile renamed but accounts were not all moved: %w\nRun 'ccp env %s --account <name> --relink' for each", err, newName)
	}

	fmt.Printf("Renamed profile '%s' to '%s'\n", oldName, newName)
	if isActive {
		fmt.Println("Updated active profile symlink")
	}

	return nil
}
