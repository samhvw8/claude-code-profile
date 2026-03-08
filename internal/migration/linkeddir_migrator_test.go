package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samhoang/ccp/internal/config"
)

func TestLinkedDirMigrator_NeedsMigration(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		HubDir:      filepath.Join(tmpDir, "hub"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	os.MkdirAll(filepath.Join(paths.HubDir, "rules"), 0755)

	profileDir := filepath.Join(paths.ProfilesDir, "test-profile")
	os.MkdirAll(profileDir, 0755)

	// No CLAUDE.md — no migration needed
	migrator := NewLinkedDirMigrator(paths)
	if migrator.NeedsMigration() {
		t.Error("expected no migration needed without CLAUDE.md")
	}

	// Create CLAUDE.md with @import
	os.WriteFile(filepath.Join(profileDir, "CLAUDE.md"), []byte("@principles/se.md\n"), 0644)

	// Create the referenced directory
	os.MkdirAll(filepath.Join(profileDir, "principles"), 0755)
	os.WriteFile(filepath.Join(profileDir, "principles", "se.md"), []byte("# SE"), 0644)

	// Create a manifest without linked-dirs
	manifest := `version = 3
name = "test-profile"
created = 2024-01-15T10:30:00Z
updated = 2024-01-15T10:30:00Z
`
	os.WriteFile(filepath.Join(profileDir, "profile.toml"), []byte(manifest), 0644)

	// Create rules dir in profile
	os.MkdirAll(filepath.Join(profileDir, "rules"), 0755)

	if !migrator.NeedsMigration() {
		t.Error("expected migration needed when CLAUDE.md has @imports and manifest lacks linked-dirs")
	}
}

func TestLinkedDirMigrator_Migrate(t *testing.T) {
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      tmpDir,
		HubDir:      filepath.Join(tmpDir, "hub"),
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	os.MkdirAll(filepath.Join(paths.HubDir, "rules"), 0755)

	profileDir := filepath.Join(paths.ProfilesDir, "my-profile")
	os.MkdirAll(profileDir, 0755)
	os.MkdirAll(filepath.Join(profileDir, "rules"), 0755)

	// Create CLAUDE.md with @imports
	os.WriteFile(filepath.Join(profileDir, "CLAUDE.md"),
		[]byte("@principles/delegation.md\n@principles/se.md\n@references/api.md\n"), 0644)

	// Create the referenced directories
	os.MkdirAll(filepath.Join(profileDir, "principles"), 0755)
	os.WriteFile(filepath.Join(profileDir, "principles", "delegation.md"), []byte("# Del"), 0644)
	os.WriteFile(filepath.Join(profileDir, "principles", "se.md"), []byte("# SE"), 0644)
	os.MkdirAll(filepath.Join(profileDir, "references"), 0755)
	os.WriteFile(filepath.Join(profileDir, "references", "api.md"), []byte("# API"), 0644)

	// Create manifest without linked-dirs
	manifest := `version = 3
name = "my-profile"
created = 2024-01-15T10:30:00Z
updated = 2024-01-15T10:30:00Z
`
	os.WriteFile(filepath.Join(profileDir, "profile.toml"), []byte(manifest), 0644)

	migrator := NewLinkedDirMigrator(paths)

	count, err := migrator.Migrate()
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 profile updated, got %d", count)
	}

	// Verify dirs moved to hub/rules/
	hubPrinciples := filepath.Join(paths.HubDir, "rules", "principles")
	if _, err := os.Stat(filepath.Join(hubPrinciples, "se.md")); os.IsNotExist(err) {
		t.Error("expected principles/se.md in hub/rules/")
	}
	hubReferences := filepath.Join(paths.HubDir, "rules", "references")
	if _, err := os.Stat(filepath.Join(hubReferences, "api.md")); os.IsNotExist(err) {
		t.Error("expected references/api.md in hub/rules/")
	}

	// Verify root-level symlinks created
	principlesLink := filepath.Join(profileDir, "principles")
	info, err := os.Lstat(principlesLink)
	if err != nil {
		t.Fatal("expected principles symlink to exist")
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected principles to be a symlink")
	}

	// Verify rules/ symlinks created
	rulesLink := filepath.Join(profileDir, "rules", "principles")
	if _, err := os.Lstat(rulesLink); os.IsNotExist(err) {
		t.Error("expected rules/principles symlink")
	}

	// Verify manifest updated
	data, err := os.ReadFile(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "linked-dirs") {
		t.Error("expected linked-dirs in manifest")
	}
	if !strings.Contains(content, "principles") {
		t.Error("expected 'principles' tracked")
	}

	// Verify content still accessible through symlink
	seContent, err := os.ReadFile(filepath.Join(profileDir, "principles", "se.md"))
	if err != nil {
		t.Fatal("failed to read through symlink")
	}
	if string(seContent) != "# SE" {
		t.Errorf("unexpected content: %q", string(seContent))
	}

	// No longer needs migration
	if migrator.NeedsMigration() {
		t.Error("expected no migration needed after migrate")
	}

	// Idempotent
	count, err = migrator.Migrate()
	if err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 on second run, got %d", count)
	}
}
