---
type: File Format
title: profile.toml manifest
description: Schema of the per-profile manifest that declares which hub items and settings template a profile uses.
resource: internal/profile/manifest.go
tags: [reference, schema, profiles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
  - id: manifest-go
    resource: internal/profile/manifest.go
    title: Manifest and HubLinks types
    last_modified: 2026-09-14
---

# Schema

| Key | Type | Description |
|-----|------|-------------|
| `version` | int | `3` current; `2` = earlier TOML, `1` = YAML (migrated by `ccp migrate`) |
| `name` | string | Profile name, matches the directory |
| `description` | string | Optional |
| `settings-template` | string | Optional [settings template](/reference/settings-templates.md) name |
| `created`, `updated` | datetime | Timestamps |
| `[hub].skills/agents/hooks/rules/commands` | string list | Hub item names to link |
| `[hub].bundles` | string list | [Bundle](/features/bundles.md) names only; members are not recorded |
| `engine`, `context` | string | Deprecated since v0.28, flattened by migration |

# Examples

```toml
# ~/.ccp/profiles/quickfix/profile.toml
version = 3
name = "quickfix"
description = "Minimal bug-fixing configuration"
settings-template = "opus-full"
created = 2025-01-28T10:00:00Z
updated = 2025-01-28T10:00:00Z

[hub]
skills = ["debugging-core", "git-basics"]
hooks = ["pre-commit-lint"]
rules = ["minimal-change"]
commands = ["quick-test"]
bundles = ["impeccable"]
```

# Validation

Every item name must be a relative path that stays inside its item directory
(`hub.ValidateItemName`), checked when the manifest loads, for both TOML and YAML.[^manifest-go]
Nested names such as `group/tone.md` are valid; `..` escapes are rejected, because a name becomes a
link name inside the profile and inside omp. A manifest with an invalid name fails to load.

There is no `[data]` section: data directories are always shared
([decision](/decisions/data-sharing-always-shared.md)), and old `[data]` sections are ignored.

[^manifest-go]: Manifest and HubLinks types
