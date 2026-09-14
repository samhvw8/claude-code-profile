---
type: Feature
title: Bundles
description: Atomic, non-separable groups of hub items that are linked and removed as one unit.
tags: [feature, hub, bundles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Overview

A bundle groups skills, agents, hooks, rules and commands that only work together — for example
a hook whose command points into its own skill directory. Members live *inside* the bundle
directory, so they can only be linked or removed as a unit. Why:
[the bundles decision](/decisions/bundles-atomic-composite.md).

# Examples

```bash
ccp bundle create <name>                      # interactive composer
ccp bundle create <name> --skill a --hook b   # non-interactive, repeatable flags
ccp bundle list
ccp bundle show <name>
ccp bundle remove <name>

ccp link <profile> bundles/<name>             # expands to per-member symlinks
ccp unlink <profile> bundles/<name>
```

# Storage and linking

| Aspect | Behavior |
|--------|----------|
| Storage | `~/.ccp/hub/bundles/<name>/` holds `bundle.yaml` plus members under `skills/`, `agents/`, `hooks/`, … |
| Composer | **Copies** selected hub items in; originals stay untouched |
| Manifest | Records only the bundle name (`[hub] bundles = [...]`) |
| Linking | Materializes per-member symlinks into the profile's leaf dirs and merges bundle hooks into `settings.json` |
| Type system | Composite: excluded from `config.AllHubItemTypes()` so leaf loops (scan, drift, settings) never treat a bundle as a leaf |
| omp | Members of linkable types resolve inside their bundle directory; a bundle copy of an item the profile also links directly is the same item |

# Member name validation

Member names are validated when a bundle loads (`hub.ValidateItemName`, the same rule the
manifest loader applies): a name must be a relative path that stays inside its item directory.
Names become link names inside the profile and inside each harness, so a name climbing with `..`
would send that write outside the tree ccp owns. A bundle manifest can come from a source
registry, so it is not ccp's own input. Nested names (`group/tone.md`) stay valid.
`ccp bundle show` names the cause when a manifest is unusable.
