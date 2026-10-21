---
type: Feature
title: Source system
description: Finding and installing hub items from skills.sh and GitHub, including the repository layouts ccp discovers.
tags: [feature, sources, install]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-21T00:00:00Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Examples

```bash
ccp find <query>                        # search skills.sh (PACKAGE + SKILL columns)
ccp find -r github <query>              # search GitHub repos
ccp install                             # replay ccp.toml + ccp.lock (machine migration)
ccp install <owner/repo>                # auto-add + interactive install (recommended)
ccp install <owner/repo> skills/<name>  # one skill, no picker
ccp install <owner/repo> -a             # auto-add + install everything
ccp install <owner/repo> skills/<name> --clone  # editable local copy (converts a shadow)
ccp install <github-url>/blob/<ref>/SKILL.md   # add the repo at that ref, install that skill
ccp source add <owner/repo>             # add only (skills.sh first, GitHub fallback)
ccp source list
ccp source update [name]
ccp source remove <name>
```

# Workflow

1. `find` searches skills.sh by default, showing PACKAGE (owner/repo) and SKILL separately.
2. `install <owner/repo>` adds the source if unknown, then opens the picker.
3. `install <owner/repo> skills/<name>` installs one skill directly.
4. `install <blob-url>` to a `SKILL.md` adds the repo at the URL's ref and installs just that skill via `InstallPath`. `ParseGitWebURL` handles `/blob/`, `/tree/`, raw, and GitLab `/-/blob/` URLs.
5. `install` with no args syncs every source in [ccp.toml](/reference/ccp-config.md): clones missing ones, checks each out at its `commit` in `ccp.lock`, and recreates missing shadows.
6. `source add` tries skills.sh first, then GitHub on the default branch.

# Shadows and local items

An installed item is a **shadow**: `hub/<type>/<name>` is a relative symlink into the source
clone, computed between resolved directories so it works when the hub sits behind a symlink.
The hub stores real content only for **local items** — anything that is not a symlink into
`sources/`. ([decision](/decisions/shadow-source-items.md))

| Action | Effect on a shadow |
|--------|--------------------|
| `source update` | Item updates in place. Refused when the clone has local edits (a changed tracked file, or a new file inside an installed item), listing the files |
| `install … --clone` | Real copy replaces the link, the item leaves `installed`, and its path in the clone is reset to HEAD. On a tracked copy or the user's own link: kept as is, untracked |
| `hub update`, `hub outdated` | Refused / skipped: use `source update` |
| `hub remove` | Removes the link only; the clone stays, and the next `ccp install` recreates it while `installed` lists it |
| `doctor --fix` | Replaces a hub copy identical to its source (ignoring files the source's git ignores) with a shadow; a copy that differs is reported, never touched; sweeps interrupted-migration leftovers |

The local-edit guard and the commit lock need a git source. An item that resolves outside
`sources/` is refused. In `internal/source/shadow.go`: `InstallMode` (`Shadow`, `Clone`),
`Installer.State`, `Reshadow`, `IsShadow`, `LocalEdits`, `SweepLeftovers`, `ValidCommit` and
`GitProvider.CheckoutCommit`.

# Discovery layouts

`DiscoverItems()` scans Claude Code and Codex layouts; `resolveItemPaths()` falls back through them:

| Path | Format | Priority |
|------|--------|----------|
| `skills/` | Root-level (shared) | 1 |
| `.claude/skills/` | Claude Code plugin | 2 |
| `.agents/skills/` | Codex cross-tool standard | 3 |
| `.codex/skills/` | Codex project-level | 4 |
| `.codex-plugin/plugin.json` | Codex plugin manifest | 5 |
| `SKILL.md` at repo root | Bare skill repo (whole repo = one skill) | Last |

A **bare skill repo** has `SKILL.md` at the root with no `skills/<name>/` wrapper. The item name
comes from the frontmatter `name:` (else the source dir name), the whole repo installs as
`skills/<name>`, and `CopyDir()` skips `.git` (a shadow of a bare repo links the repo root, `.git` included).

Codex *consumption* of the hub is not implemented: there is no `ccp codex` command and no
`[codex]` config section. Codex reads project `.agents/skills/`, so
[project setup](/features/project-setup.md) covers it.
