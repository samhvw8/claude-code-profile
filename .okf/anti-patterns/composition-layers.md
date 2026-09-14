---
type: Anti-Pattern
title: "Composition layers"
description: "Indirection between profiles and hub items (engines, contexts, resolvers) — profiles reference hub items directly."
tags: [anti-pattern, architecture]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Add indirection between profiles and hub items: engines, contexts, resolvers.

# Instead

Profiles reference hub items directly. Period. Shared configuration means referencing the same hub
items ([v0.28 simplification](/decisions/v0-28-simplification.md)).
