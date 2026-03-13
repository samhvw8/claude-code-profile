package profile

import (
	"github.com/samhoang/ccp/internal/config"
)

// ResolveManifest merges engine + context + profile into a flat Manifest.
// The resolution order (lowest to highest priority):
//  1. Engine hub items + data config
//  2. Context hub items
//  3. Profile's own hub items
//
// All lists are union-merged (deduplicated).
// Profile's data config overrides engine's if any field differs from defaults.
func ResolveManifest(m *Manifest, paths *config.Paths) (*Manifest, error) {
	if !m.UsesComposition() {
		return m, nil
	}

	resolved := &Manifest{
		Version:          m.Version,
		Name:             m.Name,
		Description:      m.Description,
		Engine:           m.Engine,
		Context:          m.Context,
		SettingsTemplate: m.SettingsTemplate,
		Created:          m.Created,
		Updated:          m.Updated,
		Data:             m.Data,
		Hooks:            m.Hooks,
	}

	// Start with empty hub
	var allSkills, allAgents, allHooks, allRules, allCommands, allFragments []string

	// Layer 1: Engine
	if m.Engine != "" {
		engineMgr := NewEngineManager(paths)
		engine, err := engineMgr.Get(m.Engine)
		if err != nil {
			return nil, err
		}

		allFragments = append(allFragments, engine.Hub.SettingFragments...)
		allHooks = append(allHooks, engine.Hub.Hooks...)

		// Engine's settings-template is the base; profile overrides if set
		if resolved.SettingsTemplate == "" && engine.SettingsTemplate != "" {
			resolved.SettingsTemplate = engine.SettingsTemplate
		}

		// Use engine's data config as base
		resolved.Data = engine.Data
	}

	// Layer 2: Context
	if m.Context != "" {
		ctxMgr := NewContextManager(paths)
		ctx, err := ctxMgr.Get(m.Context)
		if err != nil {
			return nil, err
		}

		allSkills = append(allSkills, ctx.Hub.Skills...)
		allAgents = append(allAgents, ctx.Hub.Agents...)
		allRules = append(allRules, ctx.Hub.Rules...)
		allCommands = append(allCommands, ctx.Hub.Commands...)
		allHooks = append(allHooks, ctx.Hub.Hooks...)
	}

	// Layer 3: Profile's own hub items (overrides)
	allSkills = append(allSkills, m.Hub.Skills...)
	allAgents = append(allAgents, m.Hub.Agents...)
	allHooks = append(allHooks, m.Hub.Hooks...)
	allRules = append(allRules, m.Hub.Rules...)
	allCommands = append(allCommands, m.Hub.Commands...)
	allFragments = append(allFragments, m.Hub.SettingFragments...)

	// Override data config if profile specifies non-default values
	if m.Engine != "" {
		defaults := config.DefaultDataConfig()
		overrideDataField(&resolved.Data.Tasks, m.Data.Tasks, defaults[config.DataTasks])
		overrideDataField(&resolved.Data.Todos, m.Data.Todos, defaults[config.DataTodos])
		overrideDataField(&resolved.Data.PasteCache, m.Data.PasteCache, defaults[config.DataPasteCache])
		overrideDataField(&resolved.Data.History, m.Data.History, defaults[config.DataHistory])
		overrideDataField(&resolved.Data.FileHistory, m.Data.FileHistory, defaults[config.DataFileHistory])
		overrideDataField(&resolved.Data.SessionEnv, m.Data.SessionEnv, defaults[config.DataSessionEnv])
		overrideDataField(&resolved.Data.Projects, m.Data.Projects, defaults[config.DataProjects])
		overrideDataField(&resolved.Data.Plans, m.Data.Plans, defaults[config.DataPlans])
	}

	// Deduplicate all lists
	resolved.Hub = HubLinks{
		Skills:           dedup(allSkills),
		Agents:           dedup(allAgents),
		Hooks:            dedup(allHooks),
		Rules:            dedup(allRules),
		Commands:         dedup(allCommands),
		SettingFragments: dedup(allFragments),
	}

	return resolved, nil
}

// overrideDataField overrides the target only if the profile value differs from the default
func overrideDataField(target *config.ShareMode, profileVal, defaultVal config.ShareMode) {
	if profileVal != defaultVal {
		*target = profileVal
	}
}

// dedup removes duplicate strings while preserving order
func dedup(items []string) []string {
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
