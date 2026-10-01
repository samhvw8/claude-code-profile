package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhvw8/claude-code-profile/internal/config"
	"github.com/samhvw8/claude-code-profile/internal/hub"
	"github.com/samhvw8/claude-code-profile/internal/picker"
	"github.com/samhvw8/claude-code-profile/internal/profile"
)

var (
	editAddSkills      []string
	editAddHooks       []string
	editAddRules       []string
	editAddCommands    []string
	editRemoveSkills   []string
	editRemoveHooks    []string
	editRemoveRules    []string
	editRemoveCommands []string
	editInteractive    bool
	editTemplate       string
)

var profileEditCmd = &cobra.Command{
	Use:   "edit [name]",
	Short: "Edit profile hub items",
	Long: `Edit a profile by adding or removing hub items.

If no profile name is given, edits the active profile.

Examples:
  ccp profile edit                                    # Interactive edit of active profile
  ccp profile edit default -i                         # Interactive edit
  ccp profile edit default --add-skills=git-basics   # Add a skill
  ccp profile edit default --remove-hooks=session-start  # Remove a hook
  ccp profile edit default --add-skills=a,b --remove-rules=c`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileEdit,
}

func init() {
	// Add flags
	profileEditCmd.Flags().StringSliceVar(&editAddSkills, "add-skills", nil, "Skills to add")
	profileEditCmd.Flags().StringSliceVar(&editAddHooks, "add-hooks", nil, "Hooks to add")
	profileEditCmd.Flags().StringSliceVar(&editAddRules, "add-rules", nil, "Rules to add")
	profileEditCmd.Flags().StringSliceVar(&editAddCommands, "add-commands", nil, "Commands to add")

	// Remove flags
	profileEditCmd.Flags().StringSliceVar(&editRemoveSkills, "remove-skills", nil, "Skills to remove")
	profileEditCmd.Flags().StringSliceVar(&editRemoveHooks, "remove-hooks", nil, "Hooks to remove")
	profileEditCmd.Flags().StringSliceVar(&editRemoveRules, "remove-rules", nil, "Rules to remove")
	profileEditCmd.Flags().StringSliceVar(&editRemoveCommands, "remove-commands", nil, "Commands to remove")

	profileEditCmd.Flags().BoolVarP(&editInteractive, "interactive", "i", false, "Interactive picker mode")
	profileEditCmd.Flags().StringVar(&editTemplate, "template", "", "Set settings template")

	profileCmd.AddCommand(profileEditCmd)
}

func runProfileEdit(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Get target profile
	var profileName string
	if len(args) > 0 {
		profileName = args[0]
	} else {
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile and no profile name specified")
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

	// Handle template changes
	if editTemplate != "" {
		tmplMgr := hub.NewTemplateManager(paths.HubDir)
		if !tmplMgr.Exists(editTemplate) {
			return fmt.Errorf("settings template not found: %s", editTemplate)
		}
		p.Manifest.SettingsTemplate = editTemplate
		fmt.Printf("Set settings template: %s\n", editTemplate)
	}

	// Check if any flags were provided
	hasFlags := len(editAddSkills) > 0 || len(editAddHooks) > 0 || len(editAddRules) > 0 ||
		len(editAddCommands) > 0 ||
		len(editRemoveSkills) > 0 || len(editRemoveHooks) > 0 || len(editRemoveRules) > 0 ||
		len(editRemoveCommands) > 0 || editTemplate != ""

	if editInteractive || !hasFlags {
		// Interactive mode
		if err := runInteractiveEdit(paths, p); err != nil {
			return err
		}
	} else {
		// Flag-based mode
		if err := runFlagEdit(paths, p); err != nil {
			return err
		}
	}

	// Sync the profile
	fmt.Println("\nSyncing profile...")
	if err := syncProfileEdit(paths, p); err != nil {
		return fmt.Errorf("failed to sync profile: %w", err)
	}

	fmt.Printf("Profile '%s' updated successfully\n", profileName)
	followOpencode(paths, profileName)
	return nil
}

func runInteractiveEdit(paths *config.Paths, p *profile.Profile) error {
	// Scan hub for available items
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Build tabs for the tabbed picker
	var tabs []picker.Tab

	for _, itemType := range config.AllHubItemTypes() {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}

		var pickerItems []picker.Item
		currentSelected := make(map[string]bool)
		for _, name := range p.Manifest.GetHubItems(itemType) {
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

	if len(tabs) == 0 {
		fmt.Println("No hub items available to edit")
		return nil
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

	// Apply selections to manifest
	for _, itemType := range config.AllHubItemTypes() {
		if items, ok := selections[string(itemType)]; ok {
			p.Manifest.SetHubItems(itemType, items)
		}
	}

	// Save manifest
	manifestPath := profile.ManifestPath(p.Path)
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	return nil
}

func runFlagEdit(paths *config.Paths, p *profile.Profile) error {
	// Process additions
	addItems := map[config.HubItemType][]string{
		config.HubSkills:   editAddSkills,
		config.HubHooks:    editAddHooks,
		config.HubRules:    editAddRules,
		config.HubCommands: editAddCommands,
	}

	for itemType, items := range addItems {
		for _, name := range items {
			// Verify item exists in hub
			hubItemPath := paths.HubItemPath(itemType, name)
			if _, err := os.Stat(hubItemPath); err != nil {
				fmt.Printf("Warning: hub item not found: %s/%s\n", itemType, name)
				continue
			}
			p.Manifest.AddHubItem(itemType, name)
			fmt.Printf("Added %s: %s\n", itemType, name)
		}
	}

	// Process removals
	removeItems := map[config.HubItemType][]string{
		config.HubSkills:   editRemoveSkills,
		config.HubHooks:    editRemoveHooks,
		config.HubRules:    editRemoveRules,
		config.HubCommands: editRemoveCommands,
	}

	for itemType, items := range removeItems {
		for _, name := range items {
			if p.Manifest.RemoveHubItem(itemType, name) {
				fmt.Printf("Removed %s: %s\n", itemType, name)
			} else {
				fmt.Printf("Warning: %s not found in profile: %s\n", itemType, name)
			}
		}
	}

	// Save manifest
	manifestPath := profile.ManifestPath(p.Path)
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	return nil
}

func syncProfileEdit(paths *config.Paths, p *profile.Profile) error {
	if err := profile.SyncLinks(paths, p, nil); err != nil {
		return err
	}

	// Regenerate settings.json for hooks and templates, keeping uncaptured edits
	uncaptured, err := profile.ApplySettings(paths, p.Path, p.Manifest, false)
	if err != nil {
		return fmt.Errorf("failed to regenerate settings.json: %w", err)
	}
	if len(uncaptured) > 0 {
		noteUncaptured(p.Name, uncaptured)
	}

	return nil
}
