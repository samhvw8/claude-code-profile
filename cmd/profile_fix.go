package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

var fixDryRun bool

var profileFixCmd = &cobra.Command{
	Use:   "fix <name>",
	Short: "Reconcile profile to match its manifest",
	Long: `Fix configuration drift by reconciling the profile directory to match profile.yaml.

Actions taken:
  - Create missing symlinks
  - Remove extra items not in manifest
  - Recreate broken or mismatched symlinks

Use --dry-run to preview changes without executing.`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileFix,
}

func init() {
	profileFixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Preview changes without executing")
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

	// Apply fixes
	actions, err := detector.Fix(p, report, fixDryRun)
	if err != nil {
		return fmt.Errorf("failed to fix drift: %w", err)
	}

	if fixDryRun {
		fmt.Println("Dry run - changes that would be made:")
	} else {
		fmt.Println("Changes applied:")
	}

	for _, action := range actions {
		fmt.Printf("  - %s\n", action)
	}

	if !fixDryRun {
		fmt.Printf("\nProfile '%s' is now in sync\n", profileName)
	}

	return nil
}
