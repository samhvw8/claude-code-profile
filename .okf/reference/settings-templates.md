---
type: Feature
title: Settings templates and fragment capture
description: Complete settings.json files stored in the hub and referenced by name, plus capturing manual edits as a per-profile fragment.
tags: [reference, settings, templates]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-01T08:00:00Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Overview

A settings template is a complete `settings.json` in the hub; a profile names one. What you see is
what you get — no per-key merging ([decision](/decisions/complete-settings-templates.md)).
Hooks are always overlaid from hub hooks, never stored in a template
([hooks format](/reference/hooks-format.md)).

Storage: `~/.ccp/hub/settings-templates/<name>/settings.json`

# Examples

```bash
ccp template list                             # list templates
ccp template show <name>                      # print template JSON
ccp template create <name>                    # $EDITOR, or --from-file <path>
ccp template extract <name> --from <profile>  # from a profile's settings
ccp template edit <name>
ccp template delete <name>

ccp profile create <name> --template opus-full
ccp profile edit <name> --template minimal
```

# Fragment capture

Manual edits to a profile's `settings.json` survive regeneration once captured:

```bash
ccp profile capture              # active profile
ccp profile capture dev          # named profile
ccp profile capture --dry-run    # show the diff only
```

It computes `DiffSettings(base_template, current_settings)`, strips hooks, and saves only the keys
that differ from the template as `settings-fragment.json`. Without a template, every non-hook key
becomes the fragment; with no diff, a stale fragment file is removed. A key the template has but `settings.json`
no longer does is recorded as `null` (at any depth), and regeneration reads a fragment `null` as
"remove this key" — so dropping a plugin from the template's `enabledPlugins` can be captured too.

# When settings.json is rewritten

| Trigger | What changes |
|---------|--------------|
| `ccp link`/`unlink` of a hook or bundle, `hub add`/`hub link`/`hub remove` touching hooks | Only the `hooks` key (`SyncHooks`); every other key stays as on disk |
| `ccp use`, `ccp profile edit`, `ccp profile sync` | Full regeneration (template → fragment → hooks), unless it would drop an uncaptured edit |
| `ccp profile sync --force` | Full regeneration, discarding uncaptured edits |

An **uncaptured edit** is a top-level non-hook key that regeneration would lose: changed, added or
deleted in `settings.json` since ccp last wrote it — typically by hand or by Claude Code (`/model`,
plugin toggles, permission grants) — and not reproduced by the template and fragment
(`UncapturedSettings`). ccp knows what it last wrote from a snapshot,
`~/.ccp/state/settings/<profile>.json`, so a template switch or a fragment edit is never mistaken for an edit, and a
captured edit stops counting because the fragment now reproduces it.

The snapshot is machine-local and lives outside the profile, so dotfiles never carry it
([decision](/decisions/settings-snapshot.md)); `profile rename` moves it and `profile delete`
removes it. Every write of `settings.json` through ccp's settings generation creates or refreshes it; a profile written by an older ccp, or freshly restored on a
new machine, has none until then, and meanwhile any on-disk key that regeneration would change
counts as uncaptured.

When there is an uncaptured edit, `ApplySettings` rewrites hooks only and ccp names the keys, with
two ways out: `ccp profile capture` then sync, or sync with `--force`. Sync never prompts, so it
behaves the same in a pipe.

Old-style manifests that list hooks under `[[hooks]]` (`manifest.Hooks`) are generated alongside
hub hooks, so no rewrite drops them.

# Hooks ccp did not create

Hook entries added by hand or with Claude Code's `/hooks` survive every rewrite, `--force`
included. ccp owns an entry when its command points into the profile's `hooks/` directory or the
hub's `hooks/` or `bundles/` (where `ccp init` and migrations point), when ccp wrote it last time
(per the snapshot), or when any hub hook, bundle hook or old-style manifest hook would generate it.
It replaces those and keeps the rest after them.

# Nulls and unreadable files

ccp never writes `null` into `settings.json`: a fragment `null` removes the key, and nulls nested in
a fragment value are dropped. A `null` on disk is treated as an absent key when looking for
uncaptured edits, matching how Claude Code reads it.

An unparseable `settings.json` stops every command that would merge into it, with a message saying
how to fix it. `ccp profile sync --force` regenerates it and keeps the old file as
`settings.json.bak`.
