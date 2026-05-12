# ccp - Claude Code Profile Manager

**Current version: v0.38.0**

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

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **claude-code-profile** (4226 symbols, 11890 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/claude-code-profile/context` | Codebase overview, check index freshness |
| `gitnexus://repo/claude-code-profile/clusters` | All functional areas |
| `gitnexus://repo/claude-code-profile/processes` | All execution flows |
| `gitnexus://repo/claude-code-profile/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
