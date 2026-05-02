---
name: ccp-dev
description: "ccp (Claude Code Profile Manager) development. Go CLI with Cobra, go-toml/v2, Bubble Tea TUI. Architecture: cmd/ (thin Cobra layer), internal/ (config, hub, profile, source, symlink, migration, picker). Capabilities: adding commands, hub item management, source system, settings generation, profile CRUD, platform symlinks. Actions: build, test, add commands, extend hub, fix bugs in ccp. Keywords: ccp, Claude Code Profile, profile manager, hub, source, Cobra, Go CLI, symlink, settings template, manifest, picker, migration. Use when: developing ccp features, adding commands, modifying hub/source/profile logic, debugging ccp issues, understanding ccp architecture."
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
└── picker/             # Bubble Tea multi-select TUI
```

## Patterns

### Adding a Command
**When you see:** Need for a new CLI command
**This indicates:** Create file in cmd/, wire to parent
**Therefore:**
1. Create `cmd/<parent>_<child>.go` (or `cmd/<name>.go` for top-level)
2. Define `var <name>Cmd = &cobra.Command{...}` with `RunE`
3. Register in `init()` with `parentCmd.AddCommand()`
4. Keep cmd layer thin — delegate to `internal/`
**Watch out:** Check complexity budget (max 5 user-facing concepts). Prefer flags on existing commands over new commands.

### Hub Item Types
**When you see:** Code referencing item types
**This indicates:** Fixed set: skills, agents, commands, rules, hooks, settings-templates
**Therefore:** Use `config.HubItemType` constants. Hub scanner discovers these from `~/.ccp/hub/<type>/`
**Watch out:** Don't add new item types without strong justification

### Source System
**When you see:** Code for fetching/installing external items
**This indicates:** Three-layer architecture: Registry → Provider → Installer
**Therefore:**
- Registry: resolves package ID to URL (skills.sh, GitHub, manual)
- Provider: fetches content (git clone, HTTP download)
- Installer: discovers items in source dir, copies to hub
**Watch out:** Installer supports multiple layouts: root-level `skills/`, `.claude/skills/`, `.claude-plugin/plugin.json`, and `plugins/<name>/skills/`

### Settings Generation
**When you see:** Code generating settings.json
**This indicates:** Single function, not a pipeline
**Therefore:** `profile.GenerateSettings(manifest, hubDir)` — loads template, overlays hooks, returns map
**Watch out:** No processor/builder interfaces. This was deliberately simplified in v0.28.

### Profile Manifest
**When you see:** Code reading/writing profile.toml
**This indicates:** Manifest v3 (TOML format)
**Therefore:** `profile.Manifest` struct with Name, Description, SettingsTemplate, Hub links
**Watch out:** Version field drives migration. All data dirs are always shared — no per-type config.

### Platform Symlinks
**When you see:** Symlink creation/resolution
**This indicates:** Platform-specific code with build tags
**Therefore:** Use `internal/symlink/` package. Relative symlinks for portability.
**Watch out:** macOS resolves `/var` → `/private/var`. Use `filepath.EvalSymlinks()` in tests.

## Anti-Patterns (Do NOT Re-introduce)

| Pattern | Why Removed |
|---------|-------------|
| Engines/Contexts | Premature abstraction for solo-dev tool |
| Setting fragments | Per-key YAML replaced by complete templates |
| CLAUDE.md @import parsing | Too complex for niche feature |
| Processor interface chains | Single function suffices |
| Configurable data sharing | All data always shared |
| Force-updating git tags | Prevents GitHub CI from re-running |

## Build & Test

```bash
go build -o ccp .       # Build
go test ./...            # All tests
go test ./... -v         # Verbose
go mod tidy              # Dependencies
```

## Key Types

```go
// Paths — all directory resolution
type Paths struct {
    CcpDir, ClaudeDir, GlobalClaudeDir string
    HubDir, ProfilesDir, SharedDir, StoreDir string
}

// Manifest — profile definition (profile.toml)
type Manifest struct {
    Version int
    Name, Description, SettingsTemplate string
    Hub HubLinks
}

// Source — external package in registry
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
