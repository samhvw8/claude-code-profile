package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/migration"
	"github.com/samhoang/ccp/internal/picker"
)

var (
	initDryRun      bool
	initForce       bool
	initAllFragments bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize ccp from existing ~/.claude",
	Long: `Migrate existing ~/.claude configuration to ccp structure.

This command:
1. Creates ~/.ccp/hub/ with all hub-eligible items (skills, agents, hooks, etc.)
2. Creates ~/.ccp/profiles/default/ as the default profile
3. Creates ~/.ccp/profiles/shared/ for shared data
4. Replaces ~/.claude with a symlink to the active profile

By default, setting fragments are interactively selected. Use --all-fragments to export all.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initDryRun, "dry-run", false, "Show migration plan without executing")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing hub structure")
	initCmd.Flags().BoolVar(&initAllFragments, "all-fragments", false, "Export all setting fragments without interactive selection")
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

	// Handle setting fragments
	if len(plan.SettingFragments) > 0 {
		fmt.Printf("  Setting fragments: %d available\n", len(plan.SettingFragments))

		if !initAllFragments && !initDryRun {
			// Interactive selection
			fmt.Println("\nSelect setting fragments to include in the default profile:")

			var pickerItems []picker.Item
			for _, fragment := range plan.SettingFragments {
				label := fragment.Name
				if fragment.Description != "" {
					label = fmt.Sprintf("%s - %s", fragment.Name, fragment.Description)
				}
				pickerItems = append(pickerItems, picker.Item{
					ID:       fragment.Name,
					Label:    label,
					Selected: true, // Default all selected
				})
			}

			selected, err := picker.Run("Setting Fragments", pickerItems)
			if err != nil {
				return fmt.Errorf("fragment selection failed: %w", err)
			}

			if selected == nil {
				fmt.Println("Cancelled")
				return nil
			}

			// Filter fragments based on selection
			selectedMap := make(map[string]bool)
			for _, id := range selected {
				selectedMap[id] = true
			}

			var filteredFragments []migration.SettingFragment
			for _, fragment := range plan.SettingFragments {
				if selectedMap[fragment.Name] {
					filteredFragments = append(filteredFragments, fragment)
				}
			}
			plan.SettingFragments = filteredFragments

			fmt.Printf("\nSelected %d fragments\n", len(filteredFragments))
		}
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

	// Initialize config file
	if err := initConfigFile(paths); err != nil {
		fmt.Printf("Warning: could not create config file: %v\n", err)
	}

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

// initConfigFile creates the default ccp.toml if it doesn't exist
func initConfigFile(paths *config.Paths) error {
	configPath := paths.CcpDir + "/ccp.toml"
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists
	}

	cfg := config.DefaultCcpConfig()
	if err := cfg.Save(paths.CcpDir); err != nil {
		return err
	}
	fmt.Printf("Created config: %s\n", configPath)
	return nil
}
