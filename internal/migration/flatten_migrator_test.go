package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

func TestFlattenMigrator_NeedsMigration_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	m := NewFlattenMigrator(paths)
	if m.NeedsMigration() {
		t.Error("NeedsMigration() = true, want false when no profiles dir")
	}
}

func TestFlattenMigrator_NeedsMigration_NoComposition(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create a profile without engine/context
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "no composition")
	manifest.Hub.Skills = []string{"coding"}
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	m := NewFlattenMigrator(paths)
	if m.NeedsMigration() {
		t.Error("NeedsMigration() = true, want false when no composition")
	}
}

func TestFlattenMigrator_NeedsMigration_WithEngine(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create a profile with engine reference
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "has engine")
	manifest.Engine = "my-engine"
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	m := NewFlattenMigrator(paths)
	if !m.NeedsMigration() {
		t.Error("NeedsMigration() = false, want true when engine is set")
	}
}

func TestFlattenMigrator_Migrate_WithEngineAndContext(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create engine
	engineDir := filepath.Join(tmpDir, "engines", "test-engine")
	os.MkdirAll(engineDir, 0755)
	engine := legacyEngine{
		Name:             "test-engine",
		SettingsTemplate: "opus-full",
		Hub:              legacyEngineHub{Hooks: []string{"session-manager"}},
	}
	engineData, _ := toml.Marshal(engine)
	os.WriteFile(filepath.Join(engineDir, "engine.toml"), engineData, 0644)

	// Create context
	contextDir := filepath.Join(tmpDir, "contexts", "test-ctx")
	os.MkdirAll(contextDir, 0755)
	ctx := legacyContext{
		Name: "test-ctx",
		Hub: legacyContextHub{
			Skills: []string{"coding", "debugging"},
			Agents: []string{"reviewer"},
			Hooks:  []string{"prompt-loader"},
		},
	}
	ctxData, _ := toml.Marshal(ctx)
	os.WriteFile(filepath.Join(contextDir, "context.toml"), ctxData, 0644)

	// Create profile with engine + context + own items
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "composed profile")
	manifest.Engine = "test-engine"
	manifest.Context = "test-ctx"
	manifest.Hub.Skills = []string{"docker"}
	manifest.Hub.Hooks = []string{"pre-commit"}
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	// Run migration
	m := NewFlattenMigrator(paths)
	count, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if count != 1 {
		t.Errorf("Migrate() count = %d, want 1", count)
	}

	// Reload and verify
	updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	// Engine and context should be cleared
	if updated.Engine != "" {
		t.Errorf("Engine = %q, want empty", updated.Engine)
	}
	if updated.Context != "" {
		t.Errorf("Context = %q, want empty", updated.Context)
	}

	// Settings template should come from engine
	if updated.SettingsTemplate != "opus-full" {
		t.Errorf("SettingsTemplate = %q, want %q", updated.SettingsTemplate, "opus-full")
	}

	// Skills: context (coding, debugging) + profile (docker) = 3
	if len(updated.Hub.Skills) != 3 {
		t.Errorf("Skills = %v, want 3 items", updated.Hub.Skills)
	}

	// Hooks: engine (session-manager) + context (prompt-loader) + profile (pre-commit) = 3
	if len(updated.Hub.Hooks) != 3 {
		t.Errorf("Hooks = %v, want 3 items", updated.Hub.Hooks)
	}

	// Agents from context
	if len(updated.Hub.Agents) != 1 || updated.Hub.Agents[0] != "reviewer" {
		t.Errorf("Agents = %v, want [reviewer]", updated.Hub.Agents)
	}

	// Should no longer need migration
	if m.NeedsMigration() {
		t.Error("NeedsMigration() = true after migration, want false")
	}
}

func TestFlattenMigrator_Migrate_Dedup(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create engine with a hook
	engineDir := filepath.Join(tmpDir, "engines", "eng")
	os.MkdirAll(engineDir, 0755)
	engine := legacyEngine{
		Name: "eng",
		Hub:  legacyEngineHub{Hooks: []string{"shared-hook"}},
	}
	engineData, _ := toml.Marshal(engine)
	os.WriteFile(filepath.Join(engineDir, "engine.toml"), engineData, 0644)

	// Create context with same hook
	contextDir := filepath.Join(tmpDir, "contexts", "ctx")
	os.MkdirAll(contextDir, 0755)
	ctx := legacyContext{
		Name: "ctx",
		Hub:  legacyContextHub{Hooks: []string{"shared-hook"}},
	}
	ctxData, _ := toml.Marshal(ctx)
	os.WriteFile(filepath.Join(contextDir, "context.toml"), ctxData, 0644)

	// Profile also has the same hook
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "")
	manifest.Engine = "eng"
	manifest.Context = "ctx"
	manifest.Hub.Hooks = []string{"shared-hook"}
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	m := NewFlattenMigrator(paths)
	count, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if count != 1 {
		t.Errorf("Migrate() count = %d, want 1", count)
	}

	// Reload and verify dedup
	updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	if len(updated.Hub.Hooks) != 1 {
		t.Errorf("Hooks = %v, want 1 item (deduplicated)", updated.Hub.Hooks)
	}
}

func TestFlattenMigrator_Migrate_MissingEngine(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Profile references engine that doesn't exist
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "")
	manifest.Engine = "nonexistent"
	manifest.Hub.Skills = []string{"coding"}
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	m := NewFlattenMigrator(paths)
	count, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error: %v (should handle missing engine gracefully)", err)
	}
	if count != 1 {
		t.Errorf("Migrate() count = %d, want 1", count)
	}

	// Verify profile was still flattened
	updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if updated.Engine != "" {
		t.Errorf("Engine = %q, want empty", updated.Engine)
	}
	if len(updated.Hub.Skills) != 1 {
		t.Errorf("Skills = %v, want 1", updated.Hub.Skills)
	}
}

func TestFlattenMigrator_Migrate_ProfileTemplateOverridesEngine(t *testing.T) {
	tmpDir := t.TempDir()
	paths := &config.Paths{
		CcpDir:      tmpDir,
		ProfilesDir: filepath.Join(tmpDir, "profiles"),
	}

	// Create engine with a settings template
	engineDir := filepath.Join(tmpDir, "engines", "eng")
	os.MkdirAll(engineDir, 0755)
	engine := legacyEngine{
		Name:             "eng",
		SettingsTemplate: "engine-template",
	}
	engineData, _ := toml.Marshal(engine)
	os.WriteFile(filepath.Join(engineDir, "engine.toml"), engineData, 0644)

	// Profile has its own template (should win)
	profileDir := filepath.Join(paths.ProfilesDir, "test")
	os.MkdirAll(profileDir, 0755)
	manifest := profile.NewManifest("test", "")
	manifest.Engine = "eng"
	manifest.SettingsTemplate = "profile-template"
	manifest.Save(filepath.Join(profileDir, "profile.toml"))

	m := NewFlattenMigrator(paths)
	_, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	updated, err := profile.LoadManifest(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	// Profile's template should be preserved (not overwritten by engine's)
	if updated.SettingsTemplate != "profile-template" {
		t.Errorf("SettingsTemplate = %q, want %q", updated.SettingsTemplate, "profile-template")
	}
}

func TestDedupStrings(t *testing.T) {
	tests := []struct {
		input []string
		want  int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"a", "b", "c"}, 3},
		{[]string{"a", "b", "a", "c", "b"}, 3},
		{[]string{"x", "x", "x"}, 1},
	}

	for _, tt := range tests {
		result := dedupStrings(tt.input)
		if len(result) != tt.want {
			t.Errorf("dedupStrings(%v) = %v, want len %d", tt.input, result, tt.want)
		}
	}
}
