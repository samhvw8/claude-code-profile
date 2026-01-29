package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var useShowFlag bool

var useCmd = &cobra.Command{
	Use:   "use [profile]",
	Short: "Set or show the default active profile",
	Long: `Set which profile ~/.claude points to, or show the current default.

Examples:
  ccp use quickfix     # Set default to quickfix profile
  ccp use --show       # Show current default profile`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUse,
}

func init() {
	useCmd.Flags().BoolVar(&useShowFlag, "show", false, "Show current default profile")
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Show mode
	if useShowFlag || len(args) == 0 {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}

		if active == nil {
			fmt.Println("No active profile (symlink not set)")
			return nil
		}

		fmt.Printf("Active profile: %s\n", active.Name)
		if active.Manifest.Description != "" {
			fmt.Printf("Description: %s\n", active.Manifest.Description)
		}
		return nil
	}

	// Set mode
	profileName := args[0]

	// Check profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	// Set active
	if err := mgr.SetActive(profileName); err != nil {
		return fmt.Errorf("failed to set active profile: %w", err)
	}

	// Regenerate settings.json with updated hook paths
	if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
		// Warn but don't fail - the profile switch succeeded
		fmt.Printf("Warning: failed to regenerate settings.json: %v\n", err)
	}

	fmt.Printf("Switched to profile: %s\n", profileName)
	return nil
}
