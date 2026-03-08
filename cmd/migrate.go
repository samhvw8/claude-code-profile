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
2. Upgrades v2 manifests to v3 (adds engine/context support)
3. Creates engines/ and contexts/ directories
4. Migrates source.yaml files to ccp.toml [sources] section
5. Migrates registry.toml to ccp.toml [sources] section
6. Converts absolute symlinks to relative (for cross-computer portability)
7. Converts hook.yaml to hooks.json (official Claude Code format)
8. Moves plugin caches to shared store (marketplaces, known_marketplaces.json)

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
	structureMigrator := migration.NewStructureMigrator(paths)
	sourceMigrator := migration.NewSourceMigrator(paths)
	registryMigrator := migration.NewRegistryMigrator(paths)
	symlinkMigrator := migration.NewSymlinkMigrator(paths)
	hookFormatMigrator := migration.NewHookFormatMigrator(paths)
	pluginStoreMigrator := migration.NewPluginStoreMigrator(paths)
	linkedDirMigrator := migration.NewLinkedDirMigrator(paths)

	needsTOML := tomlMigrator.NeedsMigration()
	needsV3 := tomlMigrator.NeedsV2ToV3Upgrade()
	needsStructure := structureMigrator.NeedsMigration()
	needsSource := sourceMigrator.NeedsMigration()
	needsRegistry := registryMigrator.NeedsMigration()
	needsSymlink := symlinkMigrator.NeedsMigration()
	needsHookFormat := hookFormatMigrator.NeedsMigration()
	needsPluginStore := pluginStoreMigrator.NeedsMigration()
	needsLinkedDirs := linkedDirMigrator.NeedsMigration()

	if !needsTOML && !needsV3 && !needsStructure && !needsSource && !needsRegistry && !needsSymlink && !needsHookFormat && !needsPluginStore && !needsLinkedDirs {
		fmt.Println("No migrations needed - everything is up to date.")
		return nil
	}

	if migrateDryRun {
		fmt.Println("Migrations that would run:")
		if needsTOML {
			fmt.Println("  - Profile manifest YAML → TOML migration")
		}
		if needsV3 {
			fmt.Println("  - Profile manifest v2 → v3 upgrade (engine/context support)")
		}
		if needsStructure {
			fmt.Println("  - Create engines/ and contexts/ directories")
		}
		if needsSource {
			fmt.Println("  - source.yaml → ccp.toml migration")
		}
		if needsRegistry {
			fmt.Println("  - registry.toml → ccp.toml migration")
		}
		if needsSymlink {
			fmt.Println("  - Absolute → relative symlink migration")
		}
		if needsHookFormat {
			fmt.Println("  - hook.yaml → hooks.json format migration")
		}
		if needsPluginStore {
			fmt.Println("  - Plugin cache → shared store migration")
		}
		if needsLinkedDirs {
			fmt.Println("  - Track CLAUDE.md @import linked directories")
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

	// Create engines/ and contexts/ directories
	if needsStructure {
		fmt.Println("Creating engines/ and contexts/ directories...")
		count, err := structureMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("structure migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Created %d directory(ies)\n", count)
		}
	}

	// Run source.yaml to ccp.toml migration
	if needsSource {
		fmt.Println("Migrating source.yaml files to ccp.toml...")
		count, err := sourceMigrator.MigrateSourceYAML()
		if err != nil {
			return fmt.Errorf("source migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Migrated %d source(s) to ccp.toml\n", count)
		}
	}

	// Run registry.toml to ccp.toml migration
	if needsRegistry {
		fmt.Println("Migrating registry.toml to ccp.toml...")
		count, err := registryMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("registry migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Migrated %d source(s) to ccp.toml\n", count)
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

	// Run hook.yaml to hooks.json format migration
	if needsHookFormat {
		fmt.Println("Converting hook.yaml to hooks.json format...")
		count, err := hookFormatMigrator.MigrateHookFormats()
		if err != nil {
			return fmt.Errorf("hook format migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Converted %d hook(s) to hooks.json format\n", count)
		}
	}

	// Run plugin store migration
	if needsPluginStore {
		fmt.Println("Moving plugin caches to shared store...")
		count, err := pluginStoreMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("plugin store migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Moved %d plugin item(s) to shared store\n", count)
		}
	}

	// Track CLAUDE.md @import linked directories
	if needsLinkedDirs {
		fmt.Println("Tracking CLAUDE.md @import linked directories...")
		count, err := linkedDirMigrator.Migrate()
		if err != nil {
			return fmt.Errorf("linked dir migration failed: %w", err)
		}
		if count > 0 {
			fmt.Printf("  Updated %d profile(s) with linked directory tracking\n", count)
		}
	}

	fmt.Println()
	fmt.Println("Migrations complete!")
	return nil
}
