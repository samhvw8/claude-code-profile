# ccp - Claude Code Profile Manager

A CLI tool for managing multiple Claude Code **configuration profiles** via a central hub of reusable components.

## What This Project Does

**ccp** solves the configuration management problem for Claude Code power users:

- **Central Hub**: Store all your skills, agents, hooks, rules, and settings in one place (`~/.ccp/hub/`)
- **Multiple Profiles**: Create purpose-specific configurations (quickfix, full-stack-dev, documentation)
- **Concurrent Profiles**: Run different profiles simultaneously in different projects via mise/direnv
- **Symlink Architecture**: Profiles link to hub items, ensuring single-source-of-truth maintenance
- **Instant Switching**: Switch between profiles globally or per-project via `CLAUDE_CONFIG_DIR`
- **Shared Data**: Share tasks/todos across profiles while keeping history isolated

## Problem

Claude Code's 20 skill limit forces manual reconfiguration for different work modes. There's no mechanism to save, switch, or share configurations. Duplicating skills across setups creates maintenance burden and configuration drift.

## Installation

```bash
go install github.com/samhvw8/ccp@latest
```

Or build from source:

```bash
git clone https://github.com/samhvw8/ccp
cd ccp
go build -o ccp .
```

## Quick Start

```bash
# Initialize from existing ~/.claude
ccp init

# Search and install packages
ccp find debugging
ccp install owner/repo

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
~/.ccp/                               # CCP data directory
├── hub/                              # Single source of truth
│   ├── skills/
│   ├── agents/
│   ├── hooks/
│   ├── rules/
│   ├── commands/
│   └── setting-fragments/
│
├── store/                            # Shared downloadable resources
│   └── plugins/
│       ├── marketplaces/
│       └── cache/
│
├── profiles/
│   ├── default/                      # Migrated from original ~/.claude
│   │   ├── CLAUDE.md
│   │   ├── settings.json
│   │   ├── skills/                   # Symlinks → hub/skills/*
│   │   ├── agents/                   # Symlinks → hub/agents/*
│   │   ├── hooks/
│   │   ├── profile.toml              # Manifest
│   │   └── ...
│   │
│   ├── quickfix/                     # Purpose-specific profile
│   │   └── ...
│   │
│   └── shared/                       # Shared data namespace
│       ├── tasks/
│       ├── todos/
│       └── ...
│
└── ccp.toml                          # Config + installed sources

~/.claude → ~/.ccp/profiles/default   # Symlink to active profile
```

## Commands

### Core

| Command | Alias | Description |
|---------|-------|-------------|
| `ccp init` | | Migrate existing ~/.claude to ~/.ccp structure |
| `ccp migrate` | | Run migrations from older ccp versions |
| `ccp use <profile>` | `u` | Set default profile (~/.claude symlink) |
| `ccp use --show` | | Show current default profile |
| `ccp which` | `w` | Show current active profile |
| `ccp status` | `st` | Show ccp status and health |
| `ccp status --json` | | Output status as JSON |
| `ccp doctor` | | Diagnose and fix common issues |

### Profile Management

| Command | Alias | Description |
|---------|-------|-------------|
| `ccp profile` | `p` | Profile commands |
| `ccp profile create <name>` | `p c` | Create new profile |
| `ccp profile list` | `p l` | List all profiles |
| `ccp profile list --json` | | Output profiles as JSON |
| `ccp profile edit <name> -i` | | Interactive hub item selection |
| `ccp profile check <name>` | | Validate profile against manifest |
| `ccp profile fix <name>` | | Reconcile profile to match manifest |
| `ccp profile sync [name]` | | Regenerate symlinks and settings |
| `ccp profile delete <name>` | | Delete a profile |

### Hub Management

| Command | Alias | Description |
|---------|-------|-------------|
| `ccp hub` | `h` | Hub commands |
| `ccp hub list [type]` | `h l` | List hub contents |
| `ccp hub list --json` | | Output hub contents as JSON |
| `ccp hub link [profile]` | `h ln` | Interactive add hub items to profile |
| `ccp hub add` | | Interactive promote local items to hub |
| `ccp hub add <type> <path>` | | Add item to hub from filesystem |
| `ccp hub show <type/name>` | | Show hub item details |
| `ccp hub remove <type/name>` | | Remove item from hub |
| `ccp link [profile] [path]` | `l` | Add/edit hub items in profile |
| `ccp unlink <profile> <path>` | `ul` | Remove hub item from profile |

### Package Management

| Command | Alias | Description |
|---------|-------|-------------|
| `ccp find <query>` | `search` | Search skills.sh for packages |
| `ccp install` | `i` | Sync all sources from ccp.toml |
| `ccp install <owner/repo>` | | Install from package (auto-adds source) |
| `ccp source` | `s` | Advanced source management |
| `ccp source add <owner/repo>` | | Add source without installing |
| `ccp source list` | | List installed sources |
| `ccp source list --json` | | Output sources as JSON |
| `ccp source update` | | Update installed sources |
| `ccp source remove <name>` | | Remove a source |

## Profile Activation

### Default (symlink)

```bash
ccp use quickfix
# ~/.claude → ~/.ccp/profiles/quickfix
```

### Per-Project (mise)

```toml
# .mise.toml
[env]
CLAUDE_CONFIG_DIR = "~/.ccp/profiles/dev"
```

### Per-Project (direnv)

```bash
# .envrc
export CLAUDE_CONFIG_DIR="$HOME/.ccp/profiles/dev"
```

### Inline

```bash
CLAUDE_CONFIG_DIR=~/.ccp/profiles/quickfix claude "fix the bug"
```

## Profile Manifest

Each profile has a `profile.toml` manifest:

```toml
version = 2
name = "quickfix"
description = "Minimal bug-fixing configuration"

[hub]
skills = ["debugging-core", "git-basics"]
hooks = ["pre-commit-lint"]
rules = ["minimal-change"]

[data]
tasks = "shared"
todos = "shared"
history = "isolated"
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

## Machine Migration (chezmoi)

ccp stores all configuration in `~/.ccp/ccp.toml` and `~/.ccp/hub/`, making it easy to sync across machines:

```bash
# On source machine - add to chezmoi
chezmoi add ~/.ccp/ccp.toml
chezmoi add ~/.ccp/hub

# On new machine - restore
chezmoi apply
ccp source install  # Syncs all sources from ccp.toml
```

The `ccp install` command (with no arguments) will:
- Clone any missing source repositories
- Reinstall hub items listed in `ccp.toml`

## Development

```bash
# Run tests
go test ./...

# Build
go build -o ccp .

# Install locally
go install .
```

## Inspiration

Inspired by [CCS (Claude Code Switch)](https://github.com/kaitranntt/ccs) which provides profile switching for Claude subscriptions and API proxies.

**ccp** manages **complete configuration profiles** including skills, agents, hooks, and settings. It also supports Claude subscription and proxy switching via environment variables in `settings.json`.

| Capability | CCS | ccp |
|------------|-----|-----|
| Claude subscription switching | ✓ | ✓ (via settings env) |
| API proxy configuration | ✓ | ✓ (via settings env) |
| Multiple AI providers (Gemini, etc.) | ✓ | ✓ (manual via env + proxy) |
| Skills, agents, hooks management | — | ✓ |
| Hub-based component reuse | — | ✓ |
| Per-project profiles (mise/direnv) | — | ✓ |

## License

MIT
