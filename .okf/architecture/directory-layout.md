---
type: Filesystem Layout
title: The ~/.ccp directory layout
description: What lives under ~/.ccp — hub, store, sources, profiles, shared data — and how each class of data is shared.
tags: [architecture, filesystem, hub, profiles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Tree

```
~/.ccp/
├── hub/                        # Single source of truth, human-configurable
│   ├── skills/<name>/SKILL.md
│   ├── agents/<name>.md
│   ├── hooks/<name>/           # hooks.json + scripts/ (see hooks format)
│   ├── rules/<name>.md
│   ├── commands/
│   ├── settings-templates/<name>/settings.json
│   └── bundles/<name>/         # bundle.yaml + member dirs
├── store/                      # Shared downloadable resources
│   └── plugins/
│       ├── marketplaces/
│       ├── cache/
│       ├── known_marketplaces.json
│       └── install-counts-cache.json
├── sources/                    # Cloned source repositories
├── profiles/
│   ├── shared/                 # Shared runtime data: tasks/, todos/, paste-cache/, projects/
│   └── <name>/                 # One profile = one complete Claude Code config dir
│       ├── profile.toml        # Manifest
│       ├── CLAUDE.md
│       ├── settings.json       # Generated
│       ├── skills/ agents/ hooks/ rules/ commands/  # Relative symlinks → hub
│       ├── plugins/
│       │   ├── marketplaces → store/plugins/marketplaces
│       │   ├── cache → store/plugins/cache
│       │   └── installed_plugins.json   # Per profile
│       └── tasks/ todos/ … → shared/
└── ccp.toml                    # Config + installed sources

~/.claude → .ccp/profiles/<active>   # Global activation symlink (relative)
```

Symlinks are relative so a profile tree survives being synced to another machine.

# Data classification

| Type | Category | Location | Sharing |
|------|----------|----------|---------|
| Hub items (skills, agents, hooks, rules, commands) | Human config | `~/.ccp/hub/` | Linked per profile |
| Plugin cache (marketplaces, cache) | Human config | `~/.ccp/store/plugins/` | Shared via symlinks |
| Runtime data (tasks, todos, history) | Runtime | `~/.ccp/profiles/shared/` | Always shared ([decision](/decisions/data-sharing-always-shared.md)) |
| Plugin state (`installed_plugins.json`) | Runtime | Profile `plugins/` | Isolated |

Related: [profile manifest](/reference/profile-manifest.md), [hooks format](/reference/hooks-format.md),
[activation](/architecture/activation.md).
