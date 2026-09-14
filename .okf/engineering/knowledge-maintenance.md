---
type: Playbook
title: Knowledge maintenance
description: Where ccp's project knowledge lives after the OKF migration, and the cycle that keeps it true.
tags: [engineering, docs, okf, maintenance]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: claude-md
    resource: "CLAUDE.md, Self-Maintenance (pre-migration version, 2026-09-14)"
    title: Project CLAUDE.md
---

# What lives where

| Location | Holds | Loaded |
|----------|-------|--------|
| `.okf/` | All project knowledge: product, architecture, reference, features, CLI, engineering, decisions, anti-patterns | On demand, from `.okf/index.md` |
| `CLAUDE.md` | Version line, dev commands, a map into `.okf/`, the generated GitNexus block | Every session |
| `.claude/rules/` | Only constraints that must be in context before acting (identity, principles, workflow) | Every session |
| `README.md`, `skills/ccp/SKILL.md` | User-facing docs and agent config; they keep their own conventions | — |

Keep always-loaded files short: detail belongs in the bundle, where it costs context only when read.

# Rules for the bundle

- Load the `okf` skill before reading or writing `.okf/`; run the `validate` skill
  (`--strict`) before calling doc work done.
- Update the bundle in the same change as the code it describes: touched concepts'
  `generated`, the directory `index.md`, and a dated entry in `/log.md`.
- New decisions go in [decisions](/decisions/) and removed approaches in
  [anti-patterns](/anti-patterns/). Both are append-only: supersede with a new concept and a
  status note rather than rewriting history.

# Maintenance cycle

After significant changes (features, refactors, simplifications):

1. **Prune** — remove or deprecate knowledge that no longer applies. Dead knowledge is worse than none.
2. **Consolidate** — merge concepts that say the same thing; keep one source of truth.
3. **Synthesize** — capture new patterns, decisions, and anti-patterns from the work.
4. **Verify** — check that concepts still match the code (types, commands, architecture).

# Signals

- A concept names a type or function that no longer exists.
- A concept describes a removed feature.
- The same guidance appears in `CLAUDE.md`/rules and a concept.
- A new pattern emerged but is written down nowhere.
- A decision's rationale is lost — we know *what* but not *why*.
