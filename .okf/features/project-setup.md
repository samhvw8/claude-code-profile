---
type: Feature
title: Project setup
description: Copying hub items into a project's .claude/ directory so a team uses them without ccp.
tags: [feature, project]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Overview

ccp is the authoring tool; `.claude/` is the distribution format. One person runs
`ccp project add`, commits `.claude/`, and the team uses Claude Code without ccp. Items are
**copied**, not symlinked, so the project is self-contained and git-committable.

# Examples

```bash
ccp project add skills/coding agents/reviewer   # copy from the hub
ccp project add -i                              # interactive picker
ccp project install owner/repo skills/my-skill  # install from a source directly
ccp project install owner/repo --all
ccp project install owner/repo -i
ccp project list                                # items in the project's .claude/
ccp project remove skills/coding
```

# Behavior

| Aspect | Behavior |
|--------|----------|
| Project root | Nearest `.git/` walking up from cwd; override with `--dir` |
| Valid types | skills, agents, hooks, rules, commands |
| `project add` | Copies from the local hub |
| `project install` | Fetches from GitHub/skills.sh straight into the project; not tracked in the registry; overwrites existing items |
| Other harnesses | omp loads project `.claude/` by default, and Codex reads project `.agents/skills/`, so project setup already covers them — see [omp overview](/features/omp/overview.md) |
