package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

func setupTemplateMigratorTest(t *testing.T) (*config.Paths, string) {
	t.Helper()
	tmpDir := t.TempDir()

	paths := &config.Paths{
		CcpDir:      filepath.Join(tmpDir, ".ccp"),
		ClaudeDir:   filepath.Join(tmpDir, ".claude"),
		HubDir:      filepath.Join(tmpDir, ".ccp", "hub"),
		ProfilesDir: filepath.Join(tmpDir, ".ccp", "profiles"),
		SharedDir:   filepath.Join(tmpDir, ".ccp", "profiles", "shared"),
		EnginesDir:  filepath.Join(tmpDir, ".ccp", "engines"),
		ContextsDir: filepath.Join(tmpDir, ".ccp", "contexts"),
	}

	// Create dirs
	for _, dir := range []string{
		paths.HubDir,
		paths.ProfilesDir,
		paths.SharedDir,
		paths.EnginesDir,
		paths.ContextsDir,
		filepath.Join(paths.HubDir, "setting-fragments"),
		filepath.Join(paths.HubDir, "settings-templates"),
	} {
		os.MkdirAll(dir, 0755)
	}

	return paths, tmpDir
}

func createTestFragment(t *testing.T, hubDir, name, key string, value interface{}) {
	t.Helper()
	fragmentsDir := filepath.Join(hubDir, "setting-fragments")
	os.MkdirAll(fragmentsDir, 0755)

	content := SettingFragment{
		Name:  name,
		Key:   key,
		Value: value,
	}
	data, _ := json.Marshal(map[string]interface{}{
		"name":  content.Name,
		"key":   content.Key,
		"value": content.Value,
	})

	// Write as YAML (the format LoadSettingFragment expects)
	yamlContent := "name: " + name + "\nkey: " + key + "\nvalue: " + valueToYAML(value) + "\n"
	os.WriteFile(filepath.Join(fragmentsDir, name+".yaml"), []byte(yamlContent), 0644)
	_ = data // suppress unused
}

func valueToYAML(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return "true"
	}
}

func createTestProfile(t *testing.T, paths *config.Paths, name string, fragments []string) {
	t.Helper()
	profileDir := paths.ProfileDir(name)
	os.MkdirAll(profileDir, 0755)

	manifest := profile.NewManifest(name, "test profile")
	manifest.Hub.SettingFragments = fragments
	manifest.Save(filepath.Join(profileDir, "profile.toml"))
}

func createTestEngine(t *testing.T, paths *config.Paths, name string, fragments []string) {
	t.Helper()
	engineDir := paths.EngineDir(name)
	os.MkdirAll(engineDir, 0755)

	engine := profile.NewEngine(name, "test engine")
	engine.Hub.SettingFragments = fragments
	engine.Save(engineDir)
}

func TestTemplateMigrator_NeedsMigration(t *testing.T) {
	t.Run("no migration needed when no fragments", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		// Profile with no fragments
		profileDir := paths.ProfileDir("clean")
		os.MkdirAll(profileDir, 0755)
		manifest := profile.NewManifest("clean", "clean profile")
		manifest.Save(filepath.Join(profileDir, "profile.toml"))

		m := NewTemplateMigrator(paths)
		if m.NeedsMigration() {
			t.Error("should not need migration when no fragments exist")
		}
	})

	t.Run("migration needed when profile has fragments", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)
		createTestProfile(t, paths, "with-frags", []string{"permissions", "model"})

		m := NewTemplateMigrator(paths)
		if !m.NeedsMigration() {
			t.Error("should need migration when profile has fragments")
		}
	})

	t.Run("no migration needed when profile already has template", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		profileDir := paths.ProfileDir("with-template")
		os.MkdirAll(profileDir, 0755)
		manifest := profile.NewManifest("with-template", "has template")
		manifest.Hub.SettingFragments = []string{"permissions"}
		manifest.SettingsTemplate = "my-template"
		manifest.Save(filepath.Join(profileDir, "profile.toml"))

		m := NewTemplateMigrator(paths)
		if m.NeedsMigration() {
			t.Error("should not need migration when template already set")
		}
	})

	t.Run("migration needed when engine has fragments", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)
		createTestEngine(t, paths, "opus", []string{"model"})

		m := NewTemplateMigrator(paths)
		if !m.NeedsMigration() {
			t.Error("should need migration when engine has fragments")
		}
	})
}

func TestTemplateMigrator_Migrate(t *testing.T) {
	t.Run("migrates profile fragments to template", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		// Create fragments in hub
		createTestFragment(t, paths.HubDir, "permissions", "permissions", "allow-all")
		createTestFragment(t, paths.HubDir, "model", "model", "claude-opus")

		// Create profile with fragments
		createTestProfile(t, paths, "dev", []string{"permissions", "model"})

		m := NewTemplateMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 migration, got %d", count)
		}

		// Verify template was created
		tmplPath := filepath.Join(paths.HubDir, "settings-templates", "dev", "settings.json")
		if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
			t.Error("template settings.json should have been created")
		}

		// Verify manifest was updated
		manifestPath := filepath.Join(paths.ProfileDir("dev"), "profile.toml")
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			t.Fatalf("failed to load updated manifest: %v", err)
		}
		if manifest.SettingsTemplate != "dev" {
			t.Errorf("SettingsTemplate = %q, want 'dev'", manifest.SettingsTemplate)
		}
		if len(manifest.Hub.SettingFragments) != 0 {
			t.Errorf("SettingFragments should be empty, got %v", manifest.Hub.SettingFragments)
		}
	})

	t.Run("migrates engine fragments to template", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		createTestFragment(t, paths.HubDir, "model", "model", "claude-opus")
		createTestEngine(t, paths, "opus-full", []string{"model"})

		m := NewTemplateMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 migration, got %d", count)
		}

		// Verify template was created
		tmplPath := filepath.Join(paths.HubDir, "settings-templates", "engine-opus-full", "settings.json")
		if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
			t.Error("template should have been created for engine")
		}

		// Verify engine was updated
		engine, err := profile.LoadEngine(paths.EngineDir("opus-full"))
		if err != nil {
			t.Fatalf("failed to load updated engine: %v", err)
		}
		if engine.SettingsTemplate != "engine-opus-full" {
			t.Errorf("SettingsTemplate = %q, want 'engine-opus-full'", engine.SettingsTemplate)
		}
		if len(engine.Hub.SettingFragments) != 0 {
			t.Errorf("SettingFragments should be empty, got %v", engine.Hub.SettingFragments)
		}
	})

	t.Run("skips profiles already with template", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		profileDir := paths.ProfileDir("already-done")
		os.MkdirAll(profileDir, 0755)
		manifest := profile.NewManifest("already-done", "already migrated")
		manifest.Hub.SettingFragments = []string{"permissions"}
		manifest.SettingsTemplate = "existing-template"
		manifest.Save(filepath.Join(profileDir, "profile.toml"))

		m := NewTemplateMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 migrations, got %d", count)
		}
	})

	t.Run("handles unique template name conflict", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		createTestFragment(t, paths.HubDir, "model", "model", "claude-opus")
		createTestProfile(t, paths, "dev", []string{"model"})

		// Pre-create a template named "dev" to force uniqueness
		existingDir := filepath.Join(paths.HubDir, "settings-templates", "dev")
		os.MkdirAll(existingDir, 0755)
		os.WriteFile(filepath.Join(existingDir, "settings.json"), []byte(`{"existing": true}`), 0644)

		m := NewTemplateMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 migration, got %d", count)
		}

		// Should have created dev-2 instead
		manifestPath := filepath.Join(paths.ProfileDir("dev"), "profile.toml")
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			t.Fatalf("failed to load manifest: %v", err)
		}
		if manifest.SettingsTemplate != "dev-2" {
			t.Errorf("SettingsTemplate = %q, want 'dev-2'", manifest.SettingsTemplate)
		}
	})

	t.Run("no error when no profiles or engines exist", func(t *testing.T) {
		paths, _ := setupTemplateMigratorTest(t)

		m := NewTemplateMigrator(paths)
		count, err := m.Migrate()
		if err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 migrations, got %d", count)
		}
	})
}

func TestTemplateMigrator_Idempotent(t *testing.T) {
	paths, _ := setupTemplateMigratorTest(t)

	createTestFragment(t, paths.HubDir, "model", "model", "claude-opus")
	createTestProfile(t, paths, "dev", []string{"model"})

	m := NewTemplateMigrator(paths)

	// First run
	count1, err := m.Migrate()
	if err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	if count1 != 1 {
		t.Errorf("first run: expected 1, got %d", count1)
	}

	// Second run should be a no-op
	count2, err := m.Migrate()
	if err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("second run: expected 0, got %d", count2)
	}

	// NeedsMigration should also return false
	if m.NeedsMigration() {
		t.Error("NeedsMigration should return false after migration")
	}
}
