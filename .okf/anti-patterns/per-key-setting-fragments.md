---
type: Anti-Pattern
title: "Per-key setting fragments"
description: "Splitting settings.json into one YAML file per key — use complete settings templates."
tags: [anti-pattern, settings]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Split `settings.json` into individual YAML files per key.

# Instead

Use complete [settings templates](/reference/settings-templates.md)
([decision](/decisions/complete-settings-templates.md)).
