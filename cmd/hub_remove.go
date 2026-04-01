package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	hubRemoveForce       bool
	hubRemoveInteractive bool
)

var hubRemoveCmd = &cobra.Command{
	Use:   "remove [type/name]",
	Short: "Remove an item from the hub",
	Long: `Remove a file or directory from the hub.

Examples:
  ccp hub remove skills/my-skill
  ccp hub remove agents/my-agent
  ccp hub remove -i                   # Interactive picker`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHubRemove,
}

func init() {
	hubRemoveCmd.Flags().BoolVarP(&hubRemoveForce, "force", "f", false, "Skip confirmation and usage check")
	hubRemoveCmd.Flags().BoolVarP(&hubRemoveInteractive, "interactive", "i", false, "Interactive picker for hub items to remove")
	hubCmd.AddCommand(hubRemoveCmd)
}

func runHubRemove(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Interactive mode
	if hubRemoveInteractive || len(args) == 0 {
		return runHubRemoveInteractive(paths)
	}

	// Direct mode
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: use <type>/<name> (e.g., skills/my-skill)")
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s", parts[0])
	}

	return removeHubItem(paths, itemType, itemName)
}

func runHubRemoveInteractive(paths *config.Paths) error {
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	var tabs []picker.Tab
	for _, itemType := range config.AllHubItemTypes() {
		items := h.GetItems(itemType)
		if len(items) == 0 {
			continue
		}
		var pickerItems []picker.Item
		for _, item := range items {
			pickerItems = append(pickerItems, picker.Item{
				ID:    item.Name,
				Label: item.Name,
			})
		}
		tabs = append(tabs, picker.Tab{
			Name:  string(itemType),
			Items: pickerItems,
		})
	}

	if len(tabs) == 0 {
		fmt.Println("No hub items to remove")
		return nil
	}

	fmt.Println("Select hub items to remove:")
	selections, err := picker.RunTabbed(tabs)
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}
	if selections == nil {
		fmt.Println("Cancelled")
		return nil
	}

	removed := 0
	for _, itemType := range config.AllHubItemTypes() {
		if items, ok := selections[string(itemType)]; ok {
			for _, name := range items {
				if err := removeHubItem(paths, itemType, name); err != nil {
					fmt.Printf("  Warning: %v\n", err)
				} else {
					removed++
				}
			}
		}
	}

	if removed == 0 {
		fmt.Println("No items selected for removal")
	}
	return nil
}

func removeHubItem(paths *config.Paths, itemType config.HubItemType, itemName string) error {
	itemPath := resolveHubItemPath(paths, itemType, itemName)
	if itemPath == "" {
		return fmt.Errorf("item not found: %s/%s", itemType, itemName)
	}

	// Check which profiles use this item
	if !hubRemoveForce {
		usedBy, err := findProfilesUsingItem(paths, itemType, itemName)
		if err != nil {
			return fmt.Errorf("failed to check usage: %w", err)
		}

		if len(usedBy) > 0 {
			fmt.Printf("Warning: %s/%s is used by profiles: %s\n", itemType, itemName, strings.Join(usedBy, ", "))
			fmt.Print("Remove anyway? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Println("Skipped")
				return nil
			}
		}
	}

	if err := os.RemoveAll(itemPath); err != nil {
		return fmt.Errorf("failed to remove: %w", err)
	}

	fmt.Printf("Removed %s/%s\n", itemType, itemName)
	return nil
}

// resolveHubItemPath finds the actual path of a hub item, trying the exact name
// first, then common file extensions (.md, .yaml, .yml, .sh, .json)
func resolveHubItemPath(paths *config.Paths, itemType config.HubItemType, name string) string {
	exact := paths.HubItemPath(itemType, name)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}

	dir := paths.HubItemDir(itemType)
	for _, ext := range []string{".md", ".yaml", ".yml", ".sh", ".json"} {
		candidate := filepath.Join(dir, name+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func findProfilesUsingItem(paths *config.Paths, itemType config.HubItemType, itemName string) ([]string, error) {
	var usedBy []string

	entries, err := os.ReadDir(paths.ProfilesDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileDir := filepath.Join(paths.ProfilesDir, entry.Name())
		manifestPath := profile.ManifestPath(profileDir)

		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		linkedItems := manifest.GetHubItems(itemType)
		for _, linked := range linkedItems {
			if linked == itemName {
				usedBy = append(usedBy, entry.Name())
				break
			}
		}
	}

	return usedBy, nil
}
