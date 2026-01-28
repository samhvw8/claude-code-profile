# ccp - Claude Code Profile Manager

A CLI tool for managing multiple Claude Code profiles via a central hub of reusable components.

## Problem

Claude Code's 20 skill limit forces manual reconfiguration for different work modes. There's no way to save, switch, or share configurations. Duplicating skills across setups creates maintenance burden.

## Solution

`ccp` manages a central hub of reusable components (skills, hooks, rules, commands, md-fragments) and multiple profiles. Each profile is a complete Claude Code configuration directory, activated via `CLAUDE_CONFIG_DIR` or symlink.

## Installation

```bash
go install github.com/samhoang/ccp@latest
```

Or build from source:

```bash
git clone https://github.com/samhoang/ccp
cd ccp
go build -o ccp .
```

## Quick Start

```bash
# Initialize from existing ~/.claude
ccp init

# List profiles
ccp profile list

# Create a new profile
ccp profile create quickfix --skills=debugging-core,git-basics

# Or use interactive mode
ccp profile create dev -i

# Switch active profile
ccp use quickfix

# Show current profile
ccp use --show
```

## Architecture

```
~/.claude/
├── hub/                              # Single source of truth
│   ├── skills/
│   ├── hooks/
│   ├── rules/
│   ├── commands/
│   └── md-fragments/
│
├── profiles/
│   ├── default/                      # Migrated from original ~/.claude
│   │   ├── CLAUDE.md
│   │   ├── settings.json
│   │   ├── skills/                   # Symlinks → hub/skills/*
│   │   ├── hooks/
│   │   ├── profile.yaml              # Manifest
│   │   └── ...
│   │
│   ├── quickfix/                     # Purpose-specific profile
│   │   └── ...
│   │
│   └── shared/                       # Shared data namespace
│       ├── tasks/
│       ├── todos/
│       └── ...
```

## Commands

| Command | Description |
|---------|-------------|
| `ccp init` | Migrate existing ~/.claude to hub + profile structure |
| `ccp use <profile>` | Set default profile (~/.claude symlink) |
| `ccp use --show` | Show current default profile |
| `ccp profile create <name>` | Create new profile |
| `ccp profile list` | List all profiles |
| `ccp profile check <name>` | Validate profile against manifest |
| `ccp profile fix <name>` | Reconcile profile to match manifest |
| `ccp profile delete <name>` | Delete a profile |
| `ccp link <profile> <path>` | Add hub item to profile |
| `ccp unlink <profile> <path>` | Remove hub item from profile |
| `ccp hub list [type]` | List hub contents |

## Profile Activation

### Default (symlink)

```bash
ccp use quickfix
# ~/.claude → ~/.claude/profiles/quickfix
```

### Per-Project (mise)

```toml
# .mise.toml
[env]
CLAUDE_CONFIG_DIR = "~/.claude/profiles/dev"
```

### Per-Project (direnv)

```bash
# .envrc
export CLAUDE_CONFIG_DIR="$HOME/.claude/profiles/dev"
```

### Inline

```bash
CLAUDE_CONFIG_DIR=~/.claude/profiles/quickfix claude "fix the bug"
```

## Profile Manifest

Each profile has a `profile.yaml` manifest:

```yaml
name: quickfix
description: "Minimal bug-fixing configuration"
created: 2025-01-28T10:00:00Z
updated: 2025-01-28T10:00:00Z

hub:
  skills:
    - debugging-core
    - git-basics
  hooks:
    - pre-commit-lint
  rules:
    - minimal-change

data:
  tasks: shared
  todos: shared
  history: isolated
  file-history: isolated
```

## Shell Completion

```bash
# Bash
source <(ccp completion bash)

# Zsh
ccp completion zsh > "${fpath[1]}/_ccp"

# Fish
ccp completion fish | source
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o ccp .

# Install locally
go install .
```

## License

MIT
