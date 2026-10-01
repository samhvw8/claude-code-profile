---
type: Feature
title: Bootstrap (multi-machine sync)
description: Setting ccp up on another machine — pull sources and fix profiles after dotfiles land; chezmoi users can also push local-only hub items.
resource: cmd/bootstrap.go
tags: [feature, sync, dotfiles, chezmoi]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-01T03:00:00Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Examples

```bash
ccp bootstrap          # pull: sync sources, fix all profiles (run after your dotfiles land)
ccp bootstrap --push   # chezmoi only: add local-only hub items, ccp.toml and profiles to chezmoi
```

**New machine:** restore dotfiles (`chezmoi apply`, mise dotfiles, stow, ...), then `ccp bootstrap`.
**After changes (chezmoi):** `ccp bootstrap --push`. When `~/.ccp` is linked from a dotfiles repo
(mise dotfiles, stow), there is nothing to push — commit in that repo. Without chezmoi on `PATH`,
`--push` fails with that advice.

# Behavior

Source-installed items (tracked in [ccp.toml](/reference/ccp-config.md)) are re-downloaded, not
synced: only local-only hub items, profiles and `ccp.toml` go through the dotfiles tool.

| Step | What pull does |
|------|----------------|
| 1 | Source sync — errors warn and continue |
| 2 | `profile fix --all --force` for every profile |
| 3 | omp: mirror the active profile only when ccp already owns something in the omp tree it would write into; otherwise print a one-line hint ([profile switching](/features/omp/profile-switching.md)) |
