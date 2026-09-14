---
type: Anti-Pattern
title: "Force-updating git tags"
description: "Moving an existing tag with git tag -f — always increment the version instead."
tags: [anti-pattern, git, release]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

`git tag -f` (or `git push --tags -f`).

# Why not

Force-updating a tag prevents GitHub CI from re-running.

# Instead

Always increment the version and create a new tag ([workflow](/engineering/workflow.md)).
