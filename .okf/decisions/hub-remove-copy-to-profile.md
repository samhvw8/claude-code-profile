---
type: Decision
title: "Hub remove offers copy-to-profile"
description: "Removing a hub item used by profiles offers copy / delete / cancel instead of a destructive yes/no."
tags: [decision, hub, ux]
decided: 2026-04-15
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

A three-choice prompt (copy / delete / cancel) when removing hub items that profiles use.

# Why

The binary "Remove anyway? [y/N]" was destructive: "y" left profiles with broken symlinks. Users
need a way to keep a profile working after cleaning up the hub.

# Implication

Copy replaces the symlink with local files and removes the item from the profile's `[hub]`
manifest. `--copy` for scripting; `--force` still skips everything. See
[acceptance criteria AC-22](/product/acceptance-criteria.md).
