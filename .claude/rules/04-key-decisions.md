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
