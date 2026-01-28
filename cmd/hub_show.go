package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var hubShowCmd = &cobra.Command{
	Use:   "show <type>/<name>",
	Short: "Show hub item contents and usage",
	Long: `Display the contents of a hub item and which profiles use it.

Examples:
  ccp hub show skills/my-skill.md
  ccp hub show agents/my-agent`,
	Args: cobra.ExactArgs(1),
	RunE: runHubShow,
}

func init() {
	hubCmd.AddCommand(hubShowCmd)
}

func runHubShow(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("invalid format: use <type>/<name>")
	}

	itemType := config.HubItemType(parts[0])
	itemName := parts[1]

	if !isValidHubType(itemType) {
		return fmt.Errorf("invalid type: %s", parts[0])
	}

	itemPath := paths.HubItemPath(itemType, itemName)
	info, err := os.Stat(itemPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("item not found: %s/%s", itemType, itemName)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Item: %s/%s\n", itemType, itemName)
	fmt.Printf("Path: %s\n", itemPath)

	if info.IsDir() {
		fmt.Println("Type: directory")
		entries, err := os.ReadDir(itemPath)
		if err == nil {
			fmt.Printf("Contents: %d items\n", len(entries))
			for _, e := range entries {
				indicator := ""
				if e.IsDir() {
					indicator = "/"
				}
				fmt.Printf("  - %s%s\n", e.Name(), indicator)
			}
		}
	} else {
		fmt.Println("Type: file")
		fmt.Printf("Size: %d bytes\n", info.Size())
		fmt.Println()
		fmt.Println("--- Contents ---")
		content, err := os.ReadFile(itemPath)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		fmt.Println(string(content))
		fmt.Println("--- End ---")
	}

	// Show which profiles use this item
	usedBy, err := findProfilesUsingItemByName(paths, itemType, itemName)
	if err == nil && len(usedBy) > 0 {
		fmt.Println()
		fmt.Printf("Used by profiles: %s\n", strings.Join(usedBy, ", "))
	}

	return nil
}

func findProfilesUsingItemByName(paths *config.Paths, itemType config.HubItemType, itemName string) ([]string, error) {
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
