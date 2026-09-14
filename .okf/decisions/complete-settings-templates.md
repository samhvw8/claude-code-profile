---
type: Decision
title: "Complete settings templates, not fragments"
description: "A settings template is a whole settings.json; hooks are overlaid from hub hooks."
tags: [decision, settings, templates]
decided: 2026-03-18
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

Settings are complete `settings.json` files, not per-key fragments.

# Why

"What you see is what you get." Fragments required a mental merge; templates are transparent.

# Implication

Hooks are excluded from templates and managed by the hub hooks system: template + hooks overlay =
final `settings.json`. See [settings templates](/reference/settings-templates.md) and the
[per-key fragments anti-pattern](/anti-patterns/per-key-setting-fragments.md).
