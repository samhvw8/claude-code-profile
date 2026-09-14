---
type: Decision
title: "All data directories are always shared"
description: "Runtime data directories are always symlinked to profiles/shared, with no per-type configuration."
tags: [decision, data, profiles]
decided: 2026-03-31
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

All data directories are always shared.

# Why

The 8-mode `DataConfig` added complexity nobody used. Shared is the right default.

# Implication

No `[data]` section in `profile.toml`; old ones are silently ignored. See the
[configurable data sharing anti-pattern](/anti-patterns/configurable-data-sharing.md).
