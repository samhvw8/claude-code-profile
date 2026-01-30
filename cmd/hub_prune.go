package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/picker"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	pruneForce       bool
	pruneInteractive bool
	pruneType        string
)

var hubPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove unused hub items",
	Long: `Remove hub items that are not used by any profile.

By default, shows a list of orphaned items and asks for confirmation.
Use --interactive (-i) to select which items to remove.
Use --force (-f) to remove all orphaned items without confirmation.

Examples:
  ccp hub prune                  # Show orphans, confirm removal
  ccp hub prune -i               # Interactive selection
  ccp hub prune -f               # Remove all orphans without confirmation
  ccp hub prune --type=skills    # Only prune skills`,
	RunE: runHubPrune,
}

func init() {
	hubPruneCmd.Flags().BoolVarP(&pruneForce, "force", "f", false, "Remove all orphaned items without confirmation")
	hubPruneCmd.Flags().BoolVarP(&pruneInteractive, "interactive", "i", false, "Interactively select items to remove")
	hubPruneCmd.Flags().StringVar(&pruneType, "type", "", "Only prune specific type (skills, agents, hooks, rules, commands)")
	hubCmd.AddCommand(hubPruneCmd)
}

func runHubPrune(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	// Scan hub
	scanner := hub.NewScanner()
	h, err := scanner.Scan(paths.HubDir)
	if err != nil {
		return fmt.Errorf("failed to scan hub: %w", err)
	}

	// Build usage map
	usage := make(map[string][]string)

	// Initialize with all hub items
	for _, item := range h.AllItems() {
		key := fmt.Sprintf("%s/%s", item.Type, item.Name)
		usage[key] = []string{}
	}

	// Scan profiles for usage
	entries, err := os.ReadDir(paths.ProfilesDir)
	if err != nil {
		return fmt.Errorf("failed to read profiles: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}

		profileName := entry.Name()
		manifestPath := filepath.Join(paths.ProfilesDir, profileName, "profile.yaml")

		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		for _, itemType := range config.AllHubItemTypes() {
			items := manifest.GetHubItems(itemType)
			for _, itemName := range items {
				key := fmt.Sprintf("%s/%s", itemType, itemName)
				if _, exists := usage[key]; exists {
					usage[key] = append(usage[key], profileName)
				}
			}
		}
	}

	// Find orphans
	var orphans []string
	for key, profiles := range usage {
		if len(profiles) == 0 {
			// Filter by type if specified
			if pruneType != "" {
				parts := strings.SplitN(key, "/", 2)
				if len(parts) == 2 && parts[0] != pruneType {
					continue
				}
			}
			orphans = append(orphans, key)
		}
	}

	sort.Strings(orphans)

	if len(orphans) == 0 {
		fmt.Println("No orphaned hub items found - nothing to prune")
		return nil
	}

	fmt.Printf("Found %d orphaned hub items:\n\n", len(orphans))

	var toRemove []string

	if pruneInteractive {
		// Interactive picker mode
		var pickerItems []picker.Item
		for _, orphan := range orphans {
			pickerItems = append(pickerItems, picker.Item{
				ID:       orphan,
				Label:    orphan,
				Selected: false,
			})
		}

		selected, err := picker.Run("Select items to remove:", pickerItems)
		if err != nil {
			return fmt.Errorf("picker error: %w", err)
		}

		if len(selected) == 0 {
			fmt.Println("No items selected - nothing removed")
			return nil
		}

		toRemove = selected
	} else if pruneForce {
		// Remove all without confirmation
		toRemove = orphans
	} else {
		// Show list and ask for confirmation
		for _, orphan := range orphans {
			fmt.Printf("  - %s\n", orphan)
		}
		fmt.Println()

		fmt.Print("Remove all orphaned items? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("Cancelled")
			return nil
		}

		toRemove = orphans
	}

	// Remove items
	removed := 0
	for _, item := range toRemove {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) != 2 {
			continue
		}

		itemType := parts[0]
		itemName := parts[1]
		itemPath := filepath.Join(paths.HubDir, itemType, itemName)

		if err := os.RemoveAll(itemPath); err != nil {
			fmt.Printf("  Warning: failed to remove %s: %v\n", item, err)
		} else {
			fmt.Printf("  Removed: %s\n", item)
			removed++
		}
	}

	fmt.Printf("\nRemoved %d items\n", removed)

	return nil
}
