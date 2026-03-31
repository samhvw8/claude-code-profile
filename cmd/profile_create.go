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
	createFrom        string
	createInteractive bool
	createEmpty       bool
	createDescription string
	createTemplate    string
)

var profileCreateCmd = &cobra.Command{
	Use:     "create <name>",
	Aliases: []string{"c", "new"},
	Short:   "Create a new profile",
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
	profileCreateCmd.Flags().StringVar(&createFrom, "from", "", "Copy configuration from existing profile")
	profileCreateCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive picker mode")
	profileCreateCmd.Flags().BoolVarP(&createEmpty, "empty", "e", false, "Create empty profile without hub items")
	profileCreateCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Profile description")
	profileCreateCmd.Flags().StringVar(&createTemplate, "template", "", "Settings template to use")
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

	// Validate and assign settings template
	if createTemplate != "" {
		tmplMgr := hub.NewTemplateManager(paths.HubDir)
		if !tmplMgr.Exists(createTemplate) {
			return fmt.Errorf("settings template not found: %s", createTemplate)
		}
		manifest.SettingsTemplate = createTemplate
	}

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

		// Copy template if not overridden by flags
		if createTemplate == "" && sourceProfile.Manifest.SettingsTemplate != "" {
			manifest.SettingsTemplate = sourceProfile.Manifest.SettingsTemplate
		}

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

	// Interactive mode
	hasAnyFlags := len(createSkills) > 0 || len(createHooks) > 0 || len(createRules) > 0 ||
		len(createCommands) > 0 || createFrom != "" || createEmpty || createTemplate != ""

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

	fmt.Println()
	fmt.Println("To activate this profile:")
	fmt.Printf("  ccp use %s\n", profileName)
	fmt.Println("Or set CLAUDE_CONFIG_DIR in your project's .envrc or .mise.toml")

	return nil
}
