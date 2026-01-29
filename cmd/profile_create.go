package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	createSkills      []string
	createHooks       []string
	createRules       []string
	createCommands    []string
	createMdFragments []string
	createFrom        string
	createInteractive bool
	createDescription string
)

var profileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new profile",
	Long: `Create a new profile by selecting items from the hub.

Examples:
  ccp profile create quickfix --skills=debugging-core,git-basics
  ccp profile create dev --interactive
  ccp profile create minimal --from=default`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileCreate,
}

func init() {
	profileCreateCmd.Flags().StringSliceVar(&createSkills, "skills", nil, "Skills to include")
	profileCreateCmd.Flags().StringSliceVar(&createHooks, "hooks", nil, "Hooks to include")
	profileCreateCmd.Flags().StringSliceVar(&createRules, "rules", nil, "Rules to include")
	profileCreateCmd.Flags().StringSliceVar(&createCommands, "commands", nil, "Commands to include")
	profileCreateCmd.Flags().StringSliceVar(&createMdFragments, "md-fragments", nil, "MD fragments to include")
	profileCreateCmd.Flags().StringVar(&createFrom, "from", "", "Copy configuration from existing profile")
	profileCreateCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive picker mode")
	profileCreateCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Profile description")
	profileCmd.AddCommand(profileCreateCmd)
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Check if profile already exists
	if mgr.Exists(profileName) {
		return fmt.Errorf("profile already exists: %s", profileName)
	}

	// Create manifest
	manifest := profile.NewManifest(profileName, createDescription)

	// If --from is specified, copy from existing profile
	if createFrom != "" {
		sourceProfile, err := mgr.Get(createFrom)
		if err != nil {
			return fmt.Errorf("failed to get source profile: %w", err)
		}
		if sourceProfile == nil {
			return fmt.Errorf("source profile not found: %s", createFrom)
		}

		// Copy hub links
		for _, itemType := range config.AllHubItemTypes() {
			manifest.SetHubItems(itemType, sourceProfile.Manifest.GetHubItems(itemType))
		}

		// Copy data config
		manifest.Data = sourceProfile.Manifest.Data

		if createDescription == "" {
			manifest.Description = fmt.Sprintf("Created from %s", createFrom)
		}
	}

	// Apply CLI flags
	if len(createSkills) > 0 {
		manifest.Hub.Skills = createSkills
	}
	if len(createHooks) > 0 {
		manifest.Hub.Hooks = createHooks
	}
	if len(createRules) > 0 {
		manifest.Hub.Rules = createRules
	}
	if len(createCommands) > 0 {
		manifest.Hub.Commands = createCommands
	}
	if len(createMdFragments) > 0 {
		manifest.Hub.MdFragments = createMdFragments
	}

	// Interactive mode
	hasAnyFlags := len(createSkills) > 0 || len(createHooks) > 0 || len(createRules) > 0 ||
		len(createCommands) > 0 || len(createMdFragments) > 0 || createFrom != ""

	if createInteractive || !hasAnyFlags {
		// Scan hub for available items
		scanner := hub.NewScanner()
		h, err := scanner.Scan(paths.HubDir)
		if err != nil {
			return fmt.Errorf("failed to scan hub: %w", err)
		}

		// Run interactive picker for each type with items
		for _, itemType := range config.AllHubItemTypes() {
			items := h.GetItems(itemType)
			if len(items) == 0 {
				continue
			}

			// Build picker items
			var pickerItems []picker.Item
			currentSelected := make(map[string]bool)
			for _, name := range manifest.GetHubItems(itemType) {
				currentSelected[name] = true
			}

			for _, item := range items {
				pickerItems = append(pickerItems, picker.Item{
					ID:       item.Name,
					Label:    item.Name,
					Selected: currentSelected[item.Name],
				})
			}

			title := fmt.Sprintf("Select %s for profile '%s':", itemType, profileName)
			selected, err := picker.Run(title, pickerItems)
			if err != nil {
				return fmt.Errorf("picker error: %w", err)
			}
			if selected == nil {
				// User quit
				fmt.Println("Cancelled")
				return nil
			}

			manifest.SetHubItems(itemType, selected)
		}

		// Ask about data sharing configuration
		fmt.Println()
		fmt.Println("Configure data sharing (shared data is accessible across profiles):")
		fmt.Println()

		var dataItems []picker.Item
		defaultConfig := config.DefaultDataConfig()
		for _, dataType := range config.AllDataItemTypes() {
			isShared := defaultConfig[dataType] == config.ShareModeShared
			label := fmt.Sprintf("%s", dataType)
			if isShared {
				label = fmt.Sprintf("%s (default: shared)", dataType)
			} else {
				label = fmt.Sprintf("%s (default: isolated)", dataType)
			}
			dataItems = append(dataItems, picker.Item{
				ID:       string(dataType),
				Label:    label,
				Selected: isShared,
			})
		}

		sharedDataTypes, err := picker.Run("Select data directories to SHARE across profiles:", dataItems)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}
		if sharedDataTypes == nil {
			fmt.Println("Cancelled")
			return nil
		}

		// Build shared set
		sharedSet := make(map[string]bool)
		for _, dt := range sharedDataTypes {
			sharedSet[dt] = true
		}

		// Apply to manifest
		for _, dataType := range config.AllDataItemTypes() {
			if sharedSet[string(dataType)] {
				manifest.SetDataShareMode(dataType, config.ShareModeShared)
			} else {
				manifest.SetDataShareMode(dataType, config.ShareModeIsolated)
			}
		}
	}

	// Create the profile
	p, err := mgr.Create(profileName, manifest)
	if err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}

	fmt.Printf("Created profile: %s\n", p.Name)
	fmt.Printf("Location: %s\n", p.Path)

	// Print summary
	var summaryParts []string
	for _, itemType := range config.AllHubItemTypes() {
		items := manifest.GetHubItems(itemType)
		if len(items) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d %s", len(items), itemType))
		}
	}
	if len(summaryParts) > 0 {
		fmt.Printf("Linked: %s\n", strings.Join(summaryParts, ", "))
	}

	// Print data sharing summary
	var sharedData, isolatedData []string
	for _, dataType := range config.AllDataItemTypes() {
		if manifest.GetDataShareMode(dataType) == config.ShareModeShared {
			sharedData = append(sharedData, string(dataType))
		} else {
			isolatedData = append(isolatedData, string(dataType))
		}
	}
	if len(sharedData) > 0 {
		fmt.Printf("Shared data: %s\n", strings.Join(sharedData, ", "))
	}
	if len(isolatedData) > 0 {
		fmt.Printf("Isolated data: %s\n", strings.Join(isolatedData, ", "))
	}

	fmt.Println()
	fmt.Println("To activate this profile:")
	fmt.Printf("  ccp use %s\n", profileName)
	fmt.Println("Or set CLAUDE_CONFIG_DIR in your project's .envrc or .mise.toml")

	return nil
}
