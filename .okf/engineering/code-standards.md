---
type: Convention
title: Code standards
description: Go conventions, CLI patterns, file organization, and testing conventions for ccp.
tags: [engineering, go, cli, testing]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: claude-md
    resource: "CLAUDE.md, Code Standards (pre-migration version, 2026-09-14)"
    title: Project CLAUDE.md
  - id: workflow-rule
    resource: ".claude/rules/03-workflow.md (pre-migration version)"
    title: Workflow rule
  - id: principles-rule
    resource: ".claude/rules/02-design-principles.md (pre-migration version)"
    title: Design principles rule
---

# Go

| Convention | Detail |
|------------|--------|
| Formatting | Standard `gofmt` |
| Errors | Returned, never panicked |
| Testability | Interfaces (`Scanner`, `Manager`, `Detector`) only where a test needs a seam |
| Platform code | Build tags, e.g. `//go:build !windows` for unix/windows symlink variants |
| Symlinks | Relative targets, for cross-machine portability |

# CLI

| Pattern | Detail |
|---------|--------|
| Naming | Verb-noun: `profile create`, `hub list` |
| Files | `cmd/root.go` root + version; `cmd/<command>.go` top-level; `cmd/<parent>_<child>.go` subcommands |
| Flags | `--long-form`, with `-s` short aliases where useful |
| Output | `tabwriter` for aligned tables, `fmt` for simple lines; `--json` on list commands |
| Errors | Returned to Cobra; exit 1 on failure |
| Layering | Keep `cmd/` thin; domain logic in `internal/<domain>/` ([package layout](/architecture/package-layout.md)) |

# Tests

| Convention | Detail |
|------------|--------|
| Style | Table-driven tests preferred |
| Filesystem | `t.TempDir()` |
| Location | `*_test.go` next to the implementation |
| macOS paths | Compare symlink targets through `filepath.EvalSymlinks()` (`/var` → `/private/var`) |
| Coverage targets | profile 60%+, hub 55%+, migration 65%+ |
