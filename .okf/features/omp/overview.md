---
type: Integration
title: omp integration overview
description: Why ccp mirrors a profile into omp's own agent directory, what the mirror covers, and the ccp omp commands.
resource: internal/profile/omp.go
tags: [omp, integration, harness]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md, section omp (oh-my-pi) Portability (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
    last_modified: 2026-09-14
---

# Why a mirror exists

omp (oh-my-pi) reads its own user config from `~/.omp/agent/` (skills, agents, rules, commands,
hooks, tools, settings). It loads *foreign* user-level config — including `~/.claude` — only
when its `enabledProviders` setting opts in, and that defaults to empty. A ccp profile at
`~/.claude` is therefore invisible to omp. ccp links hub items into omp's own directories,
which omp always loads.

# What the mirror covers

| Profile content | In omp | Concept |
|-----------------|--------|---------|
| skills, agents, commands | Linked into `~/.omp/agent/<type>/` | [linking](/features/omp/linking.md) |
| rules | Carried in a generated `RULES.md`, never linked | [global context](/features/omp/global-context.md) |
| `CLAUDE.md` | `AGENTS.md` symlink | [global context](/features/omp/global-context.md) |
| hooks, settings templates | Not mirrored — different mechanism and schema | [linking](/features/omp/linking.md) |

Project scope needs no mirror: omp's `enabledProviders` gate covers only foreign *user-level*
roots, and project roots (`<project>/.claude/`, `<project>/.omp/`) load by default. A project
set up with [`ccp project add`](/features/project-setup.md) is already visible to omp there —
verified with a project-level `.claude/skills/` skill. `ccp omp` covers the user-level half.

# Commands

`ccp omp` is hidden (power-user tier). Full table: [CLI reference](/cli/command-reference.md).

```bash
ccp omp link skills/debugging commands/ship.md  # link hub items into omp
ccp omp link                                    # interactive picker
ccp omp sync                                    # mirror the active profile
ccp omp sync dev                                # mirror a named profile
ccp omp list                                    # links and diff vs the active profile
ccp omp unlink skills/debugging                 # remove ccp links
ccp omp unlink                                  # interactive picker
ccp omp unlink --all                            # remove everything ccp set up
ccp omp --profile work sync                     # target an omp named profile
ccp doctor --verify-omp                         # ask a live omp what its prompt carries
```

See also: [profile switching](/features/omp/profile-switching.md),
[health and verification](/features/omp/health-and-verification.md),
[teardown and path resolution](/features/omp/teardown-and-paths.md), and the decision trail in
[decisions](/decisions/).
