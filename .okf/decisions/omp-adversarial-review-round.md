---
type: Decision
title: "omp support, adversarial review round"
description: "The first external review's High findings were fixed; state-file ownership and a linked-plugin migration were declined."
tags: [decision, omp, review]
decided: 2026-09-13
superseded_in_part_by: [/decisions/omp-rules-travel-in-rules-md.md]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Status

Superseded in part by [rules travel in RULES.md](/decisions/omp-rules-travel-in-rules-md.md): rules
are no longer linked at all, key normalization was removed with the rules predicate, and the
loadability check became a reporter only.

# Decision

The review's High findings were accepted and fixed; its two structural recommendations
(state-file ownership, linked-plugin migration) were declined with reasons.

# Why

An external review read omp's loader out of the 18.1.19 binary and found that ccp's checks disagreed
with it in seven ways, and that ccp's own two earlier fixes had introduced new faults. Independent
reproduction confirmed every High finding before any code changed.

# Fixed

- A profile switch now *retires the outgoing profile's items* (read from its manifest before the
  symlink moves) instead of only adding: additive follow was worse — omp accumulated every
  profile's items, and `doctor` nagged about them. A link no profile asked for survives a switch and
  is reported as left behind; `sync` is still the reconciling command.
- Loadability tests **values, not key presence**: non-empty `name`/`description`, `alwaysApply` only
  when true, `enabled: false` honored, kebab/underscore keys normalized to camelCase. An unreadable
  item is `ompUnknown` — sync keeps the link and reports it rather than deleting configuration over
  a transient read error.
- Frontmatter parsing learned the shapes real hub content has: BOM, CRLF fences, indented fences
  inside block scalars, and omp's line-based fallback for prose that breaks YAML. Both false
  negatives found in real data (13 agents, 1 CRLF skill) are regression tests.
- `RULES.md` carries only the rules omp cannot load per file — a linked rule is omitted, so nothing
  is injected twice and a conditional (`condition`) rule is not forced always-on.
- Links are created and compared through resolved paths: a dotfiles-managed `~/.omp` no longer
  produces links that do not resolve while status reports healthy. Status also compares a link's
  *target*, so a re-pointed link is drift that `sync` repairs rather than invisible.
- `ccp reset` and `unlink --all` tear down the default agent directory *and* every named omp profile;
  `hooks/` is no longer scanned or pruned at all.
- `ccp bootstrap` mirrors only when ccp already owns something in omp (otherwise it hints), and
  `ccp omp --profile` rejects a name that is not a single path element, so a typo cannot write
  outside the config root.

# Declined

1. Recording ccp-created links in a state file to make ownership exact — the review's own recipe
   ("prune the previous profile's items, test values, resolve paths") restores soundness without new
   persistent state to drift or sync across machines. The residual: a hand-made link into `~/.ccp`
   is indistinguishable from ccp's own, which is documented and only affects `sync`.
2. The linked-omp-plugin migration, for the reasons in
   [the brainstorm outcome](/decisions/omp-link-only-what-omp-loads.md).

# Kept

The non-clobber guarantees, marker-gated atomic `RULES.md`, teardown-after-resetter ordering, the
resetter relative-symlink fix, `OmpAgentDir` precedence, and the command staying hidden.
