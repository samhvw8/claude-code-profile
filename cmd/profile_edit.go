package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
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

	// Check if any flags were provided
	hasFlags := len(editAddSkills) > 0 || len(editAddHooks) > 0 || len(editAddRules) > 0 ||
		len(editAddCommands) > 0 ||
		len(editRemoveSkills) > 0 || len(editRemoveHooks) > 0 || len(editRemoveRules) > 0 ||
		len(editRemoveCommands) > 0

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
	manifestPath := filepath.Join(p.Path, "profile.yaml")
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
	manifestPath := filepath.Join(p.Path, "profile.yaml")
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	return nil
}

func syncProfileEdit(paths *config.Paths, p *profile.Profile) error {
	symMgr := symlink.New()

	// Sync hub item symlinks
	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(p.Path, string(itemType))

		// Ensure directory exists
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", itemType, err)
		}

		// Get items from manifest
		manifestItems := make(map[string]bool)
		for _, name := range p.Manifest.GetHubItems(itemType) {
			manifestItems[name] = true
		}

		// Remove symlinks not in manifest
		entries, err := os.ReadDir(itemDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read %s directory: %w", itemType, err)
		}

		for _, entry := range entries {
			if !manifestItems[entry.Name()] {
				linkPath := filepath.Join(itemDir, entry.Name())
				isLink, _ := symMgr.IsSymlink(linkPath)
				if isLink {
					os.Remove(linkPath)
				}
			}
		}

		// Create missing symlinks
		for _, itemName := range p.Manifest.GetHubItems(itemType) {
			hubItemPath := paths.HubItemPath(itemType, itemName)
			profileItemPath := filepath.Join(itemDir, itemName)

			// Check if hub item exists
			if _, err := os.Stat(hubItemPath); err != nil {
				continue
			}

			// Check if symlink already exists and is correct
			isLink, _ := symMgr.IsSymlink(profileItemPath)
			if isLink {
				target, err := symMgr.ReadLink(profileItemPath)
				if err == nil && target == hubItemPath {
					continue
				}
				os.Remove(profileItemPath)
			}

			if err := symMgr.Create(profileItemPath, hubItemPath); err != nil {
				fmt.Printf("Warning: failed to create symlink for %s/%s: %v\n", itemType, itemName, err)
			}
		}
	}

	// Regenerate settings.json for hooks and setting fragments
	if len(p.Manifest.Hub.Hooks) > 0 || len(p.Manifest.Hub.SettingFragments) > 0 {
		if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
			return fmt.Errorf("failed to regenerate settings.json: %w", err)
		}
	}

	return nil
}
