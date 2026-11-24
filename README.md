# ccp - Claude Code Profile Manager

A CLI tool for managing multiple Claude Code **configuration profiles** via a central hub of reusable components.

## What This Project Does

**ccp** solves the configuration management problem for Claude Code power users:

- **Central Hub**: Store all your skills, agents, hooks, rules, commands and output styles in one place (`~/.ccp/hub/`)
- **Multiple Profiles**: Create purpose-specific configurations (quickfix, full-stack-dev, documentation)
- **Settings Templates**: Complete `settings.json` templates referenced by name — what you see is what you get
- **Sources**: Install skills and agents from GitHub or skills.sh; each item links into its source clone, so an update is one command
- **Project Setup**: Copy hub items into any project's `.claude/` for team-shareable, git-committable config
- **Concurrent Profiles**: Run different profiles simultaneously in different projects via mise/direnv
- **Accounts**: Run one profile under several Claude logins, pick one per terminal, or let ccp pick the one with the most plan limit left
- **Other Agents**: Mirror a profile into omp, opencode, Antigravity CLI (agy) and pi
- **Symlink Architecture**: Profiles link to hub items, ensuring single-source-of-truth maintenance
- **Instant Switching**: Switch between profiles globally or per-project via `CLAUDE_CONFIG_DIR`

## Problem

Claude Code reads one configuration folder. Different work modes need different skills, rules and settings, and Claude Code has no way to save, switch or share a configuration. Copying skills between setups creates maintenance work and configuration drift.

## Installation

```bash
go install github.com/samhvw8/claude-code-profile/cmd/ccp@latest
```

Or install a prebuilt release binary with [mise](https://mise.jdx.dev):

```bash
mise use -g github:samhvw8/claude-code-profile
```

Or build from source:

```bash
git clone https://github.com/samhvw8/claude-code-profile
cd claude-code-profile
go build -o ccp ./cmd/ccp
```

## Quick Start

```bash
# Initialize from existing ~/.claude
ccp init

# Search and install packages
ccp find debugging
ccp install owner/repo

# Create a new profile
ccp profile create quickfix --skills=debugging-core,git-basics

# Or use interactive mode
ccp profile create dev -i

# Switch active profile
ccp use quickfix

# Copy hub items into a project's .claude/
ccp project add skills/coding agents/reviewer
```

## Architecture

```
~/.ccp/
├── hub/                              # Single source of truth
│   ├── skills/
│   ├── agents/
│   ├── hooks/
│   ├── rules/
│   ├── commands/
│   ├── output-styles/
│   ├── bundles/                      # Coupled items that link as one unit
│   └── settings-templates/           # Complete settings.json templates
│
├── store/                            # Shared downloadable resources
│   └── plugins/
│       ├── marketplaces/
│       └── cache/
│
├── sources/                          # Cloned source repositories
│
├── profiles/
│   ├── default/                      # Migrated from original ~/.claude
│   │   ├── CLAUDE.md
│   │   ├── settings.json
│   │   ├── skills/                   # Symlinks → hub/skills/*
│   │   ├── agents/                   # Symlinks → hub/agents/*
│   │   ├── hooks/
│   │   ├── rules/
│   │   ├── profile.toml              # Manifest
│   │   └── ...
│   │
│   └── shared/                       # Shared data (tasks, todos, etc.)
│       ├── tasks/
│       ├── todos/
│       └── ...
│
├── accounts/                         # One folder per Claude login
│   └── work1/                        # Own login; links everything else
│       └── .ccp-account.toml         #   to its profile
│
├── state/                            # Machine-local: settings snapshots
├── ccp.toml                          # Config + installed sources
└── ccp.lock                          # Commit pinned for each source

~/.claude → ~/.ccp/profiles/default   # Symlink to active profile
```

## Commands

### Core

| Command | Description |
|---------|-------------|
| `ccp init` | Migrate existing ~/.claude to ~/.ccp structure |
| `ccp migrate` | Run migrations from older ccp versions |
| `ccp use <profile> [-g]` | Switch profile (project or global) |
| `ccp which` | Show current active profile |
| `ccp status` | Show ccp status and health, and each account's plan usage |
| `ccp account [list]` | List accounts with their profile, pool and plan usage |
| `ccp account add <name> [<profile>] [--pool <p>]` | Create an account for a profile, optionally in a pool |
| `ccp account pool <name> <pool>\|--clear` | Put an account in a pool (for `--account auto:<pool>`), or take it out |
| `ccp account remove <name> [--all] [--force]` | Delete an account's login, or with `--all` its login and data; refused while a session runs on it |
| `ccp doctor [--fix]` | Diagnose and fix common issues |
| `ccp bootstrap` | New machine: sync sources from `ccp.toml`, then fix all profiles |
| `ccp config shell` | Print the `ccp-use` shell function |
| `ccp reset` | Remove ccp and restore `~/.claude` as a plain folder |

### Profile Management

| Command | Description |
|---------|-------------|
| `ccp profile create <name>` | Create new profile (flags or `-i` interactive) |
| `ccp profile list` | List all profiles |
| `ccp profile edit <name>` | Add/remove hub items |
| `ccp profile sync [--all]` | Regenerate symlinks and settings |
| `ccp profile fix <name>` | Reconcile profile to match manifest |
| `ccp profile delete <name>` | Delete a profile |

### Hub Management

| Command | Description |
|---------|-------------|
| `ccp hub list [type]` | List hub contents |
| `ccp hub add <type> <path>` | Add item to hub |
| `ccp hub show <type/name>` | Show hub item details |
| `ccp hub edit <type/name>` | Edit a hub item in $EDITOR |
| `ccp hub update` | Update hub items from their source repositories |
| `ccp hub remove <type/name>` | Remove item from hub |
| `ccp link [profile] [item]` | Link hub item to profile |
| `ccp unlink <profile> <item>` | Unlink hub item from profile |

An output style (`output-styles/<name>.md`) becomes selectable in Claude Code once linked; the profile's `outputStyle` setting picks one.

### Bundles

A bundle groups coupled items (for example a skill, its hook and its agent) that must travel together. Its members can't be linked on their own: the bundle links, unlinks and is removed as one unit.

| Command | Description |
|---------|-------------|
| `ccp bundle create <name>` | Create a bundle from hub items |
| `ccp bundle list` | List bundles in the hub |
| `ccp bundle show <name>` | Show a bundle's members |
| `ccp bundle remove <name>` | Remove a bundle from the hub |

### Project Setup

| Command | Description |
|---------|-------------|
| `ccp project add [items...] [-i]` | Copy hub items into project's `.claude/` |
| `ccp project list` | List items in project's `.claude/` |
| `ccp project remove [items...]` | Remove items from project's `.claude/` |

### Other Agents (omp, opencode, agy, pi)

omp (oh-my-pi) reads its own config from `~/.omp/agent/` and only loads `~/.claude` when its `enabledProviders` setting opts in, so ccp mirrors a profile into omp's agent directory. Only items omp actually loads are linked (skills with descriptions, agents with a description and a resolvable model, commands); rules and Claude hooks are reported as inert instead, because omp surfaces a rule only with `description`/`alwaysApply` frontmatter and imports hooks as modules. `ccp omp` is hidden — run `ccp omp --help` for it.

| Command | Description |
|---------|-------------|
| `ccp omp link skills/debugging commands/ship.md` | Link hub items into `~/.omp/agent/` |
| `ccp omp link` | Interactive selection of items not linked yet |
| `ccp omp sync [profile]` | Mirror a profile's skills and commands to omp |
| `ccp omp list [profile] [--json]` | List links and show what a sync would change |
| `ccp omp unlink skills/debugging` | Remove ccp links from omp |
| `ccp omp unlink` / `--all` | Interactive selection, or remove everything ccp set up |
| `ccp omp --profile work sync` | Target an omp named profile |

Once anything is linked, `ccp use <profile> -g` syncs omp against the profile you switched to, including its global context: `~/.omp/agent/AGENTS.md` → the profile's `CLAUDE.md`, and `RULES.md` generated from the profile's rules. `ccp doctor` reports omp links too: broken ones (the hub item is gone) and drift against the active profile.

The other agents have a hidden `sync` command each. After the first sync, ccp keeps them current on `ccp use -g`, on link and unlink in the active profile, and on `ccp bootstrap`. ccp records the files it writes and replaces or removes only those; a file you wrote or edited is left alone and reported.

| Command | Description |
|---------|-------------|
| `ccp opencode sync [profile]` | Write converted copies of the profile's agents and commands into `~/.config/opencode`, plus its output style |
| `ccp agy sync [profile]` | Mirror the profile into Antigravity CLI (`~/.gemini/config`): `AGENTS.md` links to `CLAUDE.md`, skills are linked, rules and agents are converted copies, the output style goes into `GEMINI.md` |
| `ccp pi sync [profile]` | Write the profile's output style into pi's `APPEND_SYSTEM.md` |

### Package Management

| Command | Description |
|---------|-------------|
| `ccp find <query>` | Search skills.sh for packages |
| `ccp install [owner/repo] [items...]` | Install from a package, or sync all sources from `ccp.toml` |
| `ccp install owner/repo <item> --link .` | Install and link to the active profile (or name a profile) |
| `ccp install owner/repo <item> --clone` | Copy the item into the hub as a local item you can edit |
| `ccp source list` | List installed sources |
| `ccp source update` | Update installed sources |
| `ccp source remove <name>` | Remove a source |

An installed item is a shadow: a symlink into the source's clone, so `ccp source update` updates it in place. `ccp.lock` pins each source's commit, and `ccp install` with no arguments checks out those commits on another machine.

### Settings Templates

| Command | Description |
|---------|-------------|
| `ccp template list` | List settings templates |
| `ccp template show <name>` | Display template JSON |
| `ccp template create <name>` | Create new template |
| `ccp template edit <name>` | Edit in $EDITOR |
| `ccp template extract <name>` | Extract from profile's settings |

## Profile Activation

### Global (symlink)

```bash
ccp use quickfix -g
# ~/.claude → ~/.ccp/profiles/quickfix
```

### Per-Project (mise)

`ccp use dev` in a project writes this for you:

```toml
# mise.toml
[env]
CLAUDE_CONFIG_DIR = "~/.ccp/profiles/dev"
```

### Per-Project (direnv)

```bash
# .envrc
export CLAUDE_CONFIG_DIR="$HOME/.ccp/profiles/dev"
```

### One terminal or one command

```bash
ccp session dev                      # a new shell with the profile active
ccp run dev -- claude --continue     # one command; the default command is claude
eval "$(ccp env dev)"                # this terminal only
```

Everything after `--` is the command and its arguments, so name `claude` before its flags.

Claude Code loads the profile's `CLAUDE.md` and rules from `CLAUDE_CONFIG_DIR` (or the `~/.claude` symlink) by itself. Don't wrap `claude` in an alias that adds `--add-dir`: since Claude Code 2.1.326 that asks for workspace trust on every start.

## Accounts

One profile can run under several Claude logins. Each account is a folder in `~/.ccp/accounts/<name>/` with its own login; everything else links to the profile, so skills, settings and history stay the same.

Add each account, then log in once. Type `/login` in Claude Code:

```bash
ccp account add work1 work       # account work1 for profile work
ccp run --account work1          # log in with /login
ccp account add work2 work
ccp run --account work2
ccp account                      # list accounts and their usage
```

Then use them:

```bash
ccp run --account work2                          # this account, its own profile
ccp run work --account auto                      # the account with the most limit left
ccp run work --account auto -- claude --resume   # with claude flags
eval "$(ccp env work --account auto)"            # this terminal only
ccp status                                       # each account's 5-hour and weekly usage
```

To make plain `claude` in a project folder pick the account by itself:

```bash
cd ~/work/acme
ccp use work --accounts   # mise.toml (or .envrc) + a .ccp/bin/claude shim
claude                    # runs on work's account with the most limit left
```

With no account logged in yet, `claude` there runs on the profile itself.

Seats used from several profiles can share a **pool**:

```bash
ccp account pool work1 company           # work1 joins pool company (any profile)
ccp run client --account auto:company    # the pool's freest account, relinked to client if no session runs on it
ccp use client --accounts --pool company # the folder's claude picks from the pool
```

`ccp run` relinks a Claude account only while no Claude Code process runs on it; `ccp env` and `ccp session` never relink and only take pool members already linked to the profile.

`--account auto` picks among the accounts linked to the profile, once, when the command starts. It reads each account's plan usage from Claude's usage endpoint (cached 5 minutes) and prefers an account under 80% of its 5-hour limit and 95% of its weekly limit. When usage is unknown or equal, it picks the account used least recently. ccp never refreshes a token and never switches accounts in a running session: to move off a busy account, start Claude Code again.

`ccp migrate --accounts [--dry-run]` moves each profile's existing login into the account of the same name.

## Profile Manifest

Each profile has a `profile.toml` manifest:

```toml
version = 3
name = "quickfix"
description = "Minimal bug-fixing configuration"
settings-template = "opus-full"   # Optional settings template

[hub]
skills = ["debugging-core", "git-basics"]
hooks = ["pre-commit-lint"]
rules = ["minimal-change"]
commands = ["quick-test"]
```

Data directories (tasks, todos, history) are always shared across profiles.

## Shell Completion

```bash
# Bash
source <(ccp completion bash)

# Zsh
ccp completion zsh > "${fpath[1]}/_ccp"

# Fish
ccp completion fish | source
```

## Machine Migration

Keep `~/.ccp/ccp.toml`, `~/.ccp/ccp.lock`, the hub and the profiles in your dotfiles (chezmoi, a dotfiles repo, stow). On the new machine:

```bash
# restore your dotfiles, then:
ccp bootstrap   # syncs sources at the pinned commits and fixes all profiles
```

With chezmoi, `ccp bootstrap --push` adds local-only hub items, `ccp.toml` and the profiles to chezmoi.

## Development

```bash
go test ./...              # Run tests
go build -o ccp ./cmd/ccp  # Build
go install ./cmd/ccp       # Install locally
```

## License

MIT
