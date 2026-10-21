---
type: Decision
title: "Source items are shadows, not copies"
description: "A hub item installed from a source is a symlink into the source clone; ccp.lock pins each source's commit; --clone is the only way to get an editable copy."
tags: [decision, sources, hub]
decided: 2026-10-02
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-21T00:00:00Z }
---

# Decision

`ccp install owner/repo <item>` makes `hub/<type>/<name>` a **shadow**: a relative symlink into
`sources/<repo>/<item path>`. It does not copy the item. There is no copy mode beside it:

| Hub entry | Kind | Who owns it |
|-----------|------|-------------|
| Symlink into `sources/` | Shadow of a source item | The source; listed in `installed` |
| Real file or directory | Local item | The user; ccp keeps it |

`--clone` on `ccp install` is the one way to get a copy, and the one way to make an installed
item local. It installs a real copy, converts an existing shadow in place, or keeps an existing
tracked copy (or the user's own link) as it is; each way the item leaves the source's `installed`
list. Edits made through a shadow live in the clone; `--clone` moves them into the copy first and
resets that path in the clone last, so a failed reset never costs an edit.

`ccp.lock`, beside `ccp.toml`, is the lock: it holds each source's `commit`. `ccp install` with
no arguments checks each clone out at the locked commit, and `ccp source update` moves it forward. Nothing else rewrites it: a checkout
that fails (offline, rate-limited, force-pushed) only warns.

The lock and the local-edit guard need a git source. Another provider (HTTP) has no clone
history, so neither applies there.

# Why

- A copy duplicated content already in `sources/` (13M for one skill), and `source update` never
  refreshed it: on the author's machine 27 of 66 installed items had drifted from their clone.
- The hub mixed owned items with mirrors of GitHub repos, and nothing showed which was which. A
  symlink says it by itself.
- A second "copy mode" would need two code paths in install, update, doctor and remove for a
  benefit `--clone` already gives. It adds no user-facing concept: a shadow is how a
  [Source](/product/core-concepts.md) item is stored.

# Implication

- `ccp source update` refuses a clone with local edits and names the files: its
  `git reset --hard` would otherwise discard edits made through a shadow. A local edit is any
  change to a tracked file, or an untracked file inside an installed item; an untracked file
  elsewhere (a `.DS_Store`) survives the reset and does not block.
- `ccp install` (sync) leaves a clone with local edits at its current commit, with a warning.
  It installs missing items one at a time and keeps a failed one in `installed` for the next sync.
- `installed` holds hub names. A plugin item (`skills/<plugin>-<name>`) or a URL install is found
  again by the name each candidate in the clone would install as.
- `ccp.toml` holds only what the user chose (url, ref, `installed`); `ccp.lock` holds only what
  ccp resolved (`commit`, `checksum`). Neither holds a timestamp: a per-machine `updated` made
  every install differ between machines that agree. Both files are synced.
- `ccp.toml` and `ccp.lock` are synced and hand-editable, so they are not trusted: `commit` must be a full SHA,
  a `ref` or `url` cannot pass for a git option, and an `installed` entry must be a plain
  `type/name` inside the hub.
- An item that resolves outside `sources/` — itself, or through a symlink inside it — is refused
  in both modes: an upstream repo cannot point a hub item at `~/.ssh`.
- `ccp doctor --fix` replaces each hub copy that is identical to its source (files, empty
  directories, modes and bytes) with a shadow. Files only in the copy that the source repo's git
  ignores (a `__pycache__/` written when a skill's script ran) do not count: a shadow would hold
  them too, and they are dropped with the copy. A copy that differs, or a link to somewhere
  outside `sources/`, is reported and never touched. It also sweeps the hidden
  `.<name>.ccp-reshadow` / `.ccp-clone` leftovers of an interrupted migration or clone: a
  redundant one is removed, the only copy of an item is moved back.
- `ccp hub update` and `hub outdated` own items with a `.ccp-source` manifest (`ccp hub add`);
  they refuse or skip shadows, which `ccp source update` owns.
- Code that lists the hub must use `os.Stat`, not `DirEntry.IsDir()`, to see a shadow directory
  as a directory (`internal/hub/scanner.go`).
- Relative links are made by `symlink.CreateRelative` and recognized by `symlink.PointsInto`,
  shared with omp linking and profile hub links.

See [source system](/features/source-system.md).
