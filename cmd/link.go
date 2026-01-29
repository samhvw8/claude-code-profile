package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

var linkCmd = &cobra.Command{
	Use:   "link <profile> <path>",
	Short: "Add a hub item to a profile",
	Long: `Add a hub item to a profile by creating a symlink.

The path should be in format: type/name
Examples:
  ccp link quickfix skills/debugging-core
  ccp link dev-fullstack hooks/pre-commit-lint`,
	Args: cobra.ExactArgs(2),
	RunE: runLink,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	profileName := args[0]
	itemPath := args[1]

	// Parse item path
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

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
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
	mgr := profile.NewManager(paths)

	if err := mgr.LinkHubItem(profileName, itemType, itemName); err != nil {
		return fmt.Errorf("failed to link: %w", err)
	}

	fmt.Printf("Linked %s/%s to profile %s\n", itemType, itemName, profileName)
	return nil
}
