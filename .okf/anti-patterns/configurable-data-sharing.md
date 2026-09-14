---
type: Anti-Pattern
title: "Configurable data sharing"
description: "Letting users choose shared vs isolated per data directory type — all data is shared."
tags: [anti-pattern, data]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Let users choose shared vs isolated for each data directory type.

# Instead

All data is shared; no configuration needed ([decision](/decisions/data-sharing-always-shared.md)).
