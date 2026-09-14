---
type: Anti-Pattern
title: "CLAUDE.md @import parsing"
description: "Parsing CLAUDE.md for @path imports and creating dual symlinks — users manage their own imports."
tags: [anti-pattern, profiles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Parse `CLAUDE.md` for `@path` references and create dual symlinks.

# Why not

Too much complexity for a niche feature.

# Instead

Users manage their own `@import`s ([v0.28 simplification](/decisions/v0-28-simplification.md)).
