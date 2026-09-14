---
type: File Format
title: .ccp.yaml project config
description: Project-level file that names a profile for automatic selection with ccp auto.
tags: [reference, config, activation]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Schema

```yaml
# .ccp.yaml (project root)
profile: dev
```

`ccp auto` (hidden) looks for `.ccp.yaml` or `.ccp.yml` in the current directory and its
parents, and prints the profile name, or its path with `--path`.

# Examples

```bash
# .bashrc / .zshrc
export CLAUDE_CONFIG_DIR=$(ccp auto --path 2>/dev/null || echo ~/.claude)
```

For the env-file approach (`mise.toml`, `.envrc`) see [activation](/architecture/activation.md).
