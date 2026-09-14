---
type: Feature
title: Bootstrap (chezmoi sync)
description: Syncing ccp across machines with chezmoi — pull sources and fix profiles, or push local-only hub items.
resource: cmd/bootstrap.go
tags: [feature, sync, chezmoi]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Examples

```bash
ccp bootstrap          # pull: sync sources, fix all profiles (run after chezmoi apply)
ccp bootstrap --push   # push: add local-only hub items, ccp.toml and profiles to chezmoi
```

**New machine:** `chezmoi apply && ccp bootstrap`
**After changes:** `ccp bootstrap --push`

# Behavior

Source-installed items (tracked in [ccp.toml](/reference/ccp-config.md)) are re-downloaded, not
synced: only local-only hub items, profiles and `ccp.toml` go through chezmoi.

| Step | What pull does |
|------|----------------|
| 1 | Source sync — errors warn and continue |
| 2 | `profile fix --all --force` for every profile |
| 3 | omp: mirror the active profile only when ccp already owns something in the omp tree it would write into; otherwise print a one-line hint ([profile switching](/features/omp/profile-switching.md)) |
