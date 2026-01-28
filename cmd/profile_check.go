package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var profileCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Validate profile against its manifest",
	Long: `Check if a profile directory matches its profile.yaml manifest.

Reports:
  - missing: items in manifest but not in directory
  - extra: items in directory but not in manifest
  - broken: symlinks that point to non-existent targets
  - mismatched: symlinks pointing to wrong hub items

Exit codes:
  0 - profile is valid
  1 - drift detected`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileCheck,
}

func init() {
	profileCmd.AddCommand(profileCheckCmd)
}

func runProfileCheck(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	detector := profile.NewDetector(paths)
	report, err := detector.Detect(p)
	if err != nil {
		return fmt.Errorf("failed to detect drift: %w", err)
	}

	if !report.HasDrift() {
		fmt.Printf("Profile '%s' is valid - no drift detected\n", profileName)
		return nil
	}

	// Group and print issues
	byType := report.IssuesByType()

	fmt.Printf("Profile '%s' has configuration drift:\n\n", profileName)

	if items, ok := byType[profile.DriftMissing]; ok {
		fmt.Println("Missing (in manifest but not in directory):")
		for _, item := range items {
			fmt.Printf("  - %s/%s\n", item.ItemType, item.ItemName)
		}
		fmt.Println()
	}

	if items, ok := byType[profile.DriftExtra]; ok {
		fmt.Println("Extra (in directory but not in manifest):")
		for _, item := range items {
			fmt.Printf("  - %s/%s\n", item.ItemType, item.ItemName)
		}
		fmt.Println()
	}

	if items, ok := byType[profile.DriftBroken]; ok {
		fmt.Println("Broken (symlink target does not exist):")
		for _, item := range items {
			fmt.Printf("  - %s/%s -> %s\n", item.ItemType, item.ItemName, item.Actual)
		}
		fmt.Println()
	}

	if items, ok := byType[profile.DriftMismatched]; ok {
		fmt.Println("Mismatched (symlink points to wrong target):")
		for _, item := range items {
			fmt.Printf("  - %s/%s\n", item.ItemType, item.ItemName)
			fmt.Printf("      expected: %s\n", item.Expected)
			fmt.Printf("      actual:   %s\n", item.Actual)
		}
		fmt.Println()
	}

	fmt.Printf("Run 'ccp profile fix %s' to reconcile\n", profileName)

	// Exit with non-zero code to indicate drift
	os.Exit(1)
	return nil
}
