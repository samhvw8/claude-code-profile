package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink <profile> <path>",
	Short: "Remove a hub item from a profile",
	Long: `Remove a hub item from a profile by deleting its symlink.

The path should be in format: type/name
Examples:
  ccp unlink quickfix skills/debugging-core
  ccp unlink dev-fullstack hooks/pre-commit-lint`,
	Args: cobra.ExactArgs(2),
	RunE: runUnlink,
}

func init() {
	rootCmd.AddCommand(unlinkCmd)
}

func runUnlink(cmd *cobra.Command, args []string) error {
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

	// Unlink from profile
	mgr := profile.NewManager(paths)

	if err := mgr.UnlinkHubItem(profileName, itemType, itemName); err != nil {
		return fmt.Errorf("failed to unlink: %w", err)
	}

	fmt.Printf("Unlinked %s/%s from profile %s\n", itemType, itemName, profileName)
	return nil
}
