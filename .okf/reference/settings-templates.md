---
type: Feature
title: Settings templates and fragment capture
description: Complete settings.json files stored in the hub and referenced by name, plus capturing manual edits as a per-profile fragment.
tags: [reference, settings, templates]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
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
becomes the fragment; with no diff, a stale fragment file is removed.
