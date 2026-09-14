---
type: File Format
title: ccp.toml configuration
description: Global ccp configuration — default registry, registry settings, and the auto-managed list of installed sources.
resource: internal/config/ccp_config.go
tags: [reference, config, sources]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
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
commit = 'abc123...'
installed = ['skills/my-skill', 'agents/my-agent']
```

`ccp install` with no arguments replays this file on a new machine: it clones missing
sources and reinstalls their items. See [source system](/features/source-system.md) and
[bootstrap](/features/bootstrap.md).
