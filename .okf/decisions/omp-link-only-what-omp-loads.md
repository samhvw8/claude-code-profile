---
type: Decision
title: "omp support, brainstorm outcome: link only what omp loads"
description: "The first omp design — a single loadability predicate gating links, additive follow, and the refused alternatives."
tags: [decision, omp]
decided: 2026-09-13
superseded_in_part_by: [/decisions/omp-adversarial-review-round.md, /decisions/omp-rules-travel-in-rules-md.md]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Status

Superseded in part: the predicate became the value-based, reporter-only `ompLoadabilityOf()`;
rules are no longer linked; follow retires the outgoing profile's items; bootstrap is gated on
opt-in; `hooks/` is not a link type. See [the adversarial review round](/decisions/omp-adversarial-review-round.md)
and [rules travel in RULES.md](/decisions/omp-rules-travel-in-rules-md.md). Current behavior:
[omp integration](/features/omp/overview.md).

# Decision

`ompItemProblem()` is the single loadability predicate; ccp links only what omp loads, reports the
rest with a reason, and removes links that no longer qualify.

# Why

Six independent perspectives (product, minimalism, daily-omp-user, parity, adversarial,
omp-native) converged on one thing: links existed for artifacts omp ignores — 6 plain-markdown
rules, 5 Claude hooks — and `list` called them healthy. *"Fidelity is unmeasured, so the promise
is unverifiable."* A link is not evidence omp loads it.

# Key choices

- Predicates: a skill needs `description`; an agent needs `description` and a `model` other than
  `inherit` (omp resolves no such model, so the spawn fails); a rule needs
  `description`/`alwaysApply`/`condition` frontmatter; Claude hooks are never linked. Frontmatter
  parsing follows omp's permissive fallback (line-based `key: value`) because real hub prose
  (`Examples: …`) breaks YAML — and the fence must be unindented and CR-tolerant, since real
  skills ship CRLF.
- Follow is additive: `ccp use -g` links what the new profile wants and never prunes, reporting
  leftovers instead of deleting them. The adversarial review's top finding was that a by-hand
  `ccp omp link` vanished on a routine switch; explicit `ccp omp sync` still reconciles. The opt-in
  check covers context files too, so a hub-empty profile that set up `AGENTS.md`/`RULES.md` still
  follows.
- `ccp reset` tears down *after* the resetter succeeds — tearing down first destroyed the
  integration even when the reset aborted.
- `RULES.md` is written through a temp file + rename (it is injected into every session, so a torn
  read is a torn system prompt), and its size is reported: "RULES.md (6 rules, 13.4 KB injected
  every session)".
- `ccp omp --profile <name>` sets `OMP_PROFILE`, because `omp --profile x --alias` never exports it
  and every sync silently wrote the wrong directory.
- `ccp bootstrap` mirrors the active profile when omp is installed: opt-in is otherwise "the act of
  linking", which a freshly restored machine never did.
- `ccp omp` is hidden (power-user tier, against a recorded budget of ~18 visible commands vs 24).
- Flattened-name collisions are reported, and a bundle copy of an item the profile also links
  directly is treated as the same item — not a collision.

# Refused, with reasons

1. Migrating the mirror to a single linked omp plugin (`omp plugin link`) — its own proposer listed
   the risks (unrestored target removal, no cross-process locking, makes the `omp` binary a sync
   prerequisite) and three other perspectives argued to keep the boring table.
2. Generating `hooks/pre/ccp-hooks.ts` from Claude `hooks.json` — the daily-user and parity
   perspectives both called hook translation authoring, not a transform, and a generated module
   that throws at import takes the session with it.
3. Deleting `list`/`link`/the pickers (~300 lines) as the minimalist proposed, since the picker is
   how a 90-item hub is actually chosen.

# Docs gap closed

The spec and developer reference documented `ccp codex link/sync/list` and a `[codex]` config
section that do not exist (the implementation is parked in `stash@{0}`, repeating the relative-link
bug fixed here). The phantom docs were removed; the real source-discovery support stays. The stash
is left for its owner to drop.

# Supporting facts

- The mirror exists because omp loads *foreign* user-level config — including `~/.claude` — only
  when `enabledProviders` opts in, while its own agent directory always loads.
- Nested hub rule names were flattened to their base name because omp scans `rules/` one level
  deep (the Claude profile side flattens them too).
- Links are resolved through `symlink.Manager.Info()`, which handles the relative targets ccp
  writes — comparing raw `os.Readlink` output against hub paths never matches.
- `sync` is manifest-driven (bundle members included), so it owns omp's type directories and leaves
  foreign links and real files alone.
- Beyond links, `sync` sets up the profile's global context: `AGENTS.md` → the profile's
  `CLAUDE.md`, and marker-gated `RULES.md` from its rules — which makes the CLAUDE.md and rules
  maintained for Claude Code effective in omp.
- Named omp profiles are honored by mirroring omp's own resolution (`OMP_PROFILE`, falling back to
  `PI_PROFILE`, outranking `PI_CODING_AGENT_DIR`).
- Two earlier fixes stand: `ccp doctor` counts broken symlinks as issues (it used to print
  `WARN (n broken)` and still conclude "All checks passed"), and `ccp reset` resolves `~/.claude`'s
  relative target against the symlink's own directory instead of the working directory.

# Implication

A hidden sixth top-level command, not a new hub concept — the 5-concept budget is intact.
