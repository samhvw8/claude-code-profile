package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var hubLinkCmd = &cobra.Command{
	Use:     "link [profile]",
	Aliases: []string{"ln"},
	Short:   "Add hub items to a profile",
	Long: `Add hub items to a profile by selecting from non-linked components.

With no arguments: Interactive selection for current active profile
With profile: Interactive selection for specified profile

Examples:
  ccp hub link              # Interactive for active profile
  ccp hub link quickfix     # Interactive for 'quickfix' profile`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeHubLinkArgs,
	RunE:              runHubLink,
}

func init() {
	hubCmd.AddCommand(hubLinkCmd)
}

func completeHubLinkArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	mgr := profile.NewManager(paths)
	profiles, err := mgr.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var names []string
	for _, p := range profiles {
		if strings.HasPrefix(p.Name, toComplete) {
			names = append(names, p.Name)
		}
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

func runHubLink(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Determine profile
	var profileName string

	switch len(args) {
	case 0:
		// No args: use active profile
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile: specify a profile name or run 'ccp use <profile>'")
		}
		profileName = active.Name
	case 1:
		profileName = args[0]
	}

	// Verify profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	return runInteractiveAddLink(paths, p)
}

func runInteractiveAddLink(paths *config.Paths, p *profile.Profile) error {
	// Scan hub for available items
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Build tabs for the tabbed picker (only non-linked items)
	var tabs []picker.Tab

	for _, itemType := range config.AllHubItemTypes() {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}

		// Get currently linked items
		currentLinked := make(map[string]bool)
		for _, name := range p.Manifest.GetHubItems(itemType) {
			currentLinked[name] = true
		}

		// Filter to only non-linked items
		var pickerItems []picker.Item
		for _, item := range items {
			if !currentLinked[item.Name] {
				pickerItems = append(pickerItems, picker.Item{
					ID:       item.Name,
					Label:    item.Name,
					Selected: false,
				})
			}
		}

		if len(pickerItems) > 0 {
			tabs = append(tabs, picker.Tab{
				Name:  string(itemType),
				Items: pickerItems,
			})
		}
	}

	if len(tabs) == 0 {
		fmt.Println("All hub items are already linked to this profile")
		return nil
	}

	fmt.Printf("Add hub items to profile '%s'\n\n", p.Name)

	// Run tabbed picker
	selections, err := picker.RunTabbed(tabs)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}
	if selections == nil {
		fmt.Println("Cancelled")
		return nil
	}

	// Count new items to add
	totalAdded := 0
	for _, items := range selections {
		totalAdded += len(items)
	}

	if totalAdded == 0 {
		fmt.Println("No items selected")
		return nil
	}

	// Add selected items to manifest
	for _, itemType := range config.AllHubItemTypes() {
		if items, ok := selections[string(itemType)]; ok {
			for _, itemName := range items {
				p.Manifest.AddHubItem(itemType, itemName)
			}
		}
	}

	// Save manifest
	manifestPath := profile.ManifestPath(p.Path)
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	// Sync symlinks
	fmt.Println("\nSyncing profile...")
	if err := syncAddedLinks(paths, p, selections); err != nil {
		return fmt.Errorf("failed to sync profile: %w", err)
	}

	fmt.Printf("Added %d item(s) to profile '%s'\n", totalAdded, p.Name)
	return nil
}

func syncAddedLinks(paths *config.Paths, p *profile.Profile, selections map[string][]string) error {
	symMgr := symlink.New()

	// Create symlinks for newly added items
	for _, itemType := range config.AllHubItemTypes() {
		itemTypeStr := string(itemType)
		items, ok := selections[itemTypeStr]
		if !ok || len(items) == 0 {
			continue
		}

		itemDir := filepath.Join(p.Path, itemTypeStr)

		// Ensure directory exists
		if err := os.MkdirAll(itemDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", itemType, err)
		}

		for _, itemName := range items {
			hubItemPath := paths.HubItemPath(itemType, itemName)
			profileItemPath := filepath.Join(itemDir, itemName)

			// Check if hub item exists
			if _, err := os.Stat(hubItemPath); err != nil {
				fmt.Printf("Warning: hub item not found: %s/%s\n", itemType, itemName)
				continue
			}

			// Create symlink
			if err := symMgr.Create(profileItemPath, hubItemPath); err != nil {
				fmt.Printf("Warning: failed to create symlink for %s/%s: %v\n", itemType, itemName, err)
			}
		}
	}

	// Regenerate settings.json if hooks or setting fragments were added
	hasHooks := false
	hasFragments := false
	if items, ok := selections["hooks"]; ok && len(items) > 0 {
		hasHooks = true
	}
	if items, ok := selections["setting-fragments"]; ok && len(items) > 0 {
		hasFragments = true
	}

	if hasHooks || hasFragments {
		if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
			return fmt.Errorf("failed to regenerate settings.json: %w", err)
		}
	}

	return nil
}
