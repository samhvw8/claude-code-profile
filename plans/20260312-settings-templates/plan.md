# Replace Setting Fragments with Settings Templates

**Created:** 2026-03-12
**Status:** draft
**Blast radius:** 36 files reference `setting-fragments`

## Summary

Replace granular setting-fragment YAML files (one key per file) with complete `settings.json` templates. Templates are full settings files that profiles select from. Hooks remain separately managed.

## Phases

### Phase 1: New types & storage (`internal/hub/template.go`)

**Goal:** Settings template CRUD without touching existing fragment code.

Files to create:
- `internal/hub/template.go` — `Template` struct, `TemplateManager` (Load, Save, List, Delete, Exists)

Files to modify:
- `internal/config/paths.go` — Add `HubSettingsTemplates HubItemType = "settings-templates"`, add to `AllHubItemTypes()`, update `HubItemPath()` to return `settings.json` inside dir
- `internal/config/paths_test.go` — Update count in `TestAllHubItemTypes`

Storage format:
```
~/.ccp/hub/settings-templates/<name>/
└── settings.json    # Complete settings (hooks excluded — managed separately)
```

Template struct:
```go
type Template struct {
    Name     string                 // Directory name
    Settings map[string]interface{} // Raw settings.json content
}
```

---

### Phase 2: Update manifest & engine types

**Goal:** Replace `SettingFragments []string` with `SettingsTemplate string`.

Files to modify:
- `internal/profile/manifest.go`
  - Add `SettingsTemplate string` field to `Manifest` (toml:"settings-template")
  - Remove `SettingFragments` from `HubLinks`
  - Update `GetHubItems()` / `SetHubItems()` / `AddHubItem()` / `RemoveHubItem()` — remove fragment cases
- `internal/profile/engine.go`
  - Add `SettingsTemplate string` to `Engine` (toml:"settings-template")
  - Remove `SettingFragments` from `EngineHub`
- `internal/profile/resolver.go`
  - Resolution: engine.SettingsTemplate → profile.SettingsTemplate (profile wins if set)
  - Remove `allFragments` accumulation logic, add template resolution
- `internal/profile/context.go` — No change (contexts don't have settings)

---

### Phase 3: Update settings generation

**Goal:** Generate settings.json from template + hooks instead of fragments + hooks.

Files to modify:
- `internal/profile/generator.go`
  - Replace `FragmentProcessor` with `TemplateProcessor` interface
  - `TemplateProcessor.Process(manifest) → map[string]interface{}`
  - `DefaultTemplateProcessor` reads the template JSON, returns it
  - Update `DefaultSettingsBuilder.Build()` to use template instead of fragments
  - Update `BuilderFromPaths()` to create `TemplateProcessor`
- `internal/profile/settings_generator.go`
  - Update `RegenerateSettings()` — load template instead of merging fragments
  - Remove `mergeSettingFragments()` function

---

### Phase 4: CLI commands

**Goal:** Full template CRUD CLI.

Files to create:
- `cmd/template.go` — Parent command `ccp template`
- `cmd/template_list.go` — `ccp template list [--json]`
- `cmd/template_show.go` — `ccp template show <name>` (prints JSON)
- `cmd/template_create.go` — `ccp template create <name> [--from-file <path>]` (opens $EDITOR if no file)
- `cmd/template_extract.go` — `ccp template extract <name> [--from <profile>]` (extract from profile's settings.json, excluding hooks)
- `cmd/template_delete.go` — `ccp template delete <name>`
- `cmd/template_edit.go` — `ccp template edit <name>` (opens $EDITOR)

Files to modify:
- `cmd/profile_create.go` — Add `--template` flag
- `cmd/engine_create.go` — Add `--template` flag (replaces fragment selection)
- `cmd/engine_show.go` — Show template name instead of fragments
- `cmd/engine_list.go` — Show template column
- `cmd/profile_edit.go` — Add `--template` flag, remove `--add-setting-fragments`
- `cmd/completions.go` — Add template completions, remove fragment completions

---

### Phase 5: Update remaining references

Files to modify:
- `cmd/hub.go` — Remove `setting-fragments` from hub item type lists
- `cmd/hub_add.go` — Remove fragment handling
- `cmd/hub_link.go` — Remove fragment linking
- `cmd/link.go` / `cmd/unlink.go` — Remove fragment handling
- `cmd/status.go` — Show template instead of fragments
- `cmd/profile_sync.go` — Use template in sync logic
- `cmd/init.go` — Extract settings as template during init
- `internal/hub/scanner.go` — Scan includes templates
- `internal/source/installer.go` — Remove "setting-fragments" from item types
- `internal/profile/drift.go` — Update drift detection for templates

---

### Phase 6: Cleanup & migration

Files to delete:
- `internal/hub/fragment.go`
- `internal/hub/fragment_test.go`
- `cmd/hub_extract_fragments.go` — Replaced by `cmd/template_extract.go`
- `internal/migration/setting_fragments.go`
- `internal/migration/setting_fragments_test.go`

Files to modify:
- `internal/migration/migrator.go` — Add fragment-to-template migration
  - Read all existing fragments, merge into single template
  - Save as `settings-templates/migrated/settings.json`
  - Update manifests: remove `setting-fragments`, set `settings-template = "migrated"`
- `internal/migration/symlink_migrator.go` — Remove fragment references
- Bump `ManifestVersion` to 4

---

### Phase 7: Tests

- `internal/hub/template_test.go` — Template CRUD tests
- `internal/profile/generator_test.go` — Update for TemplateProcessor
- `internal/profile/settings_generator_test.go` — Update for template-based generation
- `internal/profile/resolver_test.go` — Template resolution tests
- `internal/profile/engine_test.go` — Update for template field
- `internal/migration/migrator_test.go` — Fragment-to-template migration tests

---

## Implementation Order

```
Phase 1 (types) → Phase 2 (manifest/engine) → Phase 3 (generation) →
Phase 4 (CLI) → Phase 5 (references) → Phase 6 (cleanup) → Phase 7 (tests)
```

Each phase should compile and tests pass before moving to next. Phase 1-3 can keep old fragment code alive temporarily (dual support) to allow incremental migration.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Template stores full settings.json | Yes | Simplicity — what you see is what you get |
| Hooks excluded from template | Yes | Hooks are overlay, managed by hub hooks system |
| One template per profile/engine | Yes | Fragments were N:1, template is 1:1 — simpler |
| `settings-templates` as hub item type | Yes | Consistent with hub structure |
| Directory per template (not flat file) | Yes | Consistent with other hub items (skills, hooks, etc.) |
| ManifestVersion bump to 4 | Yes | Breaking change to manifest format |
