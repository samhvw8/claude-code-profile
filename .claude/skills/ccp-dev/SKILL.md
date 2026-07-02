---
name: ccp-dev
description: "Develop, debug, and extend ccp — the Claude Code Profile Manager CLI. Use when: adding a new command, fixing a bug in profile/hub/source logic, extending the source system, modifying settings generation, working with symlinks or manifests, understanding how ccp works internally. Actions: build, fix, add, extend, refactor, debug ccp code. Keywords: ccp, profile manager, hub item, source install, Cobra command, settings template, manifest, picker, fuzzy search, symlink, migration, go build, go test. Casual triggers: 'add a command to ccp', 'ccp is broken', 'how does ccp source work', 'new ccp feature', 'profile not switching', 'hub items not linking', 'picker not working'."
---

# ccp Development

Go CLI tool for managing Claude Code profiles via a central hub.

## Architecture

```
cmd/                    # Cobra commands — thin wrappers calling internal/
internal/
├── config/             # Paths, CcpConfig (ccp.toml), types
├── errors/             # Custom error types
├── hub/                # Hub scanning, templates, item management
├── source/             # Source system (providers, registries, installer)
├── profile/            # Profile CRUD, manifest, settings generation, drift
├── symlink/            # Platform-specific symlink operations (unix/windows)
├── migration/          # Format migrations, rollback
└── picker/             # Bubble Tea TUI (fuzzy search, tab-to-toggle, fzf-style)
```

## Patterns

### Adding a Command
1. Create `cmd/<parent>_<child>.go` (or `cmd/<name>.go` for top-level)
2. Define `var <name>Cmd = &cobra.Command{...}` with `RunE`
3. Register in `init()` with `parentCmd.AddCommand()`
4. Keep cmd layer thin — delegate to `internal/`

**Watch out:** Max 5 user-facing concepts. Prefer flags on existing commands over new commands.

### Picker TUI (`internal/picker/`)

| File | Model | Use |
|------|-------|-----|
| `picker.go` | `Model` | Multi-select with checkboxes |
| `single.go` | `SingleModel` | Single-select (profile switch) |
| `tabbed.go` | `TabbedModel` | Multi-tab multi-select (profile create/edit) |
| `fuzzy.go` | — | fzf-style fuzzy scorer + `sortByFuzzyScore()` |

**Key behavior (fzf-style):**
- `/` enters search mode — type to fuzzy-filter, results sorted by match quality
- In search mode: `↑`/`↓` navigate, `Tab` toggles, `Enter` confirms, `Esc` clears
- In normal mode: `Tab`/`Space` toggle, `a` all/none, `f` filter checked/unchecked
- Fuzzy scoring: consecutive char bonus, word boundary bonus, start-of-string bonus
- `getFilteredItems()` applies filter mode first, then `sortByFuzzyScore()` if search active

### Source System
Three-layer: Registry → Provider → Installer
- Registry: resolves package ID to URL (skills.sh, GitHub, manual)
- Provider: fetches content (git clone, HTTP download)
- Installer: discovers items in source dir, copies to hub
- Layouts: root `skills/`, `.claude/skills/`, `.claude-plugin/plugin.json`, `plugins/<name>/skills/`

### Settings Generation
Single function, not a pipeline:
`profile.GenerateSettings(manifest, hubDir)` — loads template, overlays hooks, returns map.
No processor/builder interfaces (deliberately simplified in v0.28).

### Symlinks
Use `internal/symlink/` package. Relative symlinks for portability.
**Watch out:** macOS `/var` → `/private/var`. Use `filepath.EvalSymlinks()` in tests.
**Watch out:** `os.Readlink` returns raw target — resolve relative targets with `filepath.Join(filepath.Dir(link), target)` before `os.Stat`.

## Anti-Patterns (Do NOT Re-introduce)

| Pattern | Why Removed |
|---------|-------------|
| Engines/Contexts | Premature abstraction for solo-dev tool |
| Setting fragments | Per-key YAML replaced by complete templates |
| CLAUDE.md @import parsing | Too complex for niche feature |
| Processor interface chains | Single function suffices |
| Configurable data sharing | All data always shared |
| Force-updating git tags | Prevents GitHub CI from re-running |
| External fuzzy lib | Hand-rolled 70-line scorer suffices |

## Build & Test

```bash
go build -o ccp .       # Build
go test ./...            # All tests
go test ./... -v         # Verbose
go mod tidy              # Dependencies
```

## Key Types

```go
type Paths struct {
    CcpDir, ClaudeDir, GlobalClaudeDir string
    HubDir, ProfilesDir, SharedDir, StoreDir string
}
type Manifest struct {
    Version int
    Name, Description, SettingsTemplate string
    Hub HubLinks
}
type Source struct {
    Registry, Provider, URL, Path, Ref, Commit string
    Installed []string
}
```

## Decision Heuristics

- Flat profiles, no composition layers
- Hub IS the sharing mechanism
- Complete templates over fragments
- Max 5 user-facing concepts
- If it can be a flag, don't make it a command
- Hidden commands are OK for power users
