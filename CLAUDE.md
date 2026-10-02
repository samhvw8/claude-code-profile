# ccp - Claude Code Profile Manager

**Current version: v0.48.0**

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
go build -o ccp ./cmd/ccp # Build binary
go test ./...             # Run all tests
go test ./... -v          # Verbose test output
go mod tidy               # Update dependencies
./ccp --help              # Test CLI
```

`.claude/rules/` holds only the always-on constraints; everything else belongs in the bundle
(`.okf/engineering/knowledge-maintenance.md`).
