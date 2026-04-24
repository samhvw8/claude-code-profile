# ccp - Claude Code Profile Manager

**Current version: v0.32.2**

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, go-toml/v2 for TOML config, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive TUI selection.

For architecture, types, and command references, see [docs/dev-reference.md](docs/dev-reference.md). For product spec, see [docs/ccp-spec.md](docs/ccp-spec.md).

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

## Self-Maintenance: CLAUDE.md and .claude/rules/

These files are the project's **long-term memory**. Treat them like a dreaming state: periodically consolidate, prune, and synthesize.

### What Lives Where

| File | Purpose | Volatility |
|------|---------|------------|
| `CLAUDE.md` | Coding style, workflow, common tasks | Semi-stable |
| `docs/dev-reference.md` | Types, architecture, command references | Changes with code |
| `docs/ccp-spec.md` | Product spec — problem, solution, UX flows | Changes with features |
| `.claude/rules/01-project-identity.md` | What ccp is, core concepts, what was removed | Stable |
| `.claude/rules/02-design-principles.md` | How to make decisions in this codebase | Stable |
| `.claude/rules/03-workflow.md` | Release flow, testing, CLI patterns | Semi-stable |
| `.claude/rules/04-key-decisions.md` | Decision log with rationale — why we chose X | Append-only |
| `.claude/rules/05-anti-patterns.md` | What failed and must not return | Append-only |

### Maintenance Cycle

After significant changes (new features, refactors, simplifications):

1. **Prune** — Remove rules/docs that no longer apply. Dead knowledge is worse than no knowledge.
2. **Consolidate** — If multiple rules say similar things, merge them. Reduce duplication across CLAUDE.md and rules/.
3. **Synthesize** — Extract new patterns from the work. What did we learn? Add to decisions or anti-patterns.
4. **Verify** — Does CLAUDE.md still match the code? Do the types, commands, and architecture reflect reality?

### Signals That Maintenance Is Needed

- A rule references a type/function that no longer exists
- CLAUDE.md describes a feature that was removed
- The same guidance appears in both CLAUDE.md and a rules file
- A new pattern has emerged but isn't documented anywhere
- Decision rationale is lost (we know *what* but not *why*)
