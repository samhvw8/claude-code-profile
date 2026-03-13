# ccp - Claude Code Profile Manager

**Current version: v0.27.0**

## Project Context

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, go-toml/v2 for TOML config, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive TUI selection.

## Architecture

```
internal/
├── config/     # Path resolution, types, CcpConfig (ccp.toml)
├── errors/     # Custom error types (ProfileError, HubError, DriftError)
├── hub/        # Hub scanning, item management, settings templates, fragment processing (legacy)
├── source/     # Unified source system (providers, registries, installer)
├── profile/    # Profile CRUD, manifest, engines, contexts, resolver, settings, drift
├── symlink/    # Platform-specific symlink operations (unix/windows)
├── migration/  # YAML→TOML migration, source migration, rollback
└── picker/     # Bubble Tea multi-select TUI

cmd/            # Cobra commands (one file per command/subcommand)
```

## Development Commands

```bash
go build -o ccp .         # Build binary
go test ./...             # Run all tests
go test ./... -v          # Verbose test output
go mod tidy               # Update dependencies
./ccp --help              # Test CLI
```

## Code Standards

### Go Conventions
- Standard Go formatting (gofmt)
- Errors returned, not panicked
- Interfaces for testability (Scanner, Manager, Detector, FragmentReader, HookProcessor)
- Platform-specific code via build tags (`//go:build !windows`)

### CLI Patterns
- Commands: verb-noun pattern (`profile create`, `hub list`)
- Flags: `--long-form` with `-s` short aliases where useful
- Output: tabwriter for aligned columns, fmt for simple output
- Errors: return errors to Cobra, exit 1 on failure

### File Organization
- `cmd/root.go` - Root command, version
- `cmd/<command>.go` - Top-level commands (init, use, link, unlink, migrate)
- `cmd/<parent>_<child>.go` - Subcommands (profile_create, profile_list)
- `internal/<domain>/` - Domain logic, keep cmd layer thin

### Testing
- Table-driven tests preferred
- Use `t.TempDir()` for filesystem tests
- Test files: `*_test.go` alongside implementation

## Key Types

```go
// internal/config/paths.go
type Paths struct {
    CcpDir         string // ~/.ccp (ccp data directory)
    ClaudeDir      string // ~/.claude or $CLAUDE_CONFIG_DIR (may be project-specific)
    GlobalClaudeDir string // ~/.claude (always global, ignores CLAUDE_CONFIG_DIR)
    HubDir         string // ~/.ccp/hub
    ProfilesDir string // ~/.ccp/profiles
    SharedDir   string // ~/.ccp/profiles/shared
    StoreDir    string // ~/.ccp/store (shared downloadable resources)
    EnginesDir  string // ~/.ccp/engines
    ContextsDir string // ~/.ccp/contexts
}

type HubItemType string      // skills, agents, hooks, rules, commands, setting-fragments
type DataItemType string     // tasks, todos, history, etc.
type ShareMode string        // shared, isolated
type PluginStoreItem string  // marketplaces, cache, known_marketplaces.json

// internal/profile/manifest.go
type Manifest struct {
    Version           int           // 3 = current (engine/context), 2 = TOML, 1 = YAML
    Name, Description string
    Engine            string        // Optional engine reference
    Context           string        // Optional context reference
    SettingsTemplate  string        // Optional settings template name
    Created, Updated  time.Time
    Hub               HubLinks      // What hub items to link (overrides)
    Data              DataConfig    // Shared vs isolated data dirs
    LinkedDirs        []string      // Dirs referenced by @imports in CLAUDE.md
}

// internal/profile/engine.go
type Engine struct {
    Name, Description string
    SettingsTemplate  string        // Optional settings template name
    Hub               EngineHub     // hooks (+ legacy setting-fragments)
    Data              DataConfig    // Data sharing config
}

// internal/profile/context.go
type Context struct {
    Name, Description string
    Hub               ContextHub    // skills, agents, rules, commands, hooks
}

// internal/source/types.go
type Source struct {
    Registry, Provider string  // e.g., "github", "git"
    URL, Path          string  // Clone URL and local path
    Ref, Commit        string  // Git ref and pinned commit
    Installed          []string // Installed items (skills/foo, agents/bar)
}

// internal/config/ccp_config.go
type CcpConfig struct {
    DefaultRegistry string         // "skills.sh" or "github"
    GitHub          GitHubConfig   // topics, per_page
    SkillsSh        SkillsShConfig // base_url, limit
}

// internal/hub/template.go
type Template struct {
    Name     string                 // Directory name
    Settings map[string]interface{} // Complete settings.json content (hooks excluded)
}

// internal/hub/fragment.go (LEGACY — use templates for new code)
type Fragment struct {
    Name        string      // Fragment identifier
    Description string      // Human-readable description
    Key         string      // Settings key to set
    Value       interface{} // Value to set
}

// internal/profile/generator.go
type HookProcessor interface {
    ProcessAll(manifest *Manifest) (map[HookType][]SettingsHookEntry, error)
}

type FragmentProcessor interface {
    ProcessAll(manifest *Manifest) (map[string]interface{}, error)
}

type SettingsBuilder interface {
    Build(manifest *Manifest) (map[string]interface{}, error)
}
```

## Settings Templates

Complete `settings.json` templates stored in the hub. Profiles and engines reference a template by name. Replaces the old per-key setting-fragments system.

```bash
ccp template list                          # List available templates
ccp template show <name>                   # Display template JSON
ccp template create <name>                 # Create new (opens $EDITOR or --from-file)
ccp template extract <name> --from <profile>  # Extract from existing profile's settings
ccp template delete <name>
ccp template edit <name>                   # Edit in $EDITOR

# Use with profiles and engines
ccp profile create <name> --template opus-full
ccp engine create <name> --template haiku-fast
ccp profile edit <name> --template minimal
```

Storage: `~/.ccp/hub/settings-templates/<name>/settings.json`

Resolution order: Engine's template → Profile's template (profile wins if set). Hooks are always overlaid from hub hooks, not stored in templates.

## Two-Layer Profile Composition

Profiles can compose an **engine** (runtime config) + **context** (prompt/capabilities):

| Layer | Hub Items | Rationale |
|-------|-----------|-----------|
| Engine | setting-fragments, hooks | Runtime behavior, permissions |
| Context | skills, agents, rules, commands, hooks | Prompt content, capabilities |
| Profile | Any (overrides) | Profile-specific extras |

Resolution order (lowest to highest priority): Engine → Context → Profile (union-merged, deduplicated).

```bash
# Engine CRUD
ccp engine create <name> [-e | -i]
ccp engine list [--json]
ccp engine show <name>
ccp engine delete <name>

# Context CRUD
ccp context create <name> [-e | -i]
ccp context list [--json]
ccp context show <name>
ccp context delete <name>

# Profile composition
ccp profile create <name> --engine opus-full --context coding
ccp profile edit <name> --engine haiku-fast
```

Engine and context fields are **optional** — profiles without them work as before (inline hub items).

## Source System

Unified source management for skills, agents, and plugins:

```bash
ccp find <query>                        # Search skills.sh (shows PACKAGE + SKILL columns)
ccp find -r github <query>              # Search GitHub repos
ccp install                             # Sync all from ccp.toml (for machine migration)
ccp install <owner/repo>                # Auto-add + interactive install (recommended)
ccp install <owner/repo> skills/<name>  # Install specific skill directly
ccp install <owner/repo> -a             # Auto-add + install all items
ccp source add <owner/repo>             # Add source only (falls back to GitHub if not on skills.sh)
ccp source list                         # List installed sources
ccp source update [name]                # Update sources
ccp source remove <name>                # Remove source
```

### Source Workflow

1. `find` searches skills.sh by default, shows PACKAGE (owner/repo) and SKILL separately
2. `install <owner/repo>` auto-adds source if not found, then shows interactive picker
3. `install <owner/repo> skills/<name>` installs a specific skill directly without picker
4. `install` (no args) syncs all sources from ccp.toml - clones missing sources and reinstalls items
5. `source add` tries skills.sh first, falls back to GitHub with default branch

## CLAUDE.md Linked Directories

Claude Code supports `@path/to/file.md` imports in CLAUDE.md. ccp parses these references, stores the directories as reusable `rules` hub items, and creates root-level symlinks so `@` imports resolve correctly.

- **Parser**: `internal/claudemd/parser.go` extracts `@path` references (skips code blocks and annotations)
- **Hub storage**: Referenced dirs (e.g., `principles/`) are stored as `hub/rules/principles/` — reusable across profiles
- **Dual symlinks**: Each linked dir gets both `profileDir/rules/{name}` (standard) and `profileDir/{name}` (root-level for `@` resolution)
- **Manifest tracking**: `linked-dirs` field identifies which rules items need root-level symlinks
- **Init**: During `ccp init`, referenced dirs are moved to hub/rules and symlinked
- **Profile create**: `--from` copies linked-dirs + CLAUDE.md; hub items are shared via symlinks
- **Migrate**: `ccp migrate` moves existing profile dirs to hub/rules and creates symlinks

## Hooks Format

Hooks use the official Claude Code `hooks.json` format for plugin compatibility:

```
~/.ccp/hub/hooks/{name}/
├── hooks.json         # Official format
└── scripts/           # Scripts directory
    └── script.sh
```

### hooks.json Structure

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/session-start.sh",
            "timeout": 60
          }
        ]
      }
    ]
  }
}
```

### Key Types

```go
// internal/config/hooks.go
type HooksJSON struct {
    Hooks map[HookType][]HookEntry `json:"hooks"`
}

type HookEntry struct {
    Matcher string        `json:"matcher,omitempty"`
    Hooks   []HookCommand `json:"hooks"`
}

type HookCommand struct {
    Type    string `json:"type"`              // Always "command"
    Command string `json:"command"`           // Path or ${CLAUDE_PLUGIN_ROOT}/...
    Timeout int    `json:"timeout,omitempty"`
}
```

### Hook Event Types

- `SessionStart` - Session startup, resume, clear, compact
- `UserPromptSubmit` - User submits a prompt
- `PreToolUse` / `PostToolUse` - Before/after tool execution (use `matcher`)
- `Stop` / `SubagentStop` - Session or subagent stops

### Backward Compatibility

Legacy `hook.yaml` format still supported. `GetHookManifest()` tries `hooks.json` first, falls back to `hook.yaml`.

Run `ccp migrate` to convert existing `hook.yaml` to `hooks.json`:
- Moves scripts to `scripts/` subdirectory
- Converts paths to `${CLAUDE_PLUGIN_ROOT}`
- Removes old `hook.yaml` after conversion

## Configuration

Global config at `~/.ccp/ccp.toml`:

```toml
default_registry = "skills.sh"

[github]
topics = ["agent-skills", "claude-code", "claude-skills"]
per_page = 10

[skillssh]
base_url = "https://skills.sh"
limit = 10

# Installed sources (auto-managed by ccp source commands)
# Paths are relative to ~/.ccp/ for portability
[sources.'owner/repo']
registry = 'github'
provider = 'git'
url = 'https://github.com/owner/repo.git'
path = 'sources/owner--repo'
ref = 'main'
commit = 'abc123...'
installed = ['skills/my-skill', 'agents/my-agent']
```

Generate default config: `ccp config init`

## Directory Structure

```
~/.ccp/
├── engines/                    # Reusable runtime config layers
│   └── {name}/
│       └── engine.toml
├── contexts/                   # Reusable prompt/capability layers
│   └── {name}/
│       └── context.toml
├── hub/                        # Human-configurable (ccp-managed)
│   ├── skills/
│   ├── agents/
│   ├── hooks/
│   ├── rules/
│   ├── commands/
│   ├── settings-templates/     # Complete settings.json templates
│   └── setting-fragments/      # Legacy (use settings-templates instead)
├── store/                      # Shared downloadable resources
│   └── plugins/
│       ├── marketplaces/       # Downloaded marketplace repos
│       ├── cache/              # Plugin cache
│       ├── known_marketplaces.json
│       └── install-counts-cache.json
├── sources/                    # Cloned source repositories
├── profiles/
│   ├── shared/                 # Shared runtime data
│   │   ├── tasks/
│   │   ├── todos/
│   │   ├── paste-cache/
│   │   └── projects/
│   └── {name}/                 # Individual profile
│       ├── profile.toml        # Profile manifest (may ref engine + context)
│       ├── skills/ → hub/skills/{linked}
│       ├── agents/ → hub/agents/{linked}
│       ├── plugins/
│       │   ├── marketplaces → store/plugins/marketplaces
│       │   ├── cache → store/plugins/cache
│       │   └── installed_plugins.json  # Profile-specific
│       └── ...
└── ccp.toml                    # Config + installed sources
```

### Data Classification

| Type | Category | Location | Sharing |
|------|----------|----------|---------|
| Hub items (skills, agents, hooks) | Human-config | `~/.ccp/hub/` | Linked per profile |
| Plugin cache (marketplaces, cache) | Human-config | `~/.ccp/store/plugins/` | Shared via symlinks |
| Runtime data (tasks, todos, history) | Runtime | `~/.ccp/profiles/shared/` or profile | Configurable |
| Plugin state (installed_plugins.json) | Runtime | Profile `plugins/` | Isolated |

## Before Making Changes

1. **Read existing code** - Match patterns in similar files
2. **Run tests** - `go test ./...` before and after
3. **Check build** - `go build -o ccp .` compiles cleanly
4. **Platform awareness** - Symlink code has unix/windows variants

## Common Tasks

### Adding a New Command

1. Create `cmd/<name>.go` with Cobra command
2. Register in `init()` with `rootCmd.AddCommand()` or `parentCmd.AddCommand()`
3. Add flags with `cmd.Flags()` in `init()`
4. Implement `RunE` function with error handling

### Adding Domain Logic

1. Create/extend package in `internal/<domain>/`
2. Define interface for testability
3. Add tests in `*_test.go`
4. Wire up in cmd layer

### Git Tagging

- **Never force-update tags** (`git tag -f` or `git push --tags -f`)
- Force-updating tags prevents GitHub CI from re-running
- Always increment version and create a new tag
