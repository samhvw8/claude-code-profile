# ccp Simplification: v0.27 -> v1.0

**Created:** 2026-03-30
**Status:** draft
**Goal:** 60% concept reduction, ~6K LOC removal, ~15 visible commands

## Summary

Strip ccp to its essential kernel: **hub + profiles + settings templates**. Remove engines, contexts, setting-fragments, linked-dirs magic, and redundant commands. Flat profiles with per-profile settings (including API keys/accounts).

## Current State

| Metric | v0.27.0 |
|--------|---------|
| Concepts | 14 |
| Commands | 77 files (~55 distinct) |
| Source LOC | ~21,600 |
| Internal packages | 9 |
| cmd/ LOC | 11,300 |
| migration/ LOC | 3,295 + 2,835 test |

## Target State

| Metric | v1.0 |
|--------|------|
| Concepts | 5 (hub, profile, settings template, source, activation) |
| Commands | ~25 files (~15 visible) |
| Source LOC | ~15,000 |
| Internal packages | 7 (drop claudemd, merge errors into config) |
| migration/ LOC | ~1,500 (delete completed migrations) |

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Engine/context | Remove | Premature abstraction; profiles compose hub items directly |
| Setting fragments | Delete code | Migration exists; dual-system is dead weight |
| API keys / accounts | Per-profile | User has personal + work accounts; copy on clone |
| Data sharing | Default shared, no config | Remove 8-mode DataConfig; shared is the right default |
| Linked dirs (@import) | Remove | Disproportionate complexity; users manage @imports manually |
| Settings pipeline | Single function | Template + hooks overlay; no processor interfaces |
| Plugin marketplace | Keep install, simplify store | Remove marketplace abstraction, keep basic install flow |
| Command surface | Hide power-user commands | Don't delete, just remove from default help |

---

## Phases

### Phase 1: Delete setting-fragments

**Risk: Low** | **LOC removed: ~800 source + ~600 test**

The migration path already exists (template_migrator). Remove the code.

**Delete files:**
- `internal/hub/fragment.go` + `fragment_test.go`
- `internal/migration/setting_fragments.go` + `setting_fragments_test.go`
- `cmd/hub_extract_fragments.go`

**Modify files:**
- `internal/config/paths.go` — remove `HubSettingFragments` from `AllHubItemTypes()`
- `internal/config/paths_test.go` — update type count
- `internal/profile/manifest.go` — remove `SettingFragments` from `HubLinks`, remove cases from GetHubItems/SetHubItems/AddHubItem/RemoveHubItem
- `internal/profile/engine.go` — remove `SettingFragments` from `EngineHub`
- `internal/profile/resolver.go` — remove `allFragments` accumulation
- `internal/profile/generator.go` — remove `FragmentProcessor` interface and implementation
- `internal/profile/generator_test.go` — remove fragment processor tests
- `internal/profile/settings_generator.go` — remove `mergeSettingFragments()` calls
- `internal/profile/drift.go` — remove fragment drift checking
- `internal/profile/profile.go` — remove fragment references
- `internal/hub/scanner.go` — remove fragment scanning
- `internal/source/installer.go` — remove "setting-fragments" from item types
- `internal/migration/migrator.go` — remove fragment fallback paths in Execute()
- `internal/migration/symlink_migrator.go` — remove fragment references
- `internal/migration/template_migrator.go` — remove fragment import if no longer needed
- `cmd/completions.go` — remove fragment completions
- `cmd/engine_create.go` — remove `--setting-fragments` flag
- `cmd/engine_list.go`, `cmd/engine_show.go` — remove fragment display
- `cmd/hub.go` — remove "setting-fragments" from valid types
- `cmd/hub_add.go`, `cmd/hub_link.go` — remove fragment handling
- `cmd/link.go`, `cmd/unlink.go` — remove fragment handling
- `cmd/profile_create.go` — remove `--setting-fragments` flag
- `cmd/profile_edit.go` — remove `--add/remove-setting-fragments` flags
- `cmd/profile_sync.go` — remove fragment sync logic
- `cmd/status.go` — remove fragment count display

**Verify:** `go build && go test ./...`

---

### Phase 2: Remove engines and contexts

**Risk: Medium** | **LOC removed: ~1,500 source + ~500 test**

Flatten engine/context references into inline profile hub items. Add a migration step.

**Delete files:**
- `cmd/engine.go`, `engine_create.go`, `engine_delete.go`, `engine_list.go`, `engine_show.go` (5 files)
- `cmd/context.go`, `context_create.go`, `context_delete.go`, `context_list.go`, `context_show.go` (5 files)
- `internal/profile/engine.go` + `engine_test.go` (after migration created)
- `internal/profile/context.go` + `context_test.go`
- `internal/profile/resolver.go` + `resolver_test.go`

**Create files:**
- `internal/migration/flatten_migrator.go` — reads engine+context, merges hub items into profile manifest, clears engine/context fields
- `internal/migration/flatten_migrator_test.go`

**Modify files:**
- `internal/profile/manifest.go` — remove `Engine`, `Context` fields; remove `UsesComposition()`
- `internal/config/paths.go` — remove `EnginesDir`, `ContextsDir`, `EngineDir()`, `ContextDir()`
- `internal/migration/structure_migrator.go` — remove engines/contexts dir creation
- `cmd/migrate.go` — add flatten migration step
- `cmd/profile_create.go` — remove `--engine`, `--context` flags
- `cmd/profile_edit.go` — remove `--engine`, `--context` flags
- `cmd/profile_list.go` — remove engine/context columns
- `cmd/completions.go` — remove engine/context completions
- `cmd/status.go` — remove engine/context counts
- `internal/profile/generator.go` — remove ResolveManifest call; settings build from flat manifest
- `internal/profile/settings_generator.go` — simplify to work with flat manifest

**Migration logic (flatten_migrator.go):**
```go
// For each profile with engine or context set:
// 1. Load engine → merge engine.Hub.Hooks into profile.Hub.Hooks
// 2. Load engine.SettingsTemplate → set on profile if profile has none
// 3. Load context → merge context.Hub.{Skills,Agents,Rules,Commands,Hooks} into profile.Hub
// 4. Clear profile.Engine and profile.Context fields
// 5. Save profile manifest
```

**Verify:** `go build && go test ./...`

---

### Phase 3: Remove linked-dirs magic

**Risk: Low** | **LOC removed: ~500 source + ~300 test**

Remove the CLAUDE.md @import parser and dual-symlink system. Users manage @imports manually.

**Delete files:**
- `internal/claudemd/parser.go` + `parser_test.go`
- `internal/migration/linkeddir_migrator.go` + `linkeddir_migrator_test.go`

**Delete package:**
- `internal/claudemd/` (entire directory)

**Modify files:**
- `internal/profile/manifest.go` — remove `LinkedDirs` field
- `internal/profile/profile.go` — remove linked-dir symlink creation logic
- `internal/migration/migrator.go` — remove linked-dir handling in Plan() and createDefaultProfile()
- `cmd/migrate.go` — remove linkedDirMigrator
- `cmd/init.go` — remove linked-dir scanning and symlink creation
- `cmd/profile_create.go` — remove `--from` linked-dir copying

**Verify:** `go build && go test ./...`

---

### Phase 4: Simplify data sharing config

**Risk: Low** | **LOC removed: ~200**

Default all data dirs to shared. Remove per-type config from manifest.

**Modify files:**
- `internal/profile/manifest.go` — remove `DataConfig` struct; all data dirs default to shared
- `internal/profile/manifest.go` — remove `GetDataShareMode()`, `SetDataShareMode()` and 8 switch cases
- `internal/config/paths.go` — simplify data dir helpers
- `internal/profile/profile.go` — remove data sharing logic; always symlink to shared
- `internal/migration/migrator.go` — simplify data dir handling in createDefaultProfile()
- `cmd/profile_create.go` — remove data sharing prompts/flags
- `cmd/engine_create.go` — already deleted in Phase 2

**Verify:** `go build && go test ./...`

---

### Phase 5: Collapse settings pipeline

**Risk: Low** | **LOC removed: ~300**

Replace processor interfaces with a single function.

**Modify files:**
- `internal/profile/generator.go` — replace `SettingsBuilder`, `TemplateProcessor`, `HookProcessor` interfaces with one function:
  ```go
  func GenerateSettings(manifest *Manifest, hubDir string) (map[string]interface{}, error) {
      settings := map[string]interface{}{}
      if manifest.SettingsTemplate != "" {
          tmpl, err := hub.NewTemplateManager(hubDir).Load(manifest.SettingsTemplate)
          if err != nil { return nil, err }
          settings = tmpl.Settings
      }
      hooks, err := collectHooks(hubDir, manifest.Hub.Hooks)
      if err != nil { return nil, err }
      if len(hooks) > 0 {
          settings["hooks"] = hooks
      }
      return settings, nil
  }
  ```
- `internal/profile/generator_test.go` — simplify tests to test the single function
- `internal/profile/settings_generator.go` — simplify `RegenerateSettings` to call `GenerateSettings`
- `cmd/profile_sync.go` — update to use new function signature

**Verify:** `go build && go test ./...`

---

### Phase 6: Consolidate commands

**Risk: Low** | **No LOC removed, just reorganized**

Reduce visible command surface. Keep files but hide power-user commands from default help.

**Hide from default help (set `Hidden: true` on cobra command):**
- `cmd/auto.go` — scripting helper
- `cmd/hub_prune.go` — power user
- `cmd/hub_outdated.go` — power user
- `cmd/hub_protect.go` — power user
- `cmd/hub_rename.go` — power user
- `cmd/profile_diff.go` — power user
- `cmd/profile_clone.go` — power user
- `cmd/profile_check.go` — redundant with `profile fix --dry-run`
- `cmd/session.go` — niche
- `cmd/run.go` — niche
- `cmd/env.go` — redundant with `use`
- `cmd/usage.go` — power user

**Merge:**
- `cmd/skills.go` + `skills_add.go` + `skills_find.go` + `skills_update.go` — redundant with `find`/`install`/`source update`. Delete these 4 files, ensure `find`/`install` cover the use cases.
- `cmd/completion.go` + `cmd/completions.go` — merge into single file if both exist

**Visible commands after Phase 6 (~18):**
```
ccp init
ccp use <profile> [-g]
ccp which
ccp status
ccp doctor [--fix]
ccp migrate

ccp profile create/list/edit/delete/sync/fix/rename
ccp hub list/add/remove/show/edit
ccp link/unlink

ccp find
ccp install
ccp template list/show/create/edit/extract/delete

ccp source list/add/remove/update
ccp plugin list/add/update
```

**Verify:** `go build && ccp --help` shows clean output

---

### Phase 7: Clean up completed migrations

**Risk: Low** | **LOC removed: ~1,500**

Migration code for old formats that all users have already passed through.

**Delete files (assess which are safe to remove):**
- `internal/migration/setting_fragments.go` — already deleted in Phase 1
- `internal/migration/linkeddir_migrator.go` — already deleted in Phase 3
- `internal/migration/hook_format_migrator.go` — hook.yaml → hooks.json (completed months ago)
- `internal/migration/symlink_migrator.go` — absolute → relative (completed months ago)
- `internal/migration/source_migrator.go` — source.yaml → ccp.toml (completed months ago)
- `internal/migration/registry_migrator.go` — registry.toml → ccp.toml (completed months ago)
- `internal/migration/structure_migrator.go` — engines/contexts dirs (removing those)
- `internal/migration/plugin_store_migrator.go` — plugin cache move (completed)

**Keep:**
- `internal/migration/migrator.go` — init migration (always needed)
- `internal/migration/toml_migrator.go` — YAML → TOML (may still have users)
- `internal/migration/template_migrator.go` — fragment → template (fresh)
- `internal/migration/flatten_migrator.go` — engine/context → flat (fresh from Phase 2)
- `internal/migration/hooks.go` — hook utilities (used by migrator.go)
- `internal/migration/settings.go` — settings utilities (used by migrator.go)
- `internal/migration/rollback.go` — rollback utilities
- `internal/migration/resetter.go` — reset command support

**Modify:**
- `cmd/migrate.go` — remove calls to deleted migrators
- `internal/migration/migrator.go` — remove references to deleted migrators

**Verify:** `go build && go test ./...`

---

### Phase 8: Update docs

**Rewrite these files to reflect v1.0:**
- `CLAUDE.md` — remove engine/context/fragment types, simplify Paths, update command list
- `docs/ccp-spec.md` — major rewrite: cut to ~400 lines, remove engine/context sections, update schemas, compress revision history
- `README.md` — if public, update for simplified mental model

---

## Execution Order

```
Phase 1 (fragments)     → builds, tests pass
Phase 2 (engines/ctx)   → builds, tests pass, migration works
Phase 3 (linked-dirs)   → builds, tests pass
Phase 4 (data sharing)  → builds, tests pass
Phase 5 (pipeline)      → builds, tests pass
Phase 6 (commands)      → builds, help is clean
Phase 7 (old migrations)→ builds, tests pass
Phase 8 (docs)          → spec reflects reality
```

Each phase is independently shippable. Run `go build && go test ./...` after each.

## Estimated Impact

| Phase | Files touched | LOC removed | Risk |
|-------|--------------|-------------|------|
| 1. Fragments | ~30 | ~1,400 | Low |
| 2. Engines/Contexts | ~20 | ~2,000 | Medium |
| 3. Linked dirs | ~10 | ~800 | Low |
| 4. Data sharing | ~6 | ~200 | Low |
| 5. Pipeline | ~4 | ~300 | Low |
| 6. Commands | ~16 | ~600 (skills delete) | Low |
| 7. Old migrations | ~12 | ~1,500 | Low |
| 8. Docs | 3 | net reduction | Low |
| **Total** | | **~6,800** | |

## Post-v1.0 Concepts

| Concept | Status |
|---------|--------|
| Hub (skills, agents, hooks, rules, commands) | Keep |
| Profile (manifest + settings template + hub links) | Keep |
| Settings template | Keep |
| Source (external repos) | Keep |
| Activation (use -g / env var) | Keep |
| Engine | **Removed** |
| Context | **Removed** |
| Setting fragments | **Removed** |
| Linked dirs | **Removed** |
| Data sharing config | **Removed** (default shared) |
| Processor interfaces | **Removed** (single function) |

## Version Strategy

- v0.28.0 — Phase 1 (delete fragments)
- v0.29.0 — Phase 2 (remove engines/contexts)
- v0.30.0 — Phases 3-5 (linked-dirs, data sharing, pipeline)
- v0.31.0 — Phase 6-7 (commands, old migrations)
- v1.0.0 — Phase 8 (docs rewrite, tag as stable)
