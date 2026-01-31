# ccp - Claude Code Profile Manager

## Project Context

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, go-toml/v2 for TOML config, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive TUI selection.

## Architecture

```
internal/
├── config/     # Path resolution, types, CcpConfig (ccp.toml)
├── errors/     # Custom error types (ProfileError, HubError, DriftError)
├── hub/        # Hub scanning and item management
├── source/     # Unified source system (providers, registries, installer)
├── profile/    # Profile CRUD, manifest (profile.toml), drift detection
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
- Interfaces for testability (Scanner, Manager, Detector)
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
    CcpDir      string // ~/.ccp (ccp data directory)
    ClaudeDir   string // ~/.claude (symlink to active profile)
    HubDir      string // ~/.ccp/hub
    ProfilesDir string // ~/.ccp/profiles
    SharedDir   string // ~/.ccp/profiles/shared
}

type HubItemType string    // skills, agents, hooks, rules, commands, setting-fragments
type DataItemType string   // tasks, todos, history, etc.
type ShareMode string      // shared, isolated

// internal/profile/manifest.go
type Manifest struct {
    Version           int           // 2 = TOML format
    Name, Description string
    Created, Updated  time.Time
    Hub               HubLinks      // What hub items to link
    Data              DataConfig    // Shared vs isolated data dirs
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
```

## Source System

Unified source management for skills, agents, and plugins:

```bash
ccp source find <query>              # Search skills.sh (default)
ccp source find -r github <query>    # Search GitHub repos
ccp source add <owner/repo>          # Add from GitHub
ccp source add <url>                 # Add from URL
ccp source list                      # List installed sources
ccp source update [name]             # Update sources
ccp source remove <name>             # Remove source
```

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
```

Generate default config: `ccp config init`

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
