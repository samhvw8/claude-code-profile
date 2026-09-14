---
type: Requirements
title: User journeys
description: Step-by-step flows for first-time setup, creating a purpose-specific profile, and fixing drift.
tags: [product, requirements, ux]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Journey 1: first-time setup

Paths corrected from the spec's pre-v0.5 `~/.claude/hub` to `~/.ccp/hub`.

| Step | Actor | Action | System response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp init` | — |
| 2 | System | Scans `~/.claude` | Identifies hub-eligible items |
| 3 | System | Presents migration plan | "Moving 31 skills, 5 hooks, 3 rules to hub" |
| 4 | User | Confirms | — |
| 5 | System | Creates hub structure | `~/.ccp/hub/` populated |
| 6 | System | Creates default profile | `~/.ccp/profiles/default/` with symlinks |
| 7 | System | Creates shared namespace | `~/.ccp/profiles/shared/` |
| 8 | System | Outputs guidance | Shell configuration instructions |

**Success:** Claude Code works exactly as before, but the hub/profile structure exists.

**Failure paths:** `~/.claude` missing → error with setup instructions; permission denied →
error naming the path; partial migration fails → roll back and report which items failed.

# Journey 2: create a purpose-specific profile

| Step | Actor | Action | System response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp profile create quickfix` | — |
| 2 | System | Prompts for hub items | Interactive picker, or reads flags |
| 3 | User | Selects skills, hooks, rules | — |
| 4 | System | Creates profile directory | Full structure with symlinks |
| 5 | System | Generates `profile.toml` | Manifest of the selection |
| 6 | System | Outputs activation instructions | `CLAUDE_CONFIG_DIR` example |

**Success:** the profile is ready and activates via `CLAUDE_CONFIG_DIR`.

**Failure paths:** hub not initialized → "Run ccp init first"; name exists → error or
`--force`; selected item missing → error listing available items.

# Journey 3: validate and fix drift

| Step | Actor | Action | System response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp profile check quickfix` | — |
| 2 | System | Reads `profile.toml` | Parses the manifest |
| 3 | System | Compares against the directory | Detects differences |
| 4 | System | Reports drift | Missing, extra, broken, mismatched |
| 5 | User | Runs `ccp profile fix quickfix` | — |
| 6 | System | Reconciles the directory | Creates/removes symlinks |
| 7 | System | Reports changes | List of actions taken |

**Success:** the profile directory matches its manifest exactly.

Related: [user stories](/product/user-stories.md), [acceptance criteria](/product/acceptance-criteria.md).
