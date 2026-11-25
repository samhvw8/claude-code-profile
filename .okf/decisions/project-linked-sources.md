---
type: Decision
title: "Linked project installs use a machine-local project ccp.toml"
description: "ccp project install --link links source items into a project's .claude/ and records the source in <project>/ccp.toml and ccp.lock, kept out of git, not in ~/.ccp/ccp.toml."
tags: [decision, project, sources]
decided: 2026-11-25
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-25T06:00:00Z }
---

# Decision

`ccp project install <source> <items> --link` makes each item an **absolute** symlink in the
project's `.claude/`, pointing into the shared clone in `~/.ccp/sources/`. The source and its
items are recorded in the project's own `ccp.toml`, and its commit in a `ccp.lock` beside it
([format](/reference/ccp-config.md)). ccp adds both files and every link to the clone's
`.git/info/exclude`. Copying (no `--link`) is unchanged and stays the default
([project setup](/features/project-setup.md)).

| Command | Does |
|---------|------|
| `ccp project install` (no arguments) | Replays the project `ccp.toml`: clones missing sources, checks out the project's locked commit, recreates missing links |
| `ccp project update [source]` | Updates the project's sources and moves its `ccp.lock` |
| `ccp project list` | Marks each link with its source, or as broken |
| `ccp project remove` | Removes the link (never the clone) and drops it from `installed` |

# Why

- A private team source (one repo per employer) does not belong in `~/.ccp/ccp.toml`: that file
  is synced through dotfiles, and every machine without access would fail to clone it.
- A copy never follows `source update` (the drift [shadow items](/decisions/shadow-source-items.md)
  removed from the hub). A link follows the clone.
- The team syncs its committed `.claude/` with its own tool; links that point into one person's
  `~/.ccp` must never be committed, so they live in `.git/info/exclude`, git's per-clone ignore
  file, not in the shared `.gitignore`.

# Implication

- Links are absolute, unlike hub shadows: a project lives outside `~/.ccp` and moves on its own.
  The same containment check as shadows applies (an item resolving outside `sources/` is refused).
- A real file or directory in the way is never replaced; only an earlier ccp link is.
- The clone is shared. When `~/.ccp/ccp.toml` also lists the source, its lock wins: project
  sync never moves the clone, and `project update` only copies the clone's commit into the
  project lock. Otherwise the last project to sync moves the clone, and ccp says so.
- `~/.ccp/state/project-links.toml` (machine-local) lists the project roots that link on this
  machine. `ccp source remove` refuses while a recorded project's `ccp.toml` still lists the
  source (or cannot be read), whether or not `~/.ccp/ccp.toml` lists it too, and names the projects.
- A project `ccp.toml` is ccp's only when its first line is the marker comment ccp writes. Any
  other `ccp.toml` at the project root is never read as links and never overwritten: `list` and
  `remove` ignore it, and `--link` refuses until it is moved away.
- `.git/info/exclude` patterns carry the project's path below the work-tree top (`--dir` into a
  subdirectory). Adding a source writes the excludes at once, so a cancelled picker leaves
  nothing for git to see.
- `ccp project remove` reads `ccp.toml` before deleting anything: an unreadable file stops it.
- Adding a source to a project never fetches over an existing directory, because a fetch deletes
  it: a git clone of the same origin (https, ssh and `git@host:` forms compare equal) or any
  download of a non-git source is reused; anything else is refused.
- No new concept: a project `ccp.toml` is a second place a [Source](/product/core-concepts.md)
  is recorded. No new top-level command: `project update` is a subcommand.
- Not covered: other harnesses' project folders (`.agents/skills/` for Codex); omp already reads
  project `.claude/` ([omp overview](/features/omp/overview.md)). `ccp doctor` does not check
  project links.
