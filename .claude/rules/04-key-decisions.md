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

## Command Surface (2026-03-31)

**Decision:** ~18 visible commands, power-user commands hidden.
**Why:** CLI was approaching git-level surface area for a profile switcher.
**Implication:** Hidden commands still work, just not in `--help`. Don't unhide without strong justification.
