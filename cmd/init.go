package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/migration"
)

var (
	initDryRun bool
	initForce  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize ccp from existing ~/.claude",
	Long: `Migrate existing ~/.claude configuration to ccp structure.

This command:
1. Creates ~/.ccp/hub/ with all hub-eligible items (skills, agents, hooks, etc.)
2. Creates ~/.ccp/profiles/default/ as the default profile
3. Creates ~/.ccp/profiles/shared/ for shared data
4. Replaces ~/.claude with a symlink to the active profile`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initDryRun, "dry-run", false, "Show migration plan without executing")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing hub structure")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	// Check if claude directory exists as a real directory (not symlink)
	if !paths.ClaudeDirExistsAsDir() {
		if paths.ClaudeDirIsSymlink() {
			if !initForce {
				return fmt.Errorf("~/.claude is already a symlink (ccp may already be initialized)\n\nUse --force to reinitialize")
			}
			// Force mode: we need to reset first, then reinit won't work without a real directory
			return fmt.Errorf("~/.claude is a symlink - run 'ccp reset' first to restore, then 'ccp init'")
		}
		return fmt.Errorf("~/.claude directory not found\n\nPlease create a Claude Code configuration first by running claude")
	}

	// Check if already initialized
	if paths.IsInitialized() && !initForce {
		return fmt.Errorf("ccp already initialized (~/.ccp exists)\n\nUse --force to reinitialize (this may cause data loss)")
	}

	migrator := migration.NewMigrator(paths)

	// Create migration plan
	plan, err := migrator.Plan()
	if err != nil {
		return fmt.Errorf("failed to create migration plan: %w", err)
	}

	// Print plan
	fmt.Println("Migration Plan:")
	fmt.Println()

	totalItems := 0
	for itemType, items := range plan.HubItems {
		if len(items) > 0 {
			fmt.Printf("  %s: %d items\n", itemType, len(items))
			totalItems += len(items)
		}
	}

	if totalItems == 0 {
		fmt.Println("  No hub-eligible items found")
	}

	if len(plan.FilesToCopy) > 0 {
		fmt.Printf("\n  Config files: %v\n", plan.FilesToCopy)
	}

	if len(plan.DataDirs) > 0 {
		fmt.Printf("  Data directories: %v\n", plan.DataDirs)
	}

	fmt.Println()

	if initDryRun {
		fmt.Println("Dry run - no changes made")
		return nil
	}

	// Execute migration
	fmt.Println("Executing migration...")
	if err := migrator.Execute(plan, false); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Migration complete!")
	fmt.Println()
	fmt.Println("Your Claude Code configuration is now managed by ccp.")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Verify: ccp profile check default\n")
	fmt.Printf("  2. List profiles: ccp profile list\n")
	fmt.Printf("  3. Create new profile: ccp profile create <name>\n")
	fmt.Println()
	fmt.Println("To use a profile per-project, add to your .envrc or .mise.toml:")
	fmt.Printf("  export CLAUDE_CONFIG_DIR=\"%s/profiles/<name>\"\n", paths.CcpDir)

	return nil
}
