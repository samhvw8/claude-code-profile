---
type: Principle
title: Design principles
description: How design choices are made in ccp — reuse over abstraction, a five-concept budget, flags over commands, one-function settings generation.
tags: [engineering, design, principles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: principles-rule
    resource: ".claude/rules/02-design-principles.md (pre-migration version, 2026-09-14)"
    title: Design principles rule
---

# Simplicity over abstraction

Reuse existing types and patterns before creating new ones. When the choice is "new type or
reuse", reuse wins by default; propose a new abstraction only when reuse is genuinely impossible.

> "Combine and reduce complex for easy to extend and maintain"

# Complexity budget

| Rule | Consequence |
|------|-------------|
| Max 5 user-facing concepts ([the five](/product/core-concepts.md)) | A sixth requires removing one |
| No processor/builder interfaces for single-path logic | Plain functions ([anti-pattern](/anti-patterns/processor-interface-chains.md)) |
| A feature that can be a flag on an existing command is a flag | No new command |
| Power-user commands may be hidden (`Hidden: true`) | Keeps the visible surface near 18 commands ([decision](/decisions/command-surface.md)) |

# Settings generation is one function

```go
func GenerateSettings(manifest *Manifest, paths *config.Paths, profileDir string) (map[string]interface{}, error)
```

Load the template → overlay hooks → return the settings map. No interfaces
([key types](/architecture/key-types.md)).

# Data is always shared

All data directories (tasks, todos, history, …) are symlinked to `~/.ccp/profiles/shared/`, with
no per-type configuration ([decision](/decisions/data-sharing-always-shared.md)).

# What must not return

The [anti-patterns](/anti-patterns/) list what was tried and removed. Settled
[decisions](/decisions/) are not re-opened without a user request.
