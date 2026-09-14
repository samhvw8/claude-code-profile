---
type: Anti-Pattern
title: "Processor interface chains"
description: "Interfaces like TemplateProcessor or SettingsBuilder for what is load JSON, add hooks, write file — use one function."
tags: [anti-pattern, architecture, go]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Create interfaces (`TemplateProcessor`, `FragmentProcessor`, `HookProcessor`, `SettingsBuilder`)
for what is fundamentally "load JSON, add hooks, write file."

# Instead

A single function: `GenerateSettings()` ([key types](/architecture/key-types.md),
[design principles](/engineering/design-principles.md)).
