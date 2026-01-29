# ccp Implementation Plan - COMPLETED

## Status: Implemented

All 6 phases completed successfully.

## Summary

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive selection.

## Project Structure

```
ccp/
├── main.go
├── go.mod
├── cmd/                         # Cobra commands
│   ├── root.go
│   ├── init.go
│   ├── use.go
│   ├── profile.go               # Parent command
│   ├── profile_create.go
│   ├── profile_list.go
│   ├── profile_check.go
│   ├── profile_fix.go
│   ├── profile_delete.go
│   ├── link.go
│   ├── unlink.go
│   └── hub.go
├── internal/
│   ├── config/                  # Path resolution
│   │   └── paths.go
│   ├── hub/                     # Hub domain logic
│   │   ├── hub.go
│   │   └── scanner.go
│   ├── profile/                 # Profile domain logic
│   │   ├── profile.go
│   │   ├── manifest.go
│   │   └── drift.go
│   ├── symlink/                 # Platform-specific symlinks
│   │   ├── symlink.go
│   │   ├── symlink_unix.go
│   │   └── symlink_windows.go
│   ├── migration/               # Init migration
│   │   ├── migrator.go
│   │   └── rollback.go
│   ├── picker/                  # Interactive TUI
│   │   └── picker.go
│   └── errors/                  # Custom error types
│       └── errors.go
└── docs/
    └── ccp-spec.md
```

## Dependencies

```go
github.com/spf13/cobra v1.9.1           // CLI framework
gopkg.in/yaml.v3 v3.0.1                 // YAML parsing
github.com/charmbracelet/bubbletea      // Interactive TUI
github.com/charmbracelet/bubbles        // TUI components
github.com/charmbracelet/lipgloss       // TUI styling
```

## Implementation Phases

### Phase 1: Foundation
1. Project scaffold - `go.mod`, `main.go`, `cmd/root.go`
2. `internal/config/paths.go` - Path resolution (~/.claude, hub, profiles)
3. `internal/errors/errors.go` - Custom error types
4. `internal/symlink/` - Platform-specific symlink operations

### Phase 2: Core Domain
1. `internal/hub/hub.go` - Hub type, item types (skills, rules, hooks, etc.)
2. `internal/hub/scanner.go` - Directory scanning for hub items
3. `internal/profile/manifest.go` - profile.yaml parsing/writing
4. `internal/profile/profile.go` - Profile type and operations

### Phase 3: Read Commands
1. `cmd/hub.go` - `ccp hub list [type]`
2. `cmd/profile_list.go` - `ccp profile list`
3. `cmd/use.go` - `ccp use --show`
4. `cmd/profile_check.go` - `ccp profile check <name>`

### Phase 4: Write Commands
1. `internal/migration/migrator.go` - Migration logic with rollback
2. `cmd/init.go` - `ccp init`
3. `cmd/use.go` - `ccp use <profile>` (symlink swap)
4. `cmd/link.go` - `ccp link <profile> <path>`
5. `cmd/unlink.go` - `ccp unlink <profile> <path>`

### Phase 5: Interactive & Advanced
1. `internal/picker/picker.go` - Bubble Tea multi-select
2. `cmd/profile_create.go` - `ccp profile create` (interactive mode)
3. `internal/profile/drift.go` - Drift detection logic
4. `cmd/profile_fix.go` - `ccp profile fix <name>`
5. `cmd/profile_delete.go` - `ccp profile delete <name>`

### Phase 6: Polish
1. Shell completions (Cobra built-in)
2. Integration tests
3. README and documentation

## Core Types

```go
// Manifest (profile.yaml)
type Manifest struct {
    Name        string    `yaml:"name"`
    Description string    `yaml:"description"`
    Created     time.Time `yaml:"created"`
    Updated     time.Time `yaml:"updated"`
    Hub         HubLinks  `yaml:"hub"`
    Data        DataConfig `yaml:"data"`
}

// HubLinks - what hub items to symlink
type HubLinks struct {
    Skills      []string `yaml:"skills,omitempty"`
    Hooks       []string `yaml:"hooks,omitempty"`
    Rules       []string `yaml:"rules,omitempty"`
    Commands    []string `yaml:"commands,omitempty"`
    MdFragments []string `yaml:"md-fragments,omitempty"`
}

// DataConfig - shared vs isolated
type DataConfig struct {
    Tasks       ShareMode `yaml:"tasks"`       // shared/isolated
    Todos       ShareMode `yaml:"todos"`
    History     ShareMode `yaml:"history"`
    // ... etc
}
```

## Key Interfaces (for testability)

```go
type HubScanner interface {
    Scan(basePath string) (*Hub, error)
}

type ProfileManager interface {
    Create(name string, manifest *Manifest) (*Profile, error)
    Get(name string) (*Profile, error)
    List() ([]*Profile, error)
    Delete(name string) error
}

type SymlinkManager interface {
    Create(source, target string) error
    Remove(path string) error
    Validate(path string, expectedTarget string) (bool, error)
}

type DriftDetector interface {
    Detect(profile *Profile) (*DriftReport, error)
    Fix(profile *Profile, report *DriftReport, dryRun bool) error
}
```

## Critical Files

- `/home/samhoang/workspace/ccp/docs/ccp-spec.md` - Full specification

## Verification

1. **Unit tests**: Run `go test ./...`
2. **Manual testing**:
   - `ccp init` on a test ~/.claude directory
   - `ccp profile create test-profile`
   - `ccp use test-profile`
   - `ccp profile check test-profile`
3. **Integration**: Test symlink resolution with actual Claude Code
