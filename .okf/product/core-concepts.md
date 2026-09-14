---
type: Concept Model
title: Core concepts and glossary
description: The five user-facing concepts ccp is allowed to have, bundles as a composite hub item, and the glossary.
tags: [product, concepts, glossary]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: identity-rule
    resource: ".claude/rules/01-project-identity.md (pre-migration version, 2026-09-14)"
    title: Project identity rule
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# The five concepts

ccp has a hard budget of five user-facing concepts; adding a sixth requires removing
one ([design principles](/engineering/design-principles.md)).[^identity-rule]

| Concept | What |
|---------|------|
| **Hub** | Central directory of reusable components: skills, agents, hooks, rules, commands |
| **Profile** | Named configuration that references hub items plus an optional settings template |
| **Settings Template** | Complete `settings.json` stored in the hub, referenced by name ([details](/reference/settings-templates.md)) |
| **Source** | External repository providing hub items — GitHub, skills.sh ([details](/features/source-system.md)) |
| **Activation** | How a profile becomes active — `ccp use -g` globally, an env var per project ([details](/architecture/activation.md)) |

# Bundles are a kind of hub item, not a sixth concept

A **bundle** is an atomic group of hub items (skill + agent + hook + …) installed, linked,
and removed together. Members live inside `hub/bundles/<name>/` and are never exposed as
standalone leaf items, so "cannot install separately" is structural rather than a guard.
Bundles are deliberately excluded from `config.AllHubItemTypes()`. See
[bundles](/features/bundles.md) and [the bundles decision](/decisions/bundles-atomic-composite.md).

# Glossary

| Term | Definition |
|------|------------|
| **Hub** | Central repository of reusable Claude Code components |
| **Profile** | Complete Claude Code configuration directory that can be activated |
| **Manifest** | `profile.toml`, which declares what a profile contains ([schema](/reference/profile-manifest.md)) |
| **Drift** | A profile directory that no longer matches its manifest |
| **Settings Template** | Complete `settings.json` in the hub, referenced by name |
| **Hook Type** | Event that triggers a hook — `SessionStart`, `PreToolUse`, … ([format](/reference/hooks-format.md)) |
| **Project Config** | `.ccp.yaml` in a project root for automatic profile selection ([format](/reference/project-config.md)) |
| **Source** | External repository (GitHub, skills.sh) that provides hub items |

[^identity-rule]: Project identity rule
