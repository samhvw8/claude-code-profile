package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/migration"
)

var (
	migrateDryRun bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run migrations from older ccp versions",
	Long: `Run migrations to update ccp data from older versions.

This command:
1. Migrates profile manifests from YAML to TOML (profile.yaml → profile.toml)
2. Upgrades v2 manifests to v3
3. Flattens engine/context references into inline profile hub items
4. Converts setting-fragments to a settings template
5. Cleans up stale profile.yaml files

Migrations are idempotent and safe to run multiple times.`,
	RunE: runMigrate,
}

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be migrated without making changes")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	if !paths.IsInitialized() {
		return fmt.Errorf("ccp not initialized - run 'ccp init' first")
	}

	tomlMigrator := migration.NewTOMLMigrator(paths)
	flattenMigrator := migration.NewFlattenMigrator(paths)
	fragmentMigrator := migration.NewFragmentMigrator(paths)

	needsTOML := tomlMigrator.NeedsMigration()
	needsV3 := tomlMigrator.NeedsV2ToV3Upgrade()
	needsFlatten := flattenMigrator.NeedsMigration()
	needsFragments := fragmentMigrator.NeedsMigration()
	needsYAMLCleanup := tomlMigrator.NeedsYAMLCleanup()

	if !needsTOML && !needsV3 && !needsFlatten && !needsFragments && !needsYAMLCleanup {
		fmt.Println("No migrations needed - everything is up to date.")
		return nil
	}

	if migrateDryRun {
		fmt.Println("Migrations that would run:")
		if needsTOML {
			fmt.Println("  - Profile manifest YAML → TOML migration")
		}
		if needsV3 {
			fmt.Println("  - Profile manifest v2 → v3 upgrade")
		}
		if needsFlatten {
			fmt.Println("  - Flatten engine/context references into profile hub items")
		}
		if needsFragments {
			fmt.Println("  - Convert setting-fragments to settings template")
		}
		if needsYAMLCleanup {
			fmt.Println("  - Clean up stale profile.yaml files")
		}
		fmt.Println()
		fmt.Println("Run without --dry-run to apply migrations.")
		return nil
	}

	// Run YAML to TOML profile manifest migration
	if needsTOML {
		fmt.Println("Migrating profile manifests from YAML to TOML...")
		migrated, err := tomlMigrator.MigrateProfiles()
		if err != nil {
			return fmt.Errorf("TOML migration failed: %w", err)
		}
		if len(migrated) > 0 {
			fmt.Printf("  Migrated %d profile(s): %v\n", len(migrated), migrated)
		}
	}

	// Upgrade v2 manifests to v3
	if needsV3 {
		fmt.Println("Upgrading profile manifests to v3...")
		upgraded, err := tomlMigrator.UpgradeV2ToV3()
		if err != nil {
			return fmt.Errorf("v3 upgrade failed: %w", err)
		}
		if len(upgraded) > 0 {
			fmt.Printf("  Upgraded %d profile(s): %v\n", len(upgraded), upgraded)
		}
	}

	// Flatten engine/context references into profile hub items
	if needsFlatten {
		fmt.Println("Flattening engine/context references into profiles...")
		count, err := flattenMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("flatten migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Flattened %d profile(s)\n", count)
		}
	}

	// Convert setting-fragments to settings template
	if needsFragments {
		fmt.Println("Converting setting-fragments to settings template...")
		count, err := fragmentMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("fragment migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Merged %d fragment(s) into settings template\n", count)
		}
	}

	// Clean up stale profile.yaml files
	if needsYAMLCleanup {
		fmt.Println("Cleaning up stale profile.yaml files...")
		cleaned, err := tomlMigrator.CleanupYAML()
		if err != nil {
			return fmt.Errorf("YAML cleanup failed: %w", err)
		}
		if len(cleaned) > 0 {
			fmt.Printf("  Cleaned %d profile(s): %v\n", len(cleaned), cleaned)
		}
	}

	fmt.Println()
	fmt.Println("Migrations complete!")
	return nil
}
