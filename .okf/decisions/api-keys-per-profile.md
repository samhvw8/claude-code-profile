---
type: Decision
title: "API keys are per-profile"
description: "API keys live in each profile; there is no shared account or credentials layer."
tags: [decision, settings, accounts]
decided: 2026-03-30
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

API keys are per-profile (Option A: flat).

# Why

The user has personal and work accounts. Duplicating a key across 2–4 profiles is a non-problem,
and `ccp profile create --from` copies everything, keys included.

# Implication

No "account" concept and no shared credentials layer. A settings template can include API config
or not — the user's choice.
