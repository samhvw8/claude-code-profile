# Plan: `ccp project install` — Install from sources directly into project scope

**Created:** 2025-04-25
**Status:** draft

## Problem

Currently `ccp project add` only copies items from the **local hub** (`~/.ccp/hub/`) into a project's `.claude/` directory. Users must first run `ccp install <source> <item>` to get items into their hub, then `ccp project add <type/name>` to copy them into a project.

This is two steps where one should suffice. The user wants:
```bash
ccp project install owner/repo skills/my-skill
```
→ Fetch from source, copy directly into `.claude/skills/my-skill` in the current project.

## Current State

| Command | What it does |
|---------|-------------|
| `ccp install` / `ccp source install` | Fetch source → install items into **hub** (`~/.ccp/hub/`) |
| `ccp project add` | Copy from **hub** → project `.claude/` |
| `ccp project list` | Scan project `.claude/` for items |
| `ccp project remove` | Delete items from project `.claude/` |

**Missing:** `ccp project install` — fetch from source → copy directly into project `.claude/`.

## Solution

Add a `project install` subcommand that reuses the existing source infrastructure but targets the project `.claude/` directory instead of the hub.

### Command Design

```
ccp project install [source] [items...]

Aliases: i

Flags:
  --all, -a        Install all available items (excluding settings-templates)
  --interactive, -i  Interactive picker
  --dir             Project root directory (inherited from project parent)

Examples:
  ccp project install remorses/playwriter              # Interactive selection
  ccp project install owner/repo skills/my-skill       # Specific item
  ccp project install owner/repo --all                 # All items
```

### Behavior

1. Source resolution: Same as `ccp source install` — auto-adds source if not already known
2. Item discovery: Same as `ccp source install` — scans source for available items
3. **Key difference:** Items are copied to `<project>/.claude/<type>/<name>` instead of `~/.ccp/hub/<type>/<name>`
4. Registry tracking: Source is still tracked in `registry.toml` so `ccp source update` works, but items are NOT added to `source.Installed[]` (they're project-local, not hub-managed)
5. Type filtering: Excludes `settings-templates` (same as existing `project add`)

### Architecture

No new packages needed. The flow is:

```
cmd/project_install.go
  ├── Resolve source (reuse source_install.go's addSourceForInstall)
  ├── Discover items (reuse installer.DiscoverItems)
  ├── Interactive picker (reuse picker, filter projectHubItemTypes)
  └── Copy items to project .claude/ (new: installer.InstallToDir or direct CopyTree)
```

**Option A: Add `InstallToDir` method to `source.Installer`**

```go
func (i *Installer) InstallToDir(sourceID string, items []string, targetDir string) ([]string, error)
```

Same as `Install()` but targets `targetDir` instead of `i.paths.HubDir`, and skips registry tracking.

**Option B: Inline in cmd layer using existing `resolveItemPaths` + `CopyTree`**

Since the installer's `resolveItemPaths` is unexported, Option A is cleaner — it exposes the right abstraction without leaking internals.

**Recommendation: Option A** — keeps the cmd layer thin (project identity principle), reuses path resolution logic.

### Completions

Add `completeProjectInstallArgs` in `completions.go`:
- First arg: complete with known source IDs from registry
- Subsequent args: complete with discovered items from that source

## Phases

### Phase 1: Core implementation
1. Add `InstallToDir(sourceID string, items []string, targetDir string, allowedTypes []string) ([]string, error)` to `source.Installer`
   - Same item resolution as `Install()`
   - Copy to `targetDir/<type>/<name>` instead of hub
   - Skip `registry.AddInstalled()` (project items aren't hub-tracked)
   - Filter out disallowed types (settings-templates)
2. Add `cmd/project_install.go` with the `project install` subcommand
   - Reuse `addSourceForInstall()` from `source_install.go`
   - Reuse `projectHubItemTypes` filter
   - Wire up interactive picker with type filtering
3. Add completion function in `completions.go`
4. Tests: table-driven tests for `InstallToDir`, command-level tests for project install

### Phase 2: (Optional, not in scope)
- `ccp project sync` — re-install items from a project manifest
- `ccp project diff` — compare project items vs hub versions

## Files Changed

| File | Change |
|------|--------|
| `internal/source/installer.go` | Add `InstallToDir()` method |
| `cmd/project_install.go` | New file — `project install` subcommand |
| `cmd/completions.go` | Add `completeProjectInstallArgs` |
| `cmd/project_install_test.go` | New file — tests |

## Risks

| Risk | Mitigation |
|------|-----------|
| Source auto-add side effect | Same behavior as `ccp install` — expected |
| No registry tracking for project items | By design — project items are local. Document this. |
| Duplicate items (hub + project) | User's choice — project `.claude/` takes precedence in Claude Code |

## Not Doing

- No project-level manifest tracking (project items are standalone copies)
- No `project update` (re-fetch from source) — that's a follow-up if needed
- No settings-template support in project scope (Claude Code doesn't read settings from project `.claude/`)
