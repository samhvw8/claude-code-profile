package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileCloneCmd = &cobra.Command{
	Use:   "clone <source> <new-name>",
	Short: "Clone an existing profile",
	Long: `Create a new profile by copying an existing one.

This is equivalent to: ccp profile create <new-name> --from=<source>

Examples:
  ccp profile clone default dev
  ccp profile clone minimal quickfix`,
	Args: cobra.ExactArgs(2),
	RunE: runProfileClone,
}

func init() {
	profileCmd.AddCommand(profileCloneCmd)
}

func runProfileClone(cmd *cobra.Command, args []string) error {
	sourceName := args[0]
	newName := args[1]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	// Check source exists
	source, err := mgr.Get(sourceName)
	if err != nil {
		return fmt.Errorf("failed to get source profile: %w", err)
	}
	if source == nil {
		return fmt.Errorf("source profile not found: %s", sourceName)
	}

	// Check new doesn't exist
	if mgr.Exists(newName) {
		return fmt.Errorf("profile already exists: %s", newName)
	}

	// Create new manifest from source
	manifest := profile.NewManifest(newName, fmt.Sprintf("Cloned from %s", sourceName))

	// Copy hub links
	for _, itemType := range config.AllHubItemTypes() {
		manifest.SetHubItems(itemType, source.Manifest.GetHubItems(itemType))
	}

	// Copy data config
	manifest.Data = source.Manifest.Data

	// Create the profile
	p, err := mgr.Create(newName, manifest)
	if err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}

	fmt.Printf("Cloned %s -> %s\n", sourceName, newName)
	fmt.Printf("Location: %s\n", p.Path)

	// Print summary
	var summaryParts []string
	for _, itemType := range config.AllHubItemTypes() {
		items := manifest.GetHubItems(itemType)
		if len(items) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d %s", len(items), itemType))
		}
	}
	if len(summaryParts) > 0 {
		fmt.Printf("Linked: %s\n", strings.Join(summaryParts, ", "))
	}

	return nil
}
