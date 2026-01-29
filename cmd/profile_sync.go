package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileSyncCmd = &cobra.Command{
	Use:   "sync [name]",
	Short: "Sync profile settings.json from manifest",
	Long: `Update settings.json to match the profile manifest.

This ensures hooks are properly configured in settings.json with
the correct type (SessionStart, UserPromptSubmit, etc.).

If no profile name is given, syncs the active profile.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProfileSync,
}

func init() {
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

	// Determine which profile to sync
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
			return fmt.Errorf("no active profile")
		}
		profileName = active.Name
	}

	// Get the profile
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	// Sync settings
	settingsMgr := profile.NewSettingsManager(paths)
	if err := settingsMgr.SyncHooksFromManifest(p.Path, p.Manifest); err != nil {
		return fmt.Errorf("failed to sync settings: %w", err)
	}

	fmt.Printf("Synced settings.json for profile: %s\n", profileName)

	// Show what was synced
	if len(p.Manifest.Hooks) > 0 {
		fmt.Println("\nHooks configured:")
		for _, hook := range p.Manifest.Hooks {
			fmt.Printf("  %s -> %s\n", hook.Name, hook.Type)
		}
	} else {
		fmt.Println("\nNo hooks configured in manifest")
	}

	return nil
}
