package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var (
	fixDryRun bool
	fixForce  bool
)

var profileFixCmd = &cobra.Command{
	Use:   "fix <name>",
	Short: "Reconcile profile to match its manifest",
	Long: `Fix configuration drift by reconciling the profile directory to match profile.toml.

Actions taken:
  - Create missing symlinks
  - Remove extra items not in manifest
  - Recreate broken or mismatched symlinks
  - Remove references to non-existent hub items from manifest

Use --dry-run to preview changes without executing.
Use --force to auto-remove non-existent hub items without confirmation.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileFix,
}

func init() {
	profileFixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Preview changes without executing")
	profileFixCmd.Flags().BoolVarP(&fixForce, "force", "f", false, "Auto-remove non-existent hub items from manifest")
	profileCmd.AddCommand(profileFixCmd)
}

func runProfileFix(cmd *cobra.Command, args []string) error {
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
		fmt.Printf("Profile '%s' is already in sync - no fixes needed\n", profileName)
		return nil
	}

	// Build fix options
	opts := profile.FixOptions{
		DryRun: fixDryRun,
		Force:  fixForce,
	}

	// Set up confirmation callback for hub_missing items (only if not dry-run and not force)
	if !fixDryRun && !fixForce {
		opts.ConfirmHubMissing = func(items []profile.DriftItem) ([]profile.DriftItem, error) {
			if len(items) == 0 {
				return nil, nil
			}

			fmt.Printf("Found %d hub item(s) that no longer exist:\n", len(items))
			for _, item := range items {
				fmt.Printf("  - %s/%s\n", item.ItemType, item.ItemName)
			}
			fmt.Println()

			fmt.Print("Remove these items from manifest? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}

			input = strings.TrimSpace(strings.ToLower(input))
			if input == "y" || input == "yes" {
				return items, nil
			}

			fmt.Println("Skipped removing non-existent hub items from manifest")
			return nil, nil
		}
	}

	// Apply fixes
	result, err := detector.Fix(p, report, opts)
	if err != nil {
		return fmt.Errorf("failed to fix drift: %w", err)
	}

	if len(result.Actions) == 0 {
		fmt.Printf("Profile '%s' is already in sync - no fixes needed\n", profileName)
		return nil
	}

	if fixDryRun {
		fmt.Println("Dry run - changes that would be made:")
	} else {
		fmt.Println("Changes applied:")
	}

	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}

	if !fixDryRun {
		fmt.Printf("\nProfile '%s' is now in sync\n", profileName)
	}

	return nil
}
