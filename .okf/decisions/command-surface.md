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
Since 2026-11-23 the hidden harness commands (`omp`, `opencode`, `agy`, `pi`, `harness`) are named
in an "Other harnesses" footer of `ccp --help`, built from the `ccp-harness` annotation on each
command; they stay out of the command list and the count.
The current hidden set is listed in the [command reference](/cli/command-reference.md).
