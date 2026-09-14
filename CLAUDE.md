# ccp - Claude Code Profile Manager

**Current version: v0.46.0**

Go CLI tool for managing Claude Code profiles via a central hub. Uses Cobra for CLI, go-toml/v2 for
TOML config, gopkg.in/yaml.v3 for YAML, and Bubble Tea for interactive TUI selection.

## Knowledge

Project knowledge lives in the OKF bundle at [`.okf/`](.okf/index.md). Read `.okf/index.md` first and
follow links only into what the task needs. Load the `okf` skill before editing the bundle.

| Need | Go to |
|------|-------|
| What ccp is, concepts, spec, release history | `.okf/product/` |
| Package layout, `~/.ccp` tree, key types, activation | `.okf/architecture/` |
| File formats (`profile.toml`, `ccp.toml`, hooks, templates) | `.okf/reference/` |
| Bundles, project setup, sources, bootstrap, omp | `.okf/features/` |
| Commands and flags | `.okf/cli/` |
| Code standards, workflow, keeping knowledge current | `.okf/engineering/` |
| Why we chose X / what must not return | `.okf/decisions/`, `.okf/anti-patterns/` |

## Development Commands

```bash
go build -o ccp .         # Build binary
go test ./...             # Run all tests
go test ./... -v          # Verbose test output
go mod tidy               # Update dependencies
./ccp --help              # Test CLI
```

`.claude/rules/` holds only the always-on constraints; everything else belongs in the bundle
(`.okf/engineering/knowledge-maintenance.md`).

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
