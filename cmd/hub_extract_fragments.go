package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/migration"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	extractFromProfile  string
	extractAllFragments bool
	extractReplace      bool
)

var hubExtractFragmentsCmd = &cobra.Command{
	Use:   "extract-fragments",
	Short: "Extract setting fragments from a profile's settings.json",
	Long: `Extract setting fragments from an existing profile's settings.json and add them to the hub.

This is useful when you've initialized ccp before setting-fragments existed and want to
extract your existing settings without re-initializing.

By default, extracts from the active profile. Use --from to specify a different profile.

Examples:
  ccp hub extract-fragments                    # Interactive selection from active profile
  ccp hub extract-fragments --from=default    # Extract from 'default' profile
  ccp hub extract-fragments --all              # Extract all fragments without prompting
  ccp hub extract-fragments --replace          # Replace existing fragments in hub`,
	RunE: runHubExtractFragments,
}

func init() {
	hubExtractFragmentsCmd.Flags().StringVar(&extractFromProfile, "from", "", "Profile to extract from (default: active profile)")
	hubExtractFragmentsCmd.Flags().BoolVar(&extractAllFragments, "all", false, "Extract all fragments without interactive selection")
	hubExtractFragmentsCmd.Flags().BoolVar(&extractReplace, "replace", false, "Replace existing fragments in hub")
	hubCmd.AddCommand(hubExtractFragmentsCmd)
}

func runHubExtractFragments(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Determine which profile to extract from
	var profileName string
	if extractFromProfile != "" {
		profileName = extractFromProfile
	} else {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no --from specified")
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

	// Find settings.json in the profile
	settingsPath := filepath.Join(p.Path, "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		return fmt.Errorf("settings.json not found in profile '%s'", profileName)
	}

	// Extract fragments
	fragments, err := migration.ExtractSettingFragments(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to extract fragments: %w", err)
	}

	if len(fragments) == 0 {
		fmt.Println("No setting fragments found in settings.json")
		return nil
	}

	fmt.Printf("Found %d setting fragments in profile '%s'\n", len(fragments), profileName)

	// Check for existing fragments in hub
	fragmentsDir := filepath.Join(paths.HubDir, string(config.HubSettingFragments))
	existingFragments := make(map[string]bool)
	if entries, err := os.ReadDir(fragmentsDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if filepath.Ext(name) == ".yaml" {
				existingFragments[name[:len(name)-5]] = true
			}
		}
	}

	// Filter or warn about existing fragments
	var fragmentsToSave []migration.SettingFragment
	var skipped []string

	for _, fragment := range fragments {
		if existingFragments[fragment.Name] && !extractReplace {
			skipped = append(skipped, fragment.Name)
		} else {
			fragmentsToSave = append(fragmentsToSave, fragment)
		}
	}

	if len(skipped) > 0 && !extractReplace {
		fmt.Printf("\nSkipping %d existing fragments (use --replace to overwrite):\n", len(skipped))
		for _, name := range skipped {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(fragmentsToSave) == 0 {
		fmt.Println("\nNo new fragments to extract")
		return nil
	}

	// Interactive selection unless --all
	if !extractAllFragments {
		fmt.Println("\nSelect fragments to extract to hub:")

		var pickerItems []picker.Item
		for _, fragment := range fragmentsToSave {
			label := fragment.Name
			if fragment.Description != "" {
				label = fmt.Sprintf("%s - %s", fragment.Name, fragment.Description)
			}
			if existingFragments[fragment.Name] {
				label = label + " (will replace)"
			}
			pickerItems = append(pickerItems, picker.Item{
				ID:       fragment.Name,
				Label:    label,
				Selected: true,
			})
		}

		selected, err := picker.Run("Setting Fragments", pickerItems)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}

		if selected == nil {
			fmt.Println("Cancelled")
			return nil
		}

		// Filter to selected
		selectedMap := make(map[string]bool)
		for _, id := range selected {
			selectedMap[id] = true
		}

		var filtered []migration.SettingFragment
		for _, fragment := range fragmentsToSave {
			if selectedMap[fragment.Name] {
				filtered = append(filtered, fragment)
			}
		}
		fragmentsToSave = filtered
	}

	if len(fragmentsToSave) == 0 {
		fmt.Println("No fragments selected")
		return nil
	}

	// Save fragments to hub
	if err := migration.SaveSettingFragments(paths.HubDir, fragmentsToSave); err != nil {
		return fmt.Errorf("failed to save fragments: %w", err)
	}

	fmt.Printf("\nExtracted %d fragments to hub:\n", len(fragmentsToSave))
	for _, fragment := range fragmentsToSave {
		status := "added"
		if existingFragments[fragment.Name] {
			status = "replaced"
		}
		fmt.Printf("  %s (%s)\n", fragment.Name, status)
	}

	// Suggest adding to profile manifest
	fmt.Println("\nTo use these fragments in a profile, run:")
	fmt.Printf("  ccp profile edit %s --add-setting-fragments=", profileName)
	var names []string
	for _, f := range fragmentsToSave {
		names = append(names, f.Name)
	}
	fmt.Println(names[0])
	if len(names) > 1 {
		fmt.Println("  (or add multiple with comma-separated list)")
	}

	return nil
}
