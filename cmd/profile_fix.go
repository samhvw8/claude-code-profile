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
	fixAll    bool
)

var profileFixCmd = &cobra.Command{
	Use:   "fix [name]",
	Short: "Reconcile profile to match its manifest",
	Long: `Fix configuration drift by reconciling the profile directory to match profile.toml.

Actions taken:
  - Create missing symlinks
  - Remove extra items not in manifest
  - Recreate broken or mismatched symlinks
  - Remove references to non-existent hub items from manifest

Use --dry-run to preview changes without executing.
Use --force to auto-remove non-existent hub items without confirmation.
Use --all to fix all profiles at once.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE:              runProfileFix,
}

func init() {
	profileFixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Preview changes without executing")
	profileFixCmd.Flags().BoolVarP(&fixForce, "force", "f", false, "Auto-remove non-existent hub items from manifest")
	profileFixCmd.Flags().BoolVar(&fixAll, "all", false, "Fix all profiles")
	profileCmd.AddCommand(profileFixCmd)
}

func runProfileFix(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized: run 'ccp init' first")
	}

	mgr := profile.NewManager(paths)

	if fixAll {
		profiles, err := mgr.List()
		if err != nil {
			return fmt.Errorf("failed to list profiles: %w", err)
		}

		for _, p := range profiles {
			fmt.Printf("Fixing profile: %s\n", p.Name)
			if err := fixProfile(paths, p); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
			}
		}
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a profile name or use --all")
	}

	profileName := args[0]
	p, err := mgr.Get(profileName)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}
	if p == nil {
		return fmt.Errorf("profile not found: %s", profileName)
	}

	fmt.Printf("Fixing profile: %s\n", p.Name)
	return fixProfile(paths, p)
}

func fixProfile(paths *config.Paths, p *profile.Profile) error {
	detector := profile.NewDetector(paths)
	report, err := detector.Detect(p)
	if err != nil {
		return fmt.Errorf("failed to detect drift: %w", err)
	}

	if !report.HasDrift() {
		fmt.Printf("  Profile '%s' is already in sync - no fixes needed\n", p.Name)
		return nil
	}

	// Build fix options
	opts := profile.FixOptions{
		DryRun: fixDryRun,
		Force:  fixForce,
	}

	// Set up confirmation callback for hub_missing items
	if !fixDryRun && !fixForce {
		if fixAll {
			// --all without --force: skip hub_missing items (no interactive prompt per profile)
			opts.ConfirmHubMissing = func(items []profile.DriftItem) ([]profile.DriftItem, error) {
				if len(items) > 0 {
					fmt.Printf("  Skipping %d non-existent hub item(s) (use --force to auto-remove)\n", len(items))
				}
				return nil, nil
			}
		} else {
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
	}

	// Apply fixes
	result, err := detector.Fix(p, report, opts)
	if err != nil {
		return fmt.Errorf("failed to fix drift: %w", err)
	}

	if len(result.Actions) == 0 {
		fmt.Printf("  Profile '%s' is already in sync - no fixes needed\n", p.Name)
		return nil
	}

	if fixDryRun {
		fmt.Println("  Dry run - changes that would be made:")
	}

	for _, action := range result.Actions {
		fmt.Printf("  - %s\n", action)
	}

	if !fixDryRun {
		fmt.Printf("  Profile '%s' is now in sync\n", p.Name)
	}

	return nil
}
