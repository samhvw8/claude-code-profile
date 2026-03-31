package migration

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/samhoang/ccp/internal/config"
	"github.com/samhoang/ccp/internal/profile"
)

// legacyEngine mirrors the old Engine struct for reading engine.toml files
type legacyEngine struct {
	Name             string         `toml:"name"`
	Description      string         `toml:"description,omitempty"`
	SettingsTemplate string         `toml:"settings-template,omitempty"`
	Hub              legacyEngineHub `toml:"hub"`
}

type legacyEngineHub struct {
	Hooks []string `toml:"hooks,omitempty"`
}

// legacyContext mirrors the old Context struct for reading context.toml files
type legacyContext struct {
	Name        string          `toml:"name"`
	Description string          `toml:"description,omitempty"`
	Hub         legacyContextHub `toml:"hub"`
}

type legacyContextHub struct {
	Skills   []string `toml:"skills,omitempty"`
	Agents   []string `toml:"agents,omitempty"`
	Rules    []string `toml:"rules,omitempty"`
	Commands []string `toml:"commands,omitempty"`
	Hooks    []string `toml:"hooks,omitempty"`
}

// FlattenMigrator flattens engine/context references into inline profile hub items
type FlattenMigrator struct {
	paths *config.Paths
}

// NewFlattenMigrator creates a new flatten migrator
func NewFlattenMigrator(paths *config.Paths) *FlattenMigrator {
	return &FlattenMigrator{paths: paths}
}

// NeedsMigration checks if any profiles still reference an engine or context
func (m *FlattenMigrator) NeedsMigration() bool {
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		if manifest.Engine != "" || manifest.Context != "" {
			return true
		}
	}
	return false
}

// Migrate flattens engine/context into each profile's hub items
func (m *FlattenMigrator) Migrate() (int, error) {
	entries, err := os.ReadDir(m.paths.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		profileDir := m.paths.ProfileDir(entry.Name())
		manifestPath := profile.ManifestPath(profileDir)
		manifest, err := profile.LoadManifest(manifestPath)
		if err != nil {
			continue
		}

		if manifest.Engine == "" && manifest.Context == "" {
			continue
		}

		if err := m.flattenProfile(manifest, manifestPath); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

func (m *FlattenMigrator) flattenProfile(manifest *profile.Manifest, manifestPath string) error {
	var allSkills, allAgents, allHooks, allRules, allCommands []string

	// Layer 1: Engine
	if manifest.Engine != "" {
		engine, err := m.loadEngine(manifest.Engine)
		if err == nil {
			allHooks = append(allHooks, engine.Hub.Hooks...)

			// Engine's settings-template is the base; profile overrides if set
			if manifest.SettingsTemplate == "" && engine.SettingsTemplate != "" {
				manifest.SettingsTemplate = engine.SettingsTemplate
			}
		}
		// If engine file doesn't exist, skip silently (it may have been deleted)
	}

	// Layer 2: Context
	if manifest.Context != "" {
		ctx, err := m.loadContext(manifest.Context)
		if err == nil {
			allSkills = append(allSkills, ctx.Hub.Skills...)
			allAgents = append(allAgents, ctx.Hub.Agents...)
			allRules = append(allRules, ctx.Hub.Rules...)
			allCommands = append(allCommands, ctx.Hub.Commands...)
			allHooks = append(allHooks, ctx.Hub.Hooks...)
		}
		// If context file doesn't exist, skip silently
	}

	// Layer 3: Profile's own hub items (highest priority)
	allSkills = append(allSkills, manifest.Hub.Skills...)
	allAgents = append(allAgents, manifest.Hub.Agents...)
	allHooks = append(allHooks, manifest.Hub.Hooks...)
	allRules = append(allRules, manifest.Hub.Rules...)
	allCommands = append(allCommands, manifest.Hub.Commands...)

	// Deduplicate and assign
	manifest.Hub.Skills = dedupStrings(allSkills)
	manifest.Hub.Agents = dedupStrings(allAgents)
	manifest.Hub.Hooks = dedupStrings(allHooks)
	manifest.Hub.Rules = dedupStrings(allRules)
	manifest.Hub.Commands = dedupStrings(allCommands)

	// Clear composition fields
	manifest.Engine = ""
	manifest.Context = ""

	// Save
	return manifest.Save(manifestPath)
}

func (m *FlattenMigrator) loadEngine(name string) (*legacyEngine, error) {
	enginesDir := filepath.Join(m.paths.CcpDir, "engines")
	path := filepath.Join(enginesDir, name, "engine.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e legacyEngine
	if err := toml.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (m *FlattenMigrator) loadContext(name string) (*legacyContext, error) {
	contextsDir := filepath.Join(m.paths.CcpDir, "contexts")
	path := filepath.Join(contextsDir, name, "context.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c legacyContext
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// dedupStrings removes duplicate strings while preserving order
func dedupStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
