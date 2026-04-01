package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/hub"
	"github.com/samhoang/ccp/internal/profile"
)

func setupFragmentMigratorTest(t *testing.T) *config.Paths {
	t.Helper()
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      filepath.Join(tmpDir, ".ccp"),
		ClaudeDir:   filepath.Join(tmpDir, ".claude"),
		HubDir:      filepath.Join(tmpDir, ".ccp", "hub"),
		ProfilesDir: filepath.Join(tmpDir, ".ccp", "profiles"),
		SharedDir:   filepath.Join(tmpDir, ".ccp", "profiles", "shared"),
		StoreDir:    filepath.Join(tmpDir, ".ccp", "store"),
	}
	for _, dir := range []string{
		paths.HubDir,
		paths.ProfilesDir,
		paths.SharedDir,
		filepath.Join(paths.HubDir, "settings-templates"),
	} {
		os.MkdirAll(dir, 0755)
	}
	return paths
}

func writeFragment(t *testing.T, hubDir, name, key, value string) {
	t.Helper()
	fragDir := filepath.Join(hubDir, "setting-fragments")
	os.MkdirAll(fragDir, 0755)
	content := "name: " + name + "\nkey: " + key + "\nvalue: " + value + "\n"
	os.WriteFile(filepath.Join(fragDir, name+".yaml"), []byte(content), 0644)
}

func TestFragmentMigrator_NeedsMigration(t *testing.T) {
	t.Run("no fragments dir", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		m := NewFragmentMigrator(paths)
		if m.NeedsMigration() {
			t.Error("should not need migration without fragments dir")
		}
	})

	t.Run("empty fragments dir", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		os.MkdirAll(filepath.Join(paths.HubDir, "setting-fragments"), 0755)
		m := NewFragmentMigrator(paths)
		if m.NeedsMigration() {
			t.Error("should not need migration with empty fragments dir")
		}
	})

	t.Run("has yaml files", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		writeFragment(t, paths.HubDir, "model", "model", "claude-opus")
		m := NewFragmentMigrator(paths)
		if !m.NeedsMigration() {
			t.Error("should need migration with yaml fragments")
		}
	})
}

func TestFragmentMigrator_Migrate(t *testing.T) {
	t.Run("merges fragments into template", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		writeFragment(t, paths.HubDir, "model", "model", "claude-opus")
		writeFragment(t, paths.HubDir, "api-provider", "apiProvider", "anthropic")

		m := NewFragmentMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 fragments merged, got %d", count)
		}

		// Verify template was created
		tmplMgr := hub.NewTemplateManager(paths.HubDir)
		if !tmplMgr.Exists("migrated-fragments") {
			t.Error("template 'migrated-fragments' should exist")
		}

		tmpl, err := tmplMgr.Load("migrated-fragments")
		if err != nil {
			t.Fatalf("Load template failed: %v", err)
		}
		if tmpl.Settings["model"] != "claude-opus" {
			t.Errorf("model = %v, want claude-opus", tmpl.Settings["model"])
		}
		if tmpl.Settings["apiProvider"] != "anthropic" {
			t.Errorf("apiProvider = %v, want anthropic", tmpl.Settings["apiProvider"])
		}

		// Verify fragments dir was removed
		fragDir := filepath.Join(paths.HubDir, "setting-fragments")
		if _, err := os.Stat(fragDir); !os.IsNotExist(err) {
			t.Error("setting-fragments dir should be removed after migration")
		}
	})

	t.Run("sets template on profiles without one", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		writeFragment(t, paths.HubDir, "model", "model", "claude-opus")

		// Create profile without template
		profileDir := paths.ProfileDir("dev")
		os.MkdirAll(profileDir, 0755)
		manifest := profile.NewManifest("dev", "test")
		manifest.Save(filepath.Join(profileDir, "profile.toml"))

		m := NewFragmentMigrator(paths)
		_, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}

		// Verify profile now has template
		updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
		if err != nil {
			t.Fatalf("LoadManifest failed: %v", err)
		}
		if updated.SettingsTemplate != "migrated-fragments" {
			t.Errorf("SettingsTemplate = %q, want 'migrated-fragments'", updated.SettingsTemplate)
		}
	})

	t.Run("skips profiles that already have template", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		writeFragment(t, paths.HubDir, "model", "model", "claude-opus")

		// Create profile WITH template
		profileDir := paths.ProfileDir("existing")
		os.MkdirAll(profileDir, 0755)
		manifest := profile.NewManifest("existing", "test")
		manifest.SettingsTemplate = "my-template"
		manifest.Save(filepath.Join(profileDir, "profile.toml"))

		m := NewFragmentMigrator(paths)
		_, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}

		// Verify profile still has original template
		updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
		if err != nil {
			t.Fatalf("LoadManifest failed: %v", err)
		}
		if updated.SettingsTemplate != "my-template" {
			t.Errorf("SettingsTemplate = %q, want 'my-template'", updated.SettingsTemplate)
		}
	})

	t.Run("no fragments returns 0", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		m := NewFragmentMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("idempotent — no fragments after first run", func(t *testing.T) {
		paths := setupFragmentMigratorTest(t)
		writeFragment(t, paths.HubDir, "model", "model", "claude-opus")

		m := NewFragmentMigrator(paths)
		m.Migrate()

		// Second run
		if m.NeedsMigration() {
			t.Error("should not need migration after first run")
		}
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("second Migrate failed: %v", err)
		}
		if count != 0 {
			t.Errorf("second run: expected 0, got %d", count)
		}
	})
}
