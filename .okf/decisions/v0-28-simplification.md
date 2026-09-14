---
type: Decision
title: "v0.28 simplification: flat profiles"
description: "Profiles are flat — no engine/context composition layers; the hub is the sharing mechanism."
tags: [decision, architecture, simplification]
decided: 2026-03-31
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

Flat profiles, no composition layers.

# Why

The engine/context system was premature abstraction for a solo-developer tool. The 3-layer
resolver was ~2K LOC solving a copy-3-lines problem.

# Implication

If the user wants shared config across profiles, they reference the same hub items. The hub
*is* the sharing mechanism.

# What was removed

Do not re-introduce any of these ([anti-patterns](/anti-patterns/)):

| Removed | Replaced by |
|---------|-------------|
| **Engines** — two-layer runtime config | Flat profiles |
| **Contexts** — two-layer prompt/capability config | Flat profiles |
| **Setting fragments** — per-key YAML files | [Settings templates](/decisions/complete-settings-templates.md) |
| **Linked dirs** — CLAUDE.md `@import` parsing + dual symlinks | Users manage `@import`s themselves |
| **DataConfig** — per-type sharing mode | [Always-shared data](/decisions/data-sharing-always-shared.md) |
| **Processor interfaces** — SettingsBuilder/TemplateProcessor/FragmentProcessor | One `GenerateSettings()` function |

Net effect of the release: ~6K LOC removed, concepts reduced from 14 to 5
([release history](/product/release-history.md)).
