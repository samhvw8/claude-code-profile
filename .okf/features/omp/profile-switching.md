---
type: Mechanism
title: Following profile switches in omp
description: How ccp use -g and ccp bootstrap keep omp in step with the active profile, and what counts as opting in.
resource: internal/profile/omp.go
tags: [omp, activation, profiles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md, section omp (oh-my-pi) Portability (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
    last_modified: 2026-09-14
---

# Global switch

Once ccp owns anything in omp, `ccp use <profile> -g` mirrors the newly active profile: it links
what the new profile wants and **retires the items the outgoing profile wanted**, read from the
outgoing profile's manifest *before* the symlink moves. A link no profile asked for (made with
`ccp omp link`) is never touched by a switch; it is reported instead:

```
Note: 2 omp link(s) are not in profile 'dev' — 'ccp omp sync' would prune them
```

`ccp omp sync` remains the reconciling command. A failed follow-up warns rather than failing the
switch, because the switch already happened. The migrated product spec still called this follow
"additive… never prunes"; that described an earlier version.

# Opt-in

`profile.OmpFollowProfile()` is a no-op while ccp owns nothing in the tree it would write into.
Opting in is the act of linking *there*. The check covers the context files too, so a profile
whose only trace is `AGENTS.md`/`RULES.md` still follows. `OmpOptedIn()` deliberately asks
only about the current tree: opting in through one named omp profile is not consent to start
writing into the default one.

# Project mode and bootstrap

- Project-scoped `ccp use <profile>` (no `-g`) never touches omp: omp has no per-directory
  user-level config, and its project roots already cover that case.
- [`ccp bootstrap`](/features/bootstrap.md) mirrors the active profile only when ccp already owns
  something in omp, and otherwise prints a one-line hint — writing a global context file into a
  harness the user never linked would silently change every omp session.
