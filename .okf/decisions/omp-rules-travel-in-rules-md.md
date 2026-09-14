---
type: Decision
title: "omp support, round 3: rules travel in RULES.md"
description: "Rules are never linked into omp; RULES.md carries all of them, and loadability predictions only produce notes."
tags: [decision, omp, rules]
decided: 2026-09-14
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: decisions-rule
    resource: ".claude/rules/04-key-decisions.md (pre-migration version, 2026-09-14; removed by the OKF migration)"
    title: Key decisions log
    last_modified: 2026-09-14
---

# Decision

ccp stops linking `rules/` into omp entirely. Every profile rule is carried by the `RULES.md` ccp
generates, and the loadability predicate is demoted from gatekeeper to reporter: an item omp is
predicted to ignore is linked anyway and reported, never refused and never removed.

# Why

A second adversarial review followed the first, and every finding reproduced with a real omp
18.1.19 session. One root cause: ccp answered *"does omp load this?"* by predicting another
program's loader, then acted **destructively** on the answer
([anti-pattern](/anti-patterns/predicting-another-programs-loader.md)). Rules were the sharp case,
because `RULES.md` was the only other carrier and the guess decided whether a rule appeared in it —
so a rule whose frontmatter omp does not honor landed in no bucket at all, and a rule whose link was
refused (the path was occupied) was dropped from `RULES.md` while nothing else carried it. Both are
silent content loss; neither produces an error. Rules were the only item type where ccp is the sole
carrier, so the prediction was removed from that path rather than patched again: `RULES.md` carries
all of them, always on, which is also how Claude Code treats a rule. The one hub rule with
frontmatter loaded always anyway, so the token cost is unchanged.

# Fixed (each reproduced before the fix, each with a regression test)

- `rules/` is not a link type; rule links older versions created are retired by any pass, since a
  linked rule plus `RULES.md` would inject it twice.
- Link identity is resolved on **both** sides: `resolveCcpLink` joins the recorded target against
  the link's *resolved* parent. The previous half-fix resolved only what ccp wrote, so under a root
  symlinked to a deeper directory ccp disowned its own links — `list` reported nothing, a second
  sync called the path occupied, `unlink --all` removed nothing. `AGENTS.md` status compares through
  the same helper instead of text.
- One root set for writer, gate and teardown, de-duplicated by resolved path: `ompAgentRoots()` had
  returned the active directory twice and omitted the default one while `OMP_PROFILE` was set, so
  `ccp reset` failed part-way and left the default tree's links dangling while reporting success.
- The opt-in test (`OmpOptedIn`) asks about the tree ccp is about to write into, not every tree it
  ever wrote into: a profile switch no longer starts writing into a tree the user never opted into.
- An unreadable profile rule is skipped and reported instead of aborting the sync; the `RULES.md`
  writer creates its directory first (it failed outright when a profile had rules and no linkable
  item).
- `ccp omp --profile` validates its name (`ValidateOmpProfile`) and errors instead of silently
  falling back to the default tree.
- Deleted with the prediction: `ompLinkedRules`, the rules branch of `ompLoadabilityOf`, rule-name
  flattening (rules were its only user), the flattened-name collision machinery (unreachable without
  flattening), and `ompNormalizeKey`/`ompField`/`ompHasFrontmatterField` (every key ccp reads is one
  word, and one helper was dead code).

# Observation (omp's side, not a bug)

omp lists only some linked skills in its prompt — the development machine's `skills.ignoredSkills`
names 26 of them — and names its agents through tool schemas rather than the prompt. "Not in the
prompt" is not evidence a link is broken, so ccp reports it as information.

# New

`ccp doctor --verify-omp` asks a live omp (`omp --mode rpc --no-session` plus a `get_state`
request) what its system prompt actually carries, and checks the text of every rule and of
`AGENTS.md` against it. It is the only ground truth available, so it exists — but opt-in, because it
needs a configured model and touches omp's own state (no model tokens, ~2 s). Item names are
checked as information only. See [health and verification](/features/omp/health-and-verification.md).

# Why not generate marker-owned rule copies

The alternative was to write normalized copies of every rule into `~/.omp/agent/rules/` with
frontmatter rewritten to the keys omp reads. It keeps per-rule semantics (`condition`, `globs`) but
adds a new class of ccp-owned file needing regeneration, drift checks, teardown coverage and
collision handling — for a semantic no hub rule uses — and it still embeds a belief about which keys
omp reads. Revisit only if omp-only conditional rules are wanted in the hub.

# Found while verifying (fixed in the same pass)

Item names were never validated. A name becomes a path under a type directory and a *link* name
inside a profile and each harness, so a bundle member named `../../../../../ESCAPED` — with the
source it pointed at present — made `ccp omp sync` create a symlink *outside* the agent directory
(reproduced: the link landed in the system temp dir, then removed). A bundle manifest is not ccp's
own input: `ccp source`/`ccp bundle` can pull one from a registry. `hub.ValidateItemName` now
requires a relative path that stays inside its directory, applied in `LoadBundle` and
`LoadManifest` (TOML and YAML); nested names like `group/tone.md` stay valid, since that is how the
scanner flattens rule directories — a stricter "no slashes" rule was caught by the round-trip test
for real manifests. `ccp profile check` also printed *nothing* for `DriftHubMissing`, and
`ccp bundle show` reduced an unusable manifest to "bundle not found": both now name the cause.
