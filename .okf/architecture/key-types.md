---
type: Code Reference
title: Key types and functions
description: The Go types and entry points most changes touch — Paths, Manifest, Source, CcpConfig, Template, and settings generation.
tags: [architecture, go, types]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: code
    resource: "internal/config/paths.go, internal/profile/manifest.go, internal/profile/generator.go, internal/profile/settings_generator.go, internal/source/types.go, internal/config/ccp_config.go, internal/hub/template.go"
    title: Source files the signatures below were checked against
    last_modified: 2026-09-14
---

# Types

Signatures were checked against the code on 2026-09-14.[^code] The old developer reference
listed `GenerateSettings(manifest, hubDir)`; the real signature takes `paths` and `profileDir`.

```go
// internal/config/paths.go
type Paths struct {
    CcpDir          string // ~/.ccp (or $CCP_DIR)
    ClaudeDir       string // ~/.claude or $CLAUDE_CONFIG_DIR (may be project-specific)
    GlobalClaudeDir string // ~/.claude, always global, ignores CLAUDE_CONFIG_DIR
    HubDir          string // ~/.ccp/hub
    ProfilesDir     string // ~/.ccp/profiles
    SharedDir       string // ~/.ccp/profiles/shared
    StoreDir        string // ~/.ccp/store
}

type HubItemType string     // skills, agents, hooks, rules, commands; bundles is composite
type DataItemType string    // tasks, todos, history, …
type PluginStoreItem string // marketplaces, cache, known_marketplaces.json

// internal/profile/manifest.go
type Manifest struct {
    Version          int       // 3 = current, 2 = TOML, 1 = YAML
    Name, Description string
    Engine, Context  string    // Deprecated: flattened by migration (v0.28)
    SettingsTemplate string    // toml: settings-template
    Created, Updated time.Time
    Hub              HubLinks  // skills, agents, hooks, rules, commands, bundles
    Hooks            []config.HookConfig
}

// internal/source/types.go
type Source struct {
    Registry, Provider string    // e.g. "github", "git"
    URL, Path          string    // clone URL, local path under sources/
    Ref, Commit        string    // git ref, pinned commit
    Checksum           string    // for http downloads
    Updated            time.Time
    Installed          []string  // skills/foo, agents/bar
}

// internal/config/ccp_config.go
type CcpConfig struct {
    DefaultRegistry string         // "skills.sh" (default) or "github"
    GitHub          GitHubConfig   // topics, per_page
    SkillsSh        SkillsShConfig // base_url, limit
}

// internal/hub/template.go
type Template struct {
    Name     string                 // directory name
    Settings map[string]interface{} // raw settings content, hooks excluded
}
```

# Settings generation

One function, not a pipeline ([design principles](/engineering/design-principles.md)):

```go
// internal/profile/generator.go
func GenerateSettings(manifest *Manifest, paths *config.Paths, profileDir string) (map[string]interface{}, error)
func DiffSettings(base, current map[string]interface{}) map[string]interface{}
func UpdateFragment(paths *config.Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error)
func PreviewFragment(paths *config.Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error)

// internal/profile/settings_generator.go
func GenerateSettingsHooks(paths *config.Paths, profileDir string, manifest *Manifest) (map[config.HookType][]config.SettingsHookEntry, error)
func RegenerateSettings(paths *config.Paths, profileDir string, manifest *Manifest) error
```

Load template → overlay hooks → return the settings map. See
[settings templates](/reference/settings-templates.md) and [hooks format](/reference/hooks-format.md).

[^code]: Source files the signatures below were checked against
