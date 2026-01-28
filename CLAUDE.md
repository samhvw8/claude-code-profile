# ccp - Claude Code Profile Manager

## Project Context

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive TUI selection.

## Architecture

```
internal/
├── config/     # Path resolution, types (HubItemType, DataItemType, ShareMode)
├── errors/     # Custom error types (ProfileError, HubError, DriftError)
├── hub/        # Hub scanning and item management
├── profile/    # Profile CRUD, manifest (profile.yaml), drift detection
├── symlink/    # Platform-specific symlink operations (unix/windows)
├── migration/  # Init migration with rollback support
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
- `cmd/<command>.go` - Top-level commands (init, use, link, unlink)
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

type HubItemType string    // skills, agents, hooks, rules, commands, md-fragments
type DataItemType string   // tasks, todos, history, etc.
type ShareMode string      // shared, isolated

// internal/profile/manifest.go
type Manifest struct {
    Name, Description string
    Created, Updated  time.Time
    Hub               HubLinks    // What hub items to link
    Data              DataConfig  // Shared vs isolated data dirs
}
```

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
