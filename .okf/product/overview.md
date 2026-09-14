---
type: Product Overview
title: ccp product overview
description: The problem ccp solves, its solution, explicit non-goals, assumptions, and deferred questions.
tags: [product, spec]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration; final version in commit 1ab965c)"
    title: ccp product specification
    last_modified: 2026-09-14
---

# Problem

Claude Code's 20-skill limit forces users to reconfigure by hand for each work mode
(development, quick-fix, documentation). Nothing saves, switches, or shares a
configuration, and duplicating skills across setups creates drift.[^ccp-spec]

| Signal | Assessment |
|--------|------------|
| Frequency | Daily — every project or task switch |
| Severity | High — tedious, error-prone, and context-window bloat degrades Claude Code |
| Market | Repeated requests in Claude Code GitHub issues, no official solution planned |

# Solution

A local CLI (`ccp`) that manages a central **hub** of reusable components and many
**profiles**. Each profile is a complete Claude Code configuration directory, activated
through `CLAUDE_CONFIG_DIR` or the `~/.claude` symlink. Hub items are symlinked into
profiles, so every item has one source of truth. See [core concepts](/product/core-concepts.md)
and [activation](/architecture/activation.md).

# Non-goals

| ccp does not | Rationale |
|--------------|-----------|
| Provide a web UI or registry | The hub is local filesystem only; community sharing is Phase 2+ |
| Share profiles across machines by itself | No cloud; [bootstrap](/features/bootstrap.md) delegates to chezmoi |
| Auto-detect project type | Profile selection is manual or via mise/direnv/`.ccp.yaml` |
| Support profile inheritance/extends | Flat composition only — see [the v0.28 simplification](/decisions/v0-28-simplification.md) |
| Enforce the 20-skill limit | The user's responsibility; ccp may warn but never blocks |
| Manage Claude Code internals | `cache/`, `debug/`, `telemetry/`, `statsig/`, `ide/` are ignored |
| Version-control hub items | Users put their own dirs in git if they want |
| Author skills or hooks | ccp organizes items, it does not write them |
| Resolve conflicts automatically | Broken symlinks are reported, then fixed on request |
| Merge settings.json files | Copy/template only — see [settings templates](/reference/settings-templates.md) |

# Assumptions and dependencies

| Assumption | Impact if wrong | Mitigation |
|------------|-----------------|------------|
| `CLAUDE_CONFIG_DIR` works with arbitrary paths | Core activation breaks | Fall back to swapping the symlink |
| Claude Code resolves symlinks for skills/hooks | Hub linking breaks | Copy instead of symlink (more maintenance) |
| User has mise or direnv for auto-activation | Manual export needed | Document manual activation |
| `~/.claude` layout is stable across versions | Migration logic may break | Version detection plus migration paths |
| Symlinks work on the user's OS | Windows users may struggle | Document Windows symlink requirements or use junctions |

# Open questions (deferred)

1. **Profile templates** — predefined starter profiles (minimal, full-stack, docs-writer)?
2. **Hub item metadata** — should hub items carry their own manifest (description, tags, dependencies)?
3. **Dependency resolution** — if skill A requires hook B, should ccp auto-link? ([Bundles](/features/bundles.md) cover the coupled case.)
4. **Profile export/import** — package a profile for sharing without the hub?

[^ccp-spec]: ccp product specification
