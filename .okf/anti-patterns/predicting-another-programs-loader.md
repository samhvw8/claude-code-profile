---
type: Anti-Pattern
title: "Predicting another program's loader on the path that deletes"
description: "Re-implementing another harness's loader and acting destructively on the guess — predictions may only report."
tags: [anti-pattern, omp, harness]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: anti-patterns-rule
    resource: ".claude/rules/05-anti-patterns.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Anti-patterns rule
    last_modified: 2026-09-14
---

# Don't

Answer "does the other harness load this?" by re-implementing that harness's loader, and then *act*
on the guess — refuse a link, remove a link, or omit content from the only file ccp controls.

# Why not

A wrong prediction removes configuration silently: the link looks healthy, the item is never read,
no error is raised, and nothing reports it. Rules were the sharp case, because `RULES.md` was the
only other carrier and the guess decided whether a rule appeared in it
([round 3 decision](/decisions/omp-rules-travel-in-rules-md.md)).

# Instead

- An item omp is predicted to ignore is still linked and reported (a link nobody reads costs
  nothing, and a later omp may read it).
- Only a link the profile does not want is removed.
- For the one item type ccp is solely responsible for, ccp generates the carrier itself instead of
  predicting ([global context](/features/omp/global-context.md)).
- A prediction may stay only as a *reporter*, where a wrong answer costs a line of output.
