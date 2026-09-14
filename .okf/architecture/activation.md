---
type: Mechanism
title: Profile activation
description: How a profile becomes active — per project through env files, globally through the ~/.claude symlink, or per command.
tags: [architecture, activation, profiles]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Modes

| Mode | Command | Effect |
|------|---------|--------|
| Project (default) | `ccp use dev` | Detects `mise.toml`/`.envrc` and writes `CLAUDE_CONFIG_DIR` and `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD` |
| Global | `ccp use quickfix -g` | `~/.claude` → `~/.ccp/profiles/quickfix` (relative symlink) |
| Override | env var | `CLAUDE_CONFIG_DIR` beats the symlink |

```bash
# mise (.mise.toml in the project root)
[env]
CLAUDE_CONFIG_DIR = "~/.ccp/profiles/quickfix"
CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD = "1"

# direnv (.envrc in the project root)
export CLAUDE_CONFIG_DIR="$HOME/.ccp/profiles/quickfix"
export CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD="1"

# one command
CLAUDE_CONFIG_DIR=~/.ccp/profiles/quickfix claude "fix the bug"
```

`ccp use -g` always targets `~/.claude` (`Paths.GlobalClaudeDir`), even inside a project that
sets `CLAUDE_CONFIG_DIR`. A global switch also mirrors the new profile into omp when the user
opted in ([profile switching](/features/omp/profile-switching.md)); project mode leaves omp alone.

# Parallel sessions

Different terminals can run different profiles through the env override, while the
`~/.claude` symlink stays the fallback.

# Loading a profile's CLAUDE.md and rules with --add-dir

```bash
alias claude='CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1 claude --add-dir "${CLAUDE_CONFIG_DIR:-$(ccp which --path)}"'
```

`ccp config shell` prints aliases like this one. Other entry points: `ccp session <profile>`
(a shell with the profile active), `ccp run <profile> -- <cmd>`, and `ccp auto` with
[`.ccp.yaml`](/reference/project-config.md).
