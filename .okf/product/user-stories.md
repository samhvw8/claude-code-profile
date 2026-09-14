---
type: Requirements
title: User stories
description: The ten user stories ccp was specified against, with their implementation status as of v0.45.
tags: [product, requirements]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
  - id: cmd-tree
    resource: cmd/
    title: Cobra command files (checked for implementation status)
---

# Stories

The spec marked US-4, US-7, US-8, US-9 and US-10 as deferred; each now has a
command in `cmd/`, so their status reads *implemented*.[^cmd-tree]

| ID | As a… | I want to… | So that… | Status |
|----|-------|------------|----------|--------|
| US-1 | user with an existing `~/.claude` | migrate it into a hub + default profile | I can manage profiles without losing my setup | Implemented (`ccp init`) |
| US-2 | user with an initialized hub | create a profile by selecting hub items | I get a purpose-specific configuration | Implemented (`ccp profile create`) |
| US-3 | user with several profiles | add a hub item to an existing profile | profiles evolve over time | Implemented (`ccp link`) |
| US-4 | user who edits profile dirs by hand | check a profile against its manifest | I can detect and fix drift | Implemented (`ccp profile check`, `fix`) |
| US-5 | user on different projects | auto-load profiles per project via mise/direnv | the right config activates by itself | Implemented (`ccp use`, `ccp env`) |
| US-6 | user who wants a fallback profile | set which profile `~/.claude` points to | Claude Code works without env setup | Implemented (`ccp use -g`) |
| US-7 | user uninstalling ccp | reset ccp and restore `~/.claude` | I return to a standard setup | Implemented (`ccp reset`) |
| US-8 | user with broken profiles or symlinks | run diagnostics | I can fix broken configurations | Implemented (`ccp doctor`) |
| US-9 | user switching projects | have profiles selected from project config | I never switch by hand | Implemented (`ccp auto`, hidden) |
| US-10 | user managing many profiles | compare two profiles | I understand what makes each unique | Implemented (`ccp profile diff`, hidden) |

Acceptance criteria for these stories: [acceptance criteria](/product/acceptance-criteria.md).
The end-to-end flows: [user journeys](/product/user-journeys.md).

[^cmd-tree]: Cobra command files
