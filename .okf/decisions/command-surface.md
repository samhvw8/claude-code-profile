---
type: Decision
title: "Command surface: about 18 visible commands"
description: "Keep roughly 18 visible commands and hide power-user commands."
tags: [decision, cli]
decided: 2026-03-31
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

About 18 visible commands; power-user commands are hidden.

# Why

The CLI was approaching git-level surface area for a profile switcher.

# Implication

Hidden commands still work, just not in `--help`. Don't unhide without strong justification.
The current hidden set is listed in the [command reference](/cli/command-reference.md).
