package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var useShowFlag bool

var useCmd = &cobra.Command{
	Use:   "use [profile]",
	Short: "Set or show the default active profile",
	Long: `Set which profile ~/.claude points to, or show the current default.

Without arguments, opens an interactive picker to select a profile.
With --show flag, displays the current active profile.

Examples:
  ccp use                  # Open interactive picker
  ccp use quickfix         # Set default to quickfix profile
  ccp use --show           # Show current default profile`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runUse,
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

	// Show mode (explicit --show flag)
	if useShowFlag {
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

	// Interactive mode (no args)
	if len(args) == 0 {
		return runUseInteractive(mgr, paths)
	}

	// Direct mode (profile name provided)
	return switchToProfile(mgr, paths, args[0])
}

func runUseInteractive(mgr *profile.Manager, paths *config.Paths) error {
	profiles, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found. Create one with 'ccp profile create <name>'")
		return nil
	}

	// Get active profile to mark it
	active, _ := mgr.GetActive()
	activeName := ""
	if active != nil {
		activeName = active.Name
	}

	// Build picker items
	items := make([]picker.Item, len(profiles))
	for i, p := range profiles {
		label := p.Name
		if p.Manifest.Description != "" {
			desc := p.Manifest.Description
			if len(desc) > 30 {
				desc = desc[:27] + "..."
			}
			label = fmt.Sprintf("%s - %s", p.Name, desc)
		}
		if p.Name == activeName {
			label = label + " (active)"
		}
		items[i] = picker.Item{
			ID:       p.Name,
			Label:    label,
			Selected: p.Name == activeName,
		}
	}

	selected, err := picker.RunSingle("Select profile", items)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	if selected == "" {
		// User cancelled
		return nil
	}

	return switchToProfile(mgr, paths, selected)
}

func switchToProfile(mgr *profile.Manager, paths *config.Paths, profileName string) error {
	// Check profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	// Check for drift before switching
	detector := profile.NewDetector(paths)
	report, err := detector.Detect(p)
	if err != nil {
		// Warn but don't block switch
		fmt.Printf("Warning: could not check profile health: %v\n", err)
	} else if report.HasDrift() {
		fmt.Printf("Warning: profile '%s' has configuration drift (%d issues)\n", profileName, len(report.Issues))
		fmt.Printf("  Run 'ccp profile fix %s' to reconcile\n\n", profileName)
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
