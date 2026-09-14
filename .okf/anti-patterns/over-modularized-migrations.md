---
type: Anti-Pattern
title: "Over-modularized migrations"
description: "A separate file for every migration type — keep migration code minimal and delete completed migrations."
tags: [anti-pattern, migration]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Create a separate file for every migration type.

# Instead

Keep migration code minimal, and delete completed migrations after a few versions.
