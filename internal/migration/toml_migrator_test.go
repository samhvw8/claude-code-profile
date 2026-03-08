package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestTOMLMigrator_MigrateProfiles(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create profiles directory
	if err := os.MkdirAll(paths.ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a profile with YAML manifest
	profileDir := filepath.Join(paths.ProfilesDir, "test-profile")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: test-profile
description: Test profile for migration
created: 2024-01-15T10:30:00Z
updated: 2024-01-15T10:30:00Z
hub:
  skills:
    - git-workflow
    - planning
  agents:
    - researcher
data:
  tasks: shared
  todos: shared
  paste-cache: shared
  history: isolated
  file-history: isolated
  session-env: isolated
  projects: shared
  plans: isolated
`
	yamlPath := filepath.Join(profileDir, "profile.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run migration
	migrator := NewTOMLMigrator(paths)

	if !migrator.NeedsMigration() {
		t.Error("expected NeedsMigration to return true")
	}

	migrated, err := migrator.MigrateProfiles()
	if err != nil {
		t.Fatalf("MigrateProfiles failed: %v", err)
	}

	if len(migrated) != 1 || migrated[0] != "test-profile" {
		t.Errorf("expected [test-profile], got %v", migrated)
	}

	// Check TOML file exists
	tomlPath := filepath.Join(profileDir, "profile.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		t.Error("expected profile.toml to exist")
	}

	// Check YAML was backed up
	backupPath := yamlPath + ".bak"
	if _, err := os.Stat(backupPath); err != nil {
		t.Error("expected profile.yaml.bak to exist")
	}

	// Check no longer needs migration
	if migrator.NeedsMigration() {
		t.Error("expected NeedsMigration to return false after migration")
	}
}

func TestTOMLMigrator_UpgradeV2ToV3(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	profileDir := filepath.Join(paths.ProfilesDir, "v2-profile")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a v2 TOML manifest
	v2Content := `version = 2
name = "v2-profile"
description = "A v2 profile"
created = 2024-01-15T10:30:00Z
updated = 2024-01-15T10:30:00Z

[hub]
skills = ["coding"]

[data]
tasks = "shared"
todos = "shared"
paste-cache = "shared"
history = "isolated"
file-history = "isolated"
session-env = "isolated"
projects = "shared"
plans = "isolated"
`
	tomlPath := filepath.Join(profileDir, "profile.toml")
	if err := os.WriteFile(tomlPath, []byte(v2Content), 0644); err != nil {
		t.Fatal(err)
	}

	migrator := NewTOMLMigrator(paths)

	if !migrator.NeedsV2ToV3Upgrade() {
		t.Error("expected NeedsV2ToV3Upgrade to return true")
	}

	upgraded, err := migrator.UpgradeV2ToV3()
	if err != nil {
		t.Fatalf("UpgradeV2ToV3 failed: %v", err)
	}

	if len(upgraded) != 1 || upgraded[0] != "v2-profile" {
		t.Errorf("expected [v2-profile], got %v", upgraded)
	}

	// Verify version is now 3
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version = 3") {
		t.Error("expected version = 3 in upgraded file")
	}

	// Verify hub items preserved
	if !strings.Contains(string(data), "coding") {
		t.Error("expected skills preserved in upgraded file")
	}

	// No longer needs upgrade
	if migrator.NeedsV2ToV3Upgrade() {
		t.Error("expected NeedsV2ToV3Upgrade to return false after upgrade")
	}
}

func TestStructureMigrator(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		EnginesDir:  filepath.Join(tmpDir, "engines"),
		ContextsDir: filepath.Join(tmpDir, "contexts"),
	}

	migrator := NewStructureMigrator(paths)

	if !migrator.NeedsMigration() {
		t.Error("expected NeedsMigration to return true")
	}

	count, err := migrator.Migrate()
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 dirs created, got %d", count)
	}

	// Verify dirs exist
	if _, err := os.Stat(paths.EnginesDir); os.IsNotExist(err) {
		t.Error("engines dir should exist")
	}
	if _, err := os.Stat(paths.ContextsDir); os.IsNotExist(err) {
		t.Error("contexts dir should exist")
	}

	// Idempotent
	if migrator.NeedsMigration() {
		t.Error("expected NeedsMigration to return false after migration")
	}

	count, err = migrator.Migrate()
	if err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 dirs created on second run, got %d", count)
	}
}

func TestTOMLMigrator_SkipsAlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create profile with both YAML and TOML
	profileDir := filepath.Join(paths.ProfilesDir, "already-migrated")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create both files
	os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte("name: test"), 0644)
	os.WriteFile(filepath.Join(profileDir, "profile.toml"), []byte("version = 2\nname = \"test\""), 0644)

	migrator := NewTOMLMigrator(paths)

	if migrator.NeedsMigration() {
		t.Error("expected NeedsMigration to return false when TOML exists")
	}

	migrated, err := migrator.MigrateProfiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(migrated) != 0 {
		t.Errorf("expected no migrations, got %v", migrated)
	}
}
