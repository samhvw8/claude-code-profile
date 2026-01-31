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
	createSkills           []string
	createHooks            []string
	createRules            []string
	createCommands         []string
	createSettingFragments []string
	createFrom             string
	createInteractive      bool
	createEmpty            bool
	createDescription      string
)

var profileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new profile",
	Long: `Create a new profile by selecting items from the hub.

Examples:
  ccp profile create quickfix --skills=debugging-core,git-basics
  ccp profile create dev --interactive
  ccp profile create minimal --from=default
  ccp profile create empty-profile --empty`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileCreate,
}

func init() {
	profileCreateCmd.Flags().StringSliceVar(&createSkills, "skills", nil, "Skills to include")
	profileCreateCmd.Flags().StringSliceVar(&createHooks, "hooks", nil, "Hooks to include")
	profileCreateCmd.Flags().StringSliceVar(&createRules, "rules", nil, "Rules to include")
	profileCreateCmd.Flags().StringSliceVar(&createCommands, "commands", nil, "Commands to include")
	profileCreateCmd.Flags().StringSliceVar(&createSettingFragments, "setting-fragments", nil, "Setting fragments to include")
	profileCreateCmd.Flags().StringVar(&createFrom, "from", "", "Copy configuration from existing profile")
	profileCreateCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive picker mode")
	profileCreateCmd.Flags().BoolVarP(&createEmpty, "empty", "e", false, "Create empty profile without hub items")
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
	if len(createSettingFragments) > 0 {
		manifest.Hub.SettingFragments = createSettingFragments
	}

	// Interactive mode
	hasAnyFlags := len(createSkills) > 0 || len(createHooks) > 0 || len(createRules) > 0 ||
		len(createCommands) > 0 || len(createSettingFragments) > 0 || createFrom != "" || createEmpty

	if createInteractive || !hasAnyFlags {
		// Scan hub for available items
		scanner := hub.NewScanner()
		h, err := scanner.Scan(paths.HubDir)
		if err != nil {
			return fmt.Errorf("failed to scan hub: %w", err)
		}

		// Build tabs for the tabbed picker
		var tabs []picker.Tab

		// Add hub item tabs
		for _, itemType := range config.AllHubItemTypes() {
			items := h.GetItems(itemType)
			if len(items) == 0 {
				continue
			}

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

			tabs = append(tabs, picker.Tab{
				Name:  string(itemType),
				Items: pickerItems,
			})
		}

		// Add data sharing tab
		var dataItems []picker.Item
		defaultConfig := config.DefaultDataConfig()
		for _, dataType := range config.AllDataItemTypes() {
			isShared := defaultConfig[dataType] == config.ShareModeShared
			label := string(dataType)
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
		tabs = append(tabs, picker.Tab{
			Name:  "data-sharing",
			Items: dataItems,
		})

		// Run tabbed picker
		selections, err := picker.RunTabbed(tabs)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}
		if selections == nil {
			fmt.Println("Cancelled")
			return nil
		}

		// Apply hub item selections
		for _, itemType := range config.AllHubItemTypes() {
			if items, ok := selections[string(itemType)]; ok {
				manifest.SetHubItems(itemType, items)
			}
		}

		// Apply data sharing selections
		sharedSet := make(map[string]bool)
		if dataItems, ok := selections["data-sharing"]; ok {
			for _, dt := range dataItems {
				sharedSet[dt] = true
			}
		}
		for _, dataType := range config.AllDataItemTypes() {
			if sharedSet[string(dataType)] {
				manifest.SetDataShareMode(dataType, config.ShareModeShared)
			} else {
				manifest.SetDataShareMode(dataType, config.ShareModeIsolated)
			}
		}

		// Hooks are linked from hub - types are already defined in hook.yaml
		selectedHooks := manifest.GetHubItems(config.HubHooks)
		if len(selectedHooks) > 0 {
			fmt.Printf("\nLinked %d hooks from hub (types defined in hook.yaml)\n", len(selectedHooks))
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
