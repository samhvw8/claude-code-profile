---
type: Decision
title: "Bundles are atomic composite hub items"
description: "A bundles hub item type groups coupled items that link and remove as one unit, without becoming a sixth concept."
tags: [decision, hub, bundles]
decided: 2026-06-26
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

Add a `bundles` hub item type: an atomic group of skills/agents/hooks/rules/commands that links
and removes as one unit.

# Why

Coupled setups (e.g. pbakaus/impeccable, where a hook command points *into* its skill directory)
break when their parts are linked separately. `source install` flattens such packages into
independently linkable items with nothing keeping them together.

# Key choices

- A bundle is a self-contained directory `hub/bundles/<name>/` (Option A). Non-separability is
  structural: there is no `hub/skills/<member>` to link alone.
- NOT in `config.AllHubItemTypes()` — it is composite; leaf loops (scan, drift, settings) must not
  treat it as a leaf. It is scanned and linked on its own path.
- The manifest stores only the bundle name (`[hub] bundles`); members materialize as per-member
  symlinks at link time, so a member cannot be unlinked individually.
- The composer (`ccp bundle create`) **copies** selected hub items in; originals remain.
  `--move` and `source install --as-bundle` are deliberate follow-ups.

# Implication

A *kind of hub item*, not a sixth top-level concept — the 5-concept budget is intact. Reuses
`hub.ComponentList` for members and `processHooksJSON` for the settings merge. See
[bundles](/features/bundles.md).
