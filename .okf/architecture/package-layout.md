---
type: Architecture
title: Package layout
description: How ccp's Go code is split between a thin Cobra command layer and domain packages under internal/.
tags: [architecture, go, packages]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: claude-md
    resource: "CLAUDE.md (pre-migration version, 2026-09-14)"
    title: Project CLAUDE.md
---

# Layout

```
internal/
├── config/     # Path resolution, types, CcpConfig (ccp.toml), hook JSON types
├── errors/     # Custom error types
├── hub/        # Hub scanning, item management, settings templates, bundles, item-name validation
├── source/     # Unified source system (providers, registries, installer)
├── profile/    # Profile CRUD, manifest, settings generation, sync, drift, omp mirror
├── symlink/    # Platform-specific symlink operations (unix/windows build tags)
├── migration/  # YAML→TOML and flatten migrations, reset, rollback
└── picker/     # Bubble Tea multi-select TUI with fuzzy search

cmd/            # Cobra commands, one file per command or subcommand
```

# Rules of the layout

| Rule | Detail |
|------|--------|
| Thin `cmd/` | Parse flags, call domain logic, print. No domain logic in commands |
| One file per command | `cmd/<command>.go`, subcommands as `cmd/<parent>_<child>.go` |
| Interfaces for testability | `Scanner`, `Manager`, `Detector` — only where a test needs a seam |
| Platform code | Build tags, e.g. `//go:build !windows` in `internal/symlink` |

Related: [key types](/architecture/key-types.md), [code standards](/engineering/code-standards.md).
