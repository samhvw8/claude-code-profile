package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	hubRemoveForce bool
)

var hubRemoveCmd = &cobra.Command{
	Use:   "remove <type>/<name>",
	Short: "Remove an item from the hub",
	Long: `Remove a file or directory from the hub.

Examples:
  ccp hub remove skills/my-skill.md
  ccp hub remove agents/my-agent`,
	Args: cobra.ExactArgs(1),
	RunE: runHubRemove,
}

func init() {
	hubRemoveCmd.Flags().BoolVarP(&hubRemoveForce, "force", "f", false, "Skip confirmation and usage check")
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

	// Parse type/name
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: use <type>/<name> (e.g., skills/my-skill.md)")
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s", parts[0])
	}

	// Check if item exists
	itemPath := paths.HubItemPath(itemType, itemName)
	if _, err := os.Stat(itemPath); os.IsNotExist(err) {
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
				fmt.Println("Aborted")
				return nil
			}
		}
	}

	// Remove the item
	if err := os.RemoveAll(itemPath); err != nil {
		return fmt.Errorf("failed to remove: %w", err)
	}

	fmt.Printf("Removed %s/%s\n", itemType, itemName)
	return nil
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
		manifestPath := filepath.Join(profileDir, "profile.yaml")

		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		// Check if profile links to this item
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
