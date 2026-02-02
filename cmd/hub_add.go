package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
	"github.com/samhoang/ccp/internal/symlink"
)

var (
	hubAddFromProfile string
	hubAddReplace     bool
)

var hubAddCmd = &cobra.Command{
	Use:   "add [profile] | [type] <name-or-path>",
	Short: "Add an item to the hub",
	Long: `Add a file/directory to the hub, or promote items from a profile.

With no arguments: Interactive selection for current active profile
With one argument:
  - If it's a profile name: Interactive selection for that profile
  - If it's a type: Error (requires name-or-path)
With two arguments: Add item of <type> from <name-or-path>

Types: skills, agents, hooks, rules, commands, setting-fragments

Examples:
  # Interactive mode (promote local items to hub, symlink back)
  ccp hub add              # Interactive for active profile
  ccp hub add quickfix     # Interactive for 'quickfix' profile

  # Add from filesystem
  ccp hub add skills ./my-skill.md
  ccp hub add agents ./my-agent/
  ccp hub add hooks ~/projects/shared-hooks/pre-commit.sh

  # Promote from profile to hub (legacy, use interactive instead)
  ccp hub add skills my-skill --from-profile=default

  # Replace existing hub item
  ccp hub add skills my-skill --from-profile=default --replace`,
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeHubAddArgs,
	RunE:              runHubAdd,
}

func init() {
	hubAddCmd.Flags().StringVar(&hubAddFromProfile, "from-profile", "", "Promote item from specified profile to hub")
	hubAddCmd.Flags().BoolVar(&hubAddReplace, "replace", false, "Replace existing hub item if it exists")
	hubCmd.AddCommand(hubAddCmd)
}

func runHubAdd(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	switch len(args) {
	case 0:
		// No args: Interactive mode for active profile
		return runInteractiveHubAdd(paths, "")

	case 1:
		// One arg: Could be profile name OR item type
		// Check if it's a valid hub type first
		if isValidHubType(config.HubItemType(args[0])) {
			return fmt.Errorf("missing name-or-path for type '%s'", args[0])
		}
		// Assume it's a profile name
		return runInteractiveHubAdd(paths, args[0])

	case 2:
		// Two args: Original behavior (type + name-or-path)
		itemType := config.HubItemType(args[0])
		if !isValidHubType(itemType) {
			return fmt.Errorf("invalid type: %s (valid: skills, agents, hooks, rules, commands, setting-fragments)", args[0])
		}
		if hubAddFromProfile != "" {
			return runHubAddFromProfile(paths, itemType, args[1])
		}
		return runHubAddFromPath(paths, itemType, args[1])
	}

	return nil
}

func runHubAddFromProfile(paths *config.Paths, itemType config.HubItemType, itemName string) error {
	mgr := profile.NewManager(paths)

	// Get the profile
	p, err := mgr.Get(hubAddFromProfile)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", hubAddFromProfile)
	}

	// Find the item in the profile
	profileItemPath := filepath.Join(p.Path, string(itemType), itemName)

	// Check if it's a symlink (already linked from hub)
	symMgr := symlink.New()
	isLink, _ := symMgr.IsSymlink(profileItemPath)
	if isLink {
		target, err := symMgr.ReadLink(profileItemPath)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		// Check if it points to hub
		if strings.HasPrefix(target, paths.HubDir) {
			return fmt.Errorf("item '%s' is already linked from hub (target: %s)", itemName, target)
		}
		// Resolve to actual content
		profileItemPath = target
	}

	// Check source exists
	srcInfo, err := os.Stat(profileItemPath)
	if err != nil {
		return fmt.Errorf("item not found in profile: %s/%s", itemType, itemName)
	}

	// Check if already exists in hub
	dstPath := paths.HubItemPath(itemType, itemName)
	if _, err := os.Stat(dstPath); err == nil {
		if !hubAddReplace {
			return fmt.Errorf("item already exists in hub: %s/%s (use --replace to overwrite)", itemType, itemName)
		}
		// Remove existing
		if err := os.RemoveAll(dstPath); err != nil {
			return fmt.Errorf("failed to remove existing hub item: %w", err)
		}
		fmt.Printf("Replacing existing hub item: %s/%s\n", itemType, itemName)
	}

	// Copy to hub
	if srcInfo.IsDir() {
		if err := copyDirRecursive(profileItemPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy directory: %w", err)
		}
	} else {
		if err := copyFileSimple(profileItemPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	fmt.Printf("Added %s/%s to hub from profile '%s'\n", itemType, itemName, hubAddFromProfile)

	// Offer to replace profile item with symlink
	fmt.Printf("\nTo link this item back to the profile, run:\n")
	fmt.Printf("  ccp link %s %s --profile=%s\n", itemType, itemName, hubAddFromProfile)

	return nil
}

func runHubAddFromPath(paths *config.Paths, itemType config.HubItemType, srcPath string) error {
	// Resolve source path
	srcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check source exists
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	// Get item name from source path
	itemName := filepath.Base(srcPath)

	// Check if already exists in hub
	dstPath := paths.HubItemPath(itemType, itemName)
	if _, err := os.Stat(dstPath); err == nil {
		if !hubAddReplace {
			return fmt.Errorf("item already exists: %s/%s (use --replace to overwrite)", itemType, itemName)
		}
		// Remove existing
		if err := os.RemoveAll(dstPath); err != nil {
			return fmt.Errorf("failed to remove existing hub item: %w", err)
		}
		fmt.Printf("Replacing existing hub item: %s/%s\n", itemType, itemName)
	}

	// Copy to hub
	if srcInfo.IsDir() {
		if err := copyDirRecursive(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy directory: %w", err)
		}
	} else {
		if err := copyFileSimple(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	fmt.Printf("Added %s/%s\n", itemType, itemName)
	return nil
}

func isValidHubType(t config.HubItemType) bool {
	for _, valid := range config.AllHubItemTypes() {
		if valid == t {
			return true
		}
	}
	return false
}

func copyFileSimple(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dest, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func copyDirRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileSimple(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// completeHubAddArgs provides tab completion for hub add command
func completeHubAddArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Complete with profile names or hub types
		paths, err := config.ResolvePaths()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		mgr := profile.NewManager(paths)
		profiles, err := mgr.List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var completions []string

		// Add profile names
		for _, p := range profiles {
			if strings.HasPrefix(p.Name, toComplete) {
				completions = append(completions, p.Name)
			}
		}

		// Add hub types
		for _, t := range config.AllHubItemTypes() {
			if strings.HasPrefix(string(t), toComplete) {
				completions = append(completions, string(t))
			}
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	}

	if len(args) == 1 {
		// If first arg was a hub type, complete with file paths
		if isValidHubType(config.HubItemType(args[0])) {
			return nil, cobra.ShellCompDirectiveDefault // File completion
		}
		// Otherwise no more args needed (profile name was given)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}

// NonHubItem represents a profile item that is not linked from hub
type NonHubItem struct {
	Name       string
	Path       string
	IsDir      bool
	IsExternal bool // Symlink pointing outside hub
}

// runInteractiveHubAdd runs interactive mode for promoting local items to hub
func runInteractiveHubAdd(paths *config.Paths, profileName string) error {
	mgr := profile.NewManager(paths)

	// Resolve profile
	var p *profile.Profile
	var err error

	if profileName == "" {
		// Use active profile
		p, err = mgr.GetActive()
		if err != nil {
			return fmt.Errorf("failed to get active profile: %w", err)
		}
		if p == nil {
			return fmt.Errorf("no active profile: specify a profile name or run 'ccp use <profile>'")
		}
		profileName = p.Name
	} else {
		p, err = mgr.Get(profileName)
		if err != nil {
			return fmt.Errorf("failed to get profile: %w", err)
		}
		if p == nil {
			return fmt.Errorf("profile not found: %s", profileName)
		}
	}

	// Scan profile for non-hub items
	nonHubItems, err := scanNonHubItems(paths, p)
	if err != nil {
		return fmt.Errorf("failed to scan profile: %w", err)
	}

	// Build tabs for picker
	var tabs []picker.Tab
	for _, itemType := range config.AllHubItemTypes() {
		items, ok := nonHubItems[itemType]
		if !ok || len(items) == 0 {
			continue
		}

		var pickerItems []picker.Item
		for _, item := range items {
			pickerItems = append(pickerItems, picker.Item{
				ID:       item.Name,
				Label:    item.Name,
				Selected: false,
			})
		}

		tabs = append(tabs, picker.Tab{
			Name:  string(itemType),
			Items: pickerItems,
		})
	}

	if len(tabs) == 0 {
		fmt.Println("No local items found in profile to promote to hub")
		fmt.Println("All items are either already in hub or linked from hub")
		return nil
	}

	fmt.Printf("Promote local items from profile '%s' to hub\n\n", profileName)

	// Run tabbed picker
	selections, err := picker.RunTabbed(tabs)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}
	if selections == nil {
		fmt.Println("Cancelled")
		return nil
	}

	// Count selected items
	totalAdded := 0
	for _, items := range selections {
		totalAdded += len(items)
	}

	if totalAdded == 0 {
		fmt.Println("No items selected")
		return nil
	}

	// Promote selected items
	needsSettingsRegen := false
	for _, itemType := range config.AllHubItemTypes() {
		items, ok := selections[string(itemType)]
		if !ok || len(items) == 0 {
			continue
		}

		for _, itemName := range items {
			if err := promoteToHub(paths, p, itemType, itemName); err != nil {
				fmt.Printf("Warning: failed to promote %s/%s: %v\n", itemType, itemName, err)
				continue
			}
			fmt.Printf("Promoted %s/%s to hub\n", itemType, itemName)
		}

		// Track if settings regeneration is needed
		if itemType == config.HubHooks || itemType == config.HubSettingFragments {
			needsSettingsRegen = true
		}
	}

	// Save updated manifest
	manifestPath := profile.ManifestPath(p.Path)
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	// Regenerate settings.json if hooks or fragments were promoted
	if needsSettingsRegen {
		if err := profile.RegenerateSettings(paths, p.Path, p.Manifest); err != nil {
			fmt.Printf("Warning: failed to regenerate settings.json: %v\n", err)
		}
	}

	fmt.Printf("\nPromoted %d item(s) to hub from profile '%s'\n", totalAdded, profileName)
	return nil
}

// scanNonHubItems finds items in a profile that are not linked from the hub
func scanNonHubItems(paths *config.Paths, p *profile.Profile) (map[config.HubItemType][]NonHubItem, error) {
	result := make(map[config.HubItemType][]NonHubItem)
	symMgr := symlink.New()

	for _, itemType := range config.AllHubItemTypes() {
		itemDir := filepath.Join(p.Path, string(itemType))

		entries, err := os.ReadDir(itemDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var items []NonHubItem
		for _, entry := range entries {
			itemPath := filepath.Join(itemDir, entry.Name())
			itemName := entry.Name()

			// Check if it's a symlink
			isLink, err := symMgr.IsSymlink(itemPath)
			if err != nil {
				continue
			}

			if isLink {
				// Read symlink target
				target, err := symMgr.ReadLink(itemPath)
				if err != nil {
					continue
				}

				// Resolve relative symlinks
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(itemPath), target)
				}

				// Check if it points to hub
				absTarget, _ := filepath.Abs(target)
				absHubDir, _ := filepath.Abs(paths.HubDir)

				if strings.HasPrefix(absTarget, absHubDir+string(os.PathSeparator)) {
					// This is a hub-linked item, skip it
					continue
				}

				// External symlink - treat as promotable
				items = append(items, NonHubItem{
					Name:       itemName,
					Path:       itemPath,
					IsDir:      false, // Will check actual target
					IsExternal: true,
				})
			} else {
				// Local file/directory - promotable
				items = append(items, NonHubItem{
					Name:  itemName,
					Path:  itemPath,
					IsDir: entry.IsDir(),
				})
			}
		}

		if len(items) > 0 {
			result[itemType] = items
		}
	}

	return result, nil
}

// promoteToHub moves an item from profile to hub and creates a symlink back
func promoteToHub(paths *config.Paths, p *profile.Profile, itemType config.HubItemType, itemName string) error {
	symMgr := symlink.New()

	profileItemPath := filepath.Join(p.Path, string(itemType), itemName)
	hubItemPath := paths.HubItemPath(itemType, itemName)

	// Check if already exists in hub
	if _, err := os.Stat(hubItemPath); err == nil {
		if !hubAddReplace {
			return fmt.Errorf("item already exists in hub: %s/%s (use --replace to overwrite)", itemType, itemName)
		}
		// Remove existing hub item
		if err := os.RemoveAll(hubItemPath); err != nil {
			return fmt.Errorf("failed to remove existing hub item: %w", err)
		}
	}

	// Handle symlinks pointing to external locations
	isLink, _ := symMgr.IsSymlink(profileItemPath)
	var srcPath string
	if isLink {
		target, err := symMgr.ReadLink(profileItemPath)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(profileItemPath), target)
		}
		srcPath = target
	} else {
		srcPath = profileItemPath
	}

	// Get source info
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	// Copy to hub
	if srcInfo.IsDir() {
		if err := copyDirRecursive(srcPath, hubItemPath); err != nil {
			return fmt.Errorf("failed to copy directory to hub: %w", err)
		}
	} else {
		if err := copyFileSimple(srcPath, hubItemPath); err != nil {
			return fmt.Errorf("failed to copy file to hub: %w", err)
		}
	}

	// Remove original from profile
	if err := os.RemoveAll(profileItemPath); err != nil {
		return fmt.Errorf("failed to remove original item: %w", err)
	}

	// Create symlink: profile -> hub
	if err := symMgr.Create(profileItemPath, hubItemPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Update manifest to track the hub item
	p.Manifest.AddHubItem(itemType, itemName)

	return nil
}
