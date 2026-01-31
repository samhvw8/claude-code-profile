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

var linkCmd = &cobra.Command{
	Use:   "link [profile] [path]",
	Short: "Add hub items to a profile",
	Long: `Add hub items to a profile by creating symlinks.

With no arguments: Interactive selection for current active profile
With profile only: Interactive selection for specified profile
With profile and path: Link specific item (path format: type/name)

Examples:
  ccp link                                    # Interactive for active profile
  ccp link quickfix                           # Interactive for 'quickfix' profile
  ccp link quickfix skills/debugging-core     # Link specific item`,
	Args:              cobra.RangeArgs(0, 2),
	ValidArgsFunction: completeLinkArgs,
	RunE:              runLink,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Determine profile and mode
	var profileName string
	var itemPath string

	switch len(args) {
	case 0:
		// No args: use active profile, interactive mode
		active, err := mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if active == nil {
			return fmt.Errorf("no active profile: specify a profile name or run 'ccp use <profile>'")
		}
		profileName = active.Name
	case 1:
		// One arg: profile name, interactive mode
		profileName = args[0]
	case 2:
		// Two args: profile and path, direct mode
		profileName = args[0]
		itemPath = args[1]
	}

	// Verify profile exists
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	// Interactive mode
	if itemPath == "" {
		return runInteractiveLink(paths, p)
	}

	// Direct mode: parse and link specific item
	parts := strings.SplitN(itemPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid path format: %s (expected type/name)", itemPath)
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	// Validate item type
	valid := false
	for _, t := range config.AllHubItemTypes() {
		if t == itemType {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid item type: %s (valid: skills, agents, hooks, rules, commands, setting-fragments)", parts[0])
	}

	// Verify hub item exists
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	if !h.HasItem(itemType, itemName) {
		return fmt.Errorf("hub item not found: %s/%s", itemType, itemName)
	}

	// Link to profile
	if err := mgr.LinkHubItem(profileName, itemType, itemName); err != nil {
		return fmt.Errorf("failed to link: %w", err)
	}

	fmt.Printf("Linked %s/%s to profile %s\n", itemType, itemName, profileName)
	return nil
}

func runInteractiveLink(paths *config.Paths, p *profile.Profile) error {
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
		fmt.Println("No hub items available to link")
		return nil
	}

	fmt.Printf("Editing hub items for profile '%s'\n\n", p.Name)

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

	// Sync symlinks
	fmt.Println("\nSyncing profile...")
	if err := syncLinkChanges(paths, p); err != nil {
		return fmt.Errorf("failed to sync profile: %w", err)
	}

	fmt.Printf("Profile '%s' updated successfully\n", p.Name)
	return nil
}

func syncLinkChanges(paths *config.Paths, p *profile.Profile) error {
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
