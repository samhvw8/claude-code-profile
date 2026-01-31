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
2. Migrates source.yaml files to the unified registry.toml
3. Converts absolute symlinks to relative (for cross-computer portability)

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
	sourceMigrator := migration.NewSourceMigrator(paths)
	symlinkMigrator := migration.NewSymlinkMigrator(paths)

	needsTOML := tomlMigrator.NeedsMigration()
	needsSource := sourceMigrator.NeedsMigration()
	needsSymlink := symlinkMigrator.NeedsMigration()

	if !needsTOML && !needsSource && !needsSymlink {
		fmt.Println("No migrations needed - everything is up to date.")
		return nil
	}

	if migrateDryRun {
		fmt.Println("Migrations that would run:")
		if needsTOML {
			fmt.Println("  - Profile manifest YAML → TOML migration")
		}
		if needsSource {
			fmt.Println("  - source.yaml → registry.toml migration")
		}
		if needsSymlink {
			fmt.Println("  - Absolute → relative symlink migration")
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

	// Run source.yaml to registry.toml migration
	if needsSource {
		fmt.Println("Migrating source.yaml files to registry.toml...")
		count, err := sourceMigrator.MigrateSourceYAML()
		if err != nil {
			return fmt.Errorf("source migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Migrated %d source(s) to registry\n", count)
		}
	}

	// Run absolute to relative symlink migration
	if needsSymlink {
		fmt.Println("Converting absolute symlinks to relative...")
		count, err := symlinkMigrator.MigrateSymlinks()
		if err != nil {
			return fmt.Errorf("symlink migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Converted %d symlink(s) to relative paths\n", count)
		}
	}

	fmt.Println()
	fmt.Println("Migrations complete!")
	return nil
}
