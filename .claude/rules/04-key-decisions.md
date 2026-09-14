# Key Decisions Log

Decisions that future sessions must respect. Do not re-open these without user request.

## v0.28 Simplification (2026-03-31)

**Decision:** Flat profiles, no composition layers.
**Why:** Engine/context system was premature abstraction for a solo-developer tool. The 3-layer resolver was ~2K LOC solving a copy-3-lines problem.
**Implication:** If user wants shared config across profiles, they reference the same hub items. The hub IS the sharing mechanism.

## API Keys / Accounts (2026-03-30)

**Decision:** API keys are per-profile (Option A: flat).
**Why:** User has personal + work accounts. Duplicating a key across 2-4 profiles is a non-problem. `ccp profile create --from` copies everything including keys.
**Implication:** No "account" concept. No shared credentials layer. Settings templates can include API config or not — user's choice.

## Settings Templates (2026-03-18)

**Decision:** Complete settings.json files, not per-key fragments.
**Why:** "What you see is what you get." Fragments required mental merge; templates are transparent.
**Implication:** Hooks are excluded from templates (managed by hub hooks system). Template + hooks overlay = final settings.json.

## Data Sharing (2026-03-31)

**Decision:** All data dirs always shared.
**Why:** 8-mode DataConfig added complexity nobody used. Shared is the right default.
**Implication:** No `[data]` section in profile.toml. Old ones are silently ignored.

## Hub Remove Copy-to-Profile (2026-04-15)

**Decision:** Three-choice prompt (copy/delete/cancel) when removing hub items used by profiles.
**Why:** Binary "Remove anyway? [y/N]" was destructive — choosing "y" left profiles with broken symlinks. Users need a way to keep their profile working after hub cleanup.
**Implication:** Copy replaces symlink with local files and removes item from profile's `[hub]` manifest. `--copy` flag for scripting. `--force` still skips everything.

## Bundles — atomic composite hub items (2026-06-26)

**Decision:** Added a `bundles` hub item type: an atomic group of skills/agents/hooks/rules/commands that links and removes as one unit.
**Why:** Coupled setups (e.g. pbakaus/impeccable, where a hook command points *into* its skill dir) break when their parts are linked separately. `source install` flattens such packages into independently-linkable items with nothing keeping them together.
**Key choices:**
- Bundle = self-contained dir `hub/bundles/<name>/` (Option A). Non-separability is structural — there is no `hub/skills/<member>` to link alone.
- NOT in `config.AllHubItemTypes()` — it is composite; leaf-loops (scan, drift, settings) must not treat it as a leaf. Scanned/linked on its own path.
- Manifest stores only the bundle name (`[hub] bundles`); members materialize as per-member symlinks at link time, so a member cannot be unlinked individually.
- Composer (`ccp bundle create`) **copies** selected hub items in (non-destructive); originals remain. `--move` and `source install --as-bundle` are deliberate follow-ups.
**Implication:** A *kind of hub item*, not a 6th top-level concept — the 5-concept budget is intact. Reuses `hub.ComponentList` for members and `processHooksJSON` for the settings merge.

## Command Surface (2026-03-31)

**Decision:** ~18 visible commands, power-user commands hidden.
**Why:** CLI was approaching git-level surface area for a profile switcher.
**Implication:** Hidden commands still work, just not in `--help`. Don't unhide without strong justification.

## omp Support — Adversarial Review Round (2026-09-13)

**Decision:** the review's High findings were accepted and fixed; its two structural recommendations (state-file ownership, linked-plugin migration) were declined with reasons.
**Why:** an external review read omp's loader out of the 18.1.19 binary and found that ccp's checks disagreed with it in seven ways, and that its own two earlier fixes had introduced new faults. Independent reproduction confirmed every High finding before any code changed.
**Fixed:**
- A profile switch now *retires the outgoing profile's items* (read from its manifest before the symlink moves) instead of only adding: additively was worse — omp accumulated every profile's items, and `doctor` nagged about them. A link no profile asked for survives a switch and is reported as left behind; `sync` is still the reconciling command.
- Loadability tests **values, not key presence**: non-empty `name`/`description`, `alwaysApply` only when true, `enabled: false` honoured, kebab/underscore keys normalized to camelCase. An unreadable item is now `ompUnknown` — sync keeps the link and reports it rather than deleting configuration over a transient read error.
- Frontmatter parsing learned the shapes real hub content has: BOM, CRLF fences, indented fences inside block scalars, and omp's own line-based fallback for prose that breaks YAML. Both of the false negatives found in real data (13 agents, 1 CRLF skill) are regression tests now.
- `RULES.md` carries only the rules omp cannot load per file — a linked rule is omitted, so nothing is injected twice and a conditional (`condition`) rule is not forced always-on.
- Links are created and compared through resolved paths: a dotfiles-managed `~/.omp` no longer produces links that do not resolve while status reports healthy. Status also compares a link's *target*, so a re-pointed link is drift that `sync` repairs rather than invisible.
- `ccp reset` and `unlink --all` tear down the default agent directory *and* every named omp profile; `hooks/` is no longer scanned or pruned at all.
- `ccp bootstrap` mirrors only when ccp already owns something in omp (otherwise it hints), and `ccp omp --profile` rejects a name that is not a single path element, so a typo cannot write outside the config root.
**Declined:** (a) recording ccp-created links in a state file to make ownership exact — the review's own recipe ("prune the previous profile's items, test values, resolve paths") restores soundness without new persistent state to drift or sync across machines; the residual is that a hand-made link into `~/.ccp` is indistinguishable from ccp's own, which is documented and only affects `sync`. (b) the linked-omp-plugin migration, for the reasons already recorded above.
**Kept:** the non-clobber guarantees, marker-gated atomic `RULES.md`, teardown-after-resetter ordering, the resetter relative-symlink fix, `OmpAgentDir` precedence, and the command staying hidden.

## omp Support — Brainstorm Outcome: Link Only What omp Loads (2026-09-13)

*(Superseded in part by the review round above: the predicate is now `ompLoadabilityOf()`, value-based, and `hooks/` is not a link type.)*

**Decision:** `ompItemProblem()` is the single loadability predicate; ccp links only what omp loads, reports the rest with a reason, and removes links that no longer qualify.
**Why:** six independent perspectives (product, minimalism, daily-omp-user, parity, adversarial, omp-native) converged on one thing: links existed for artifacts omp ignores — 6 plain-markdown rules, 5 Claude hooks — and `list` called them healthy. *"Fidelity is unmeasured, so the promise is unverifiable."* A link is not evidence omp loads it.
**Key choices:**
- Predicates: skill needs `description`; agent needs `description` and a `model` other than `inherit` (omp resolves no such model, so the spawn fails); rule needs `description`/`alwaysApply`/`condition` frontmatter; Claude hooks are never linked. Frontmatter parsing follows omp's own permissive fallback (line-based `key: value`) because real hub prose (`Examples: …`) breaks YAML — and the fence must be unindented and CR-tolerant, since real skills ship CRLF.
- Follow is additive: `ccp use -g` links what the new profile wants and never prunes, reporting leftovers instead of deleting them. The adversarial review's top finding was that a by-hand `ccp omp link` vanished on a routine switch; explicit `ccp omp sync` still reconciles. The opt-in check now covers context files too, so a hub-empty profile that set up `AGENTS.md`/`RULES.md` still follows.
- `ccp reset` tears down *after* the resetter succeeds — tearing down first destroyed the integration even when the reset aborted.
- `RULES.md` is written through a temp file + rename (it is injected into every session, so a torn read is a torn system prompt) and its size is reported: "RULES.md (6 rules, 13.4 KB injected every session)".
- `ccp omp --profile <name>` sets `OMP_PROFILE`, because `omp --profile x --alias` never exports it and every sync silently wrote the wrong directory.
- `ccp bootstrap` mirrors the active profile when omp is installed: opt-in is otherwise "the act of linking", which a freshly restored machine never did.
- `ccp omp` is hidden (power-user tier, against a recorded budget of ~18 visible commands vs 24).
- Flattened-name collisions are reported, and a bundle copy of an item the profile also links directly is treated as the same item — not a collision.
**Refused, with reasons:** (a) migrating the mirror to a single linked omp plugin (`omp plugin link`) — its own proposer listed the risks (unrestored target removal, no cross-process locking, makes the `omp` binary a sync prerequisite) and three other perspectives argued to keep the boring table; (b) generating `hooks/pre/ccp-hooks.ts` from Claude `hooks.json` — the daily-user and parity perspectives both called hook translation authoring, not a transform, and a generated module that throws at import takes the session with it; (c) deleting `list`/`link`/the pickers (~300 lines) as the minimalist proposed, since the picker is how a 90-item hub is actually chosen.
**Docs gap closed:** `docs/ccp-spec.md` and `docs/dev-reference.md` documented `ccp codex link/sync/list` and a `[codex]` config section that do not exist (the implementation is parked in `stash@{0}`, repeating the relative-link bug fixed here). The phantom docs are removed; the real source-discovery support stays. The stash is left for its owner to drop.

**Supporting facts (from the original mirror entry):** the mirror exists because omp loads *foreign* user-level config — including `~/.claude` — only when `enabledProviders` opts in, while its own agent directory always loads. Nested hub rule names are flattened to their base name because omp scans `rules/` one level deep (the Claude profile side flattens them too). Links are resolved through `symlink.Manager.Info()`, which handles the relative targets ccp writes — comparing raw `os.Readlink` output against hub paths never matches. `sync` is manifest-driven (bundle members included), so it owns omp's type directories and leaves foreign links and real files alone. Beyond links, `sync` sets up the profile's global context: `AGENTS.md` → the profile's `CLAUDE.md`, and marker-gated `RULES.md` from its rules, which is what makes the CLAUDE.md and rules maintained for Claude Code effective in omp. Named omp profiles are honoured by mirroring omp's own resolution (`OMP_PROFILE`, falling back to `PI_PROFILE`, outranking `PI_CODING_AGENT_DIR`). Two earlier fixes stand: `ccp doctor` counts broken symlinks as issues (it used to print `WARN (n broken)` and still conclude "All checks passed"), and `ccp reset` resolves `~/.claude`'s relative target against the symlink's own directory instead of the working directory.
**Implication:** A hidden 6th top-level command, not a new hub concept — the 5-concept budget is intact.

## omp Support — Round 3: Rules Travel in RULES.md (2026-09-14)

**Decision:** ccp stops linking `rules/` into omp entirely. Every profile rule is carried by the `RULES.md` ccp generates, and the loadability predicate is demoted from gatekeeper to reporter: an item omp is predicted to ignore is linked anyway and reported, never refused and never removed.
**Why:** a second adversarial review followed the first, and every finding reproduced with a real omp 18.1.19 session. One root cause: ccp answered *"does omp load this?"* by predicting another program's loader, then acted **destructively** on the answer. Rules were the sharp case, because `RULES.md` was the only other carrier and the guess decided whether a rule appeared in it — so a rule whose frontmatter omp does not honor landed in no bucket at all, and a rule whose link was refused (the path was occupied) was dropped from `RULES.md` while nothing else carried it. Both are silent content loss; neither produces an error. Rules were the only item type where ccp is the sole carrier, so the prediction was removed from that path rather than patched again: `RULES.md` carries all of them, always-on, which is also how Claude Code treats a rule. The one hub rule with frontmatter loaded always anyway, so the token cost is unchanged.
**Fixed (each reproduced before the fix, each with a regression test):**
- `rules/` is not a link type; rule links older versions created are retired by any pass, since a linked rule plus `RULES.md` would inject it twice.
- Link identity is resolved on **both** sides: `resolveCcpLink` joins the recorded target against the link's *resolved* parent. The previous half-fix resolved only what ccp wrote, so under a root symlinked to a deeper directory ccp disowned its own links — `list` reported nothing, a second sync called the path occupied, `unlink --all` removed nothing. `AGENTS.md` status compares through the same helper instead of text.
- One root set for writer, gate and teardown, de-duplicated by resolved path: `ompAgentRoots()` had returned the active directory twice and omitted the default one while `OMP_PROFILE` was set, so `ccp reset` failed part-way and left the default tree's links dangling while reporting success.
- The opt-in test (`OmpOptedIn`) asks about the tree ccp is about to write into, not every tree it ever wrote into: a profile switch no longer starts writing into a tree the user never opted into.
- An unreadable profile rule is skipped and reported instead of aborting the sync; the `RULES.md` writer creates its directory first (it failed outright when a profile had rules and no linkable item).
- `ccp omp --profile` validates its name (`ValidateOmpProfile`) and errors instead of silently falling back to the default tree.
- Deleted with the prediction: `ompLinkedRules`, the rules branch of `ompLoadabilityOf`, rule-name flattening (rules were its only user), the flattened-name collision machinery (unreachable without flattening), and `ompNormalizeKey`/`ompField`/`ompHasFrontmatterField` (every key ccp reads is one word, and one helper was dead code).
**Observation, omp's side and not a bug:** omp lists only some linked skills in its prompt — the development machine's `skills.ignoredSkills` names 26 of them — and names its agents through tool schemas rather than the prompt. So "not in the prompt" is not evidence a link is broken, and ccp reports it as information.
**New:** `ccp doctor --verify-omp` asks a live omp (`omp --mode rpc --no-session` plus a `get_state` request) what its system prompt actually carries, and checks the text of every rule and of `AGENTS.md` against it. It is the only ground truth available, so it exists — but opt-in, because it needs a configured model and touches omp's own state (no model tokens, ~2 s). Item names are checked as information only.
**Why not "generate marker-owned rule copies" instead:** the alternative was to write normalized copies of every rule into `~/.omp/agent/rules/` with frontmatter rewritten to the keys omp reads. It keeps per-rule semantics (`condition`, `globs`) but adds a new class of ccp-owned file needing regeneration, drift checks, teardown coverage and collision handling — for a semantic no hub rule uses, and it still embeds a belief about which keys omp reads. Revisit only if omp-only conditional rules are wanted in the hub.

**Found while verifying (not from the review, fixed in the same pass):** item names were never validated. A name becomes a path under a type directory and a *link* name inside a profile and inside each harness, so a bundle member named `../../../../../ESCAPED` — with the source it pointed at present — made `ccp omp sync` create a symlink *outside* the agent directory (reproduced: the link landed in the system temp dir, then removed). A bundle manifest is not ccp's own input: `ccp source`/`ccp bundle` can pull one from a registry. `hub.ValidateItemName` now requires a relative path that stays inside its directory, applied in `LoadBundle` (bundles) and `LoadManifest` (manifests, both TOML and YAML); nested names like `group/tone.md` stay valid, since that is how the scanner flattens rule directories — a stricter "no slashes" rule was caught by the round-trip test for real manifests. `ccp profile check` also printed *nothing* for `DriftHubMissing` (an empty drift list, "has configuration drift" with no items) and `ccp bundle show` reduced an unusable manifest to "bundle not found": both now name the cause.
