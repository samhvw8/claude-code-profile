package migration

import (
	"os"
	"path/filepath"
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
