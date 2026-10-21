---
type: File Format
title: ccp.toml configuration
description: Global ccp configuration — default registry, registry settings, and the auto-managed list of installed sources.
resource: internal/config/ccp_config.go
tags: [reference, config, sources]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-21T00:00:00Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
---

# Schema

Location: `~/.ccp/ccp.toml`. Generate defaults with `ccp config init` (`ccp init` writes it too).

```toml
default_registry = "skills.sh"   # or "github"

[github]
topics = ["agent-skills", "claude-code", "claude-skills"]
per_page = 10

[skillssh]
base_url = "https://skills.sh"
limit = 10

# Installed sources — managed by `ccp source` / `ccp install`
# Paths are relative to ~/.ccp/ for portability
[sources.'owner/repo']
registry = 'github'
provider = 'git'
url = 'https://github.com/owner/repo.git'
path = 'sources/owner--repo'
ref = 'main'
installed = ['skills/my-skill', 'agents/my-agent']
```

# ccp.lock

`ccp.lock` sits beside `ccp.toml` and holds what ccp resolved for each source. ccp writes it;
sync it with `ccp.toml`. When `~/.ccp/ccp.toml` is a symlink (into a dotfiles repo), `ccp.lock`
goes beside the real file, so the pair is synced together (`config.CcpLockPath`).

```toml
version = 1

[sources.'owner/repo']
commit = 'abc123...'   # full git SHA; 'checksum' for an HTTP source
```

`ccp.toml` holds what you chose; `ccp.lock` holds the version. Neither file holds a timestamp,
so two machines with the same sources have the same bytes.

`commit` is a lock: `ccp install` with no arguments replays both files on a new machine — it
clones missing sources, checks each one out at `commit`, and recreates the shadow links for
`installed`. `ccp source update` moves `commit` forward; a failed checkout only warns and never rewrites it.
`commit` must be a full SHA, or sync ignores it. The lock needs a git source. `installed` lists
only shadows, by hub name; an item installed with `--clone` is a local item and is not listed
([decision](/decisions/shadow-source-items.md)).

A `ccp.toml` written before `ccp.lock` still carries `commit` (and an `updated` timestamp). ccp
reads that `commit` when `ccp.lock` has no entry for the source, and the next save moves it into
`ccp.lock` and drops `updated`. See [source system](/features/source-system.md) and
[bootstrap](/features/bootstrap.md).
