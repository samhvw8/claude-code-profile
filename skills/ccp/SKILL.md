---
name: ccp
description: "Use and manage ccp — the Claude Code Profile Manager. Use when: switching profiles, creating profiles, installing skills from sources, managing hub items, linking/unlinking items, setting up ccp for the first time, activating profiles per-project. Actions: switch, create, install, link, unlink, init, find, list, update profiles and hub items. Keywords: ccp use, ccp init, profile switch, hub link, source add, install skill, settings template, find skills, profile create. Casual triggers: 'switch to my dev profile', 'install a skill', 'set up ccp', 'find skills on github', 'which profile am I using', 'add this skill to my profile', 'how do I use ccp'."
---

# ccp — Claude Code Profile Manager

Manage multiple Claude Code configurations via a central hub of reusable components.

## Installation

Before using ccp commands, check if ccp is installed. If not, install it automatically:

```bash
# Check if ccp exists
command -v ccp >/dev/null 2>&1

# If not installed, install via mise (preferred)
# Step 1: Install mise if needed
if ! command -v mise >/dev/null 2>&1; then
  curl https://mise.jdx.dev/install.sh | sh
  # Add to shell (user may need to restart shell)
  eval "$(~/.local/bin/mise activate)"
fi

# Step 2: Install ccp via mise (prebuilt release binary)
mise use -g github:samhvw8/claude-code-profile
# or build from source: go install github.com/samhvw8/claude-code-profile/cmd/ccp@latest
```

After install, initialize: `ccp init`

## Core Concepts

| Concept | What |
|---------|------|
| **Hub** | Central directory of reusable items (~/.ccp/hub/) |
| **Profile** | Named config referencing hub items via symlinks |
| **Source** | External repo providing installable items |
| **Settings Template** | Complete settings.json stored in hub |
| **Activation** | How a profile becomes active (global symlink or env var) |

## Common Workflows

### Switch Profiles
```bash
ccp use dev -g              # Global: ~/.claude → ~/.ccp/profiles/dev
ccp use quickfix            # Project: updates mise.toml or .envrc
ccp which                   # Show active profile
```

### Find and Install Skills
```bash
ccp find <query>                          # Search skills.sh
ccp find -r github <query>               # Search GitHub
ccp install owner/repo                    # Add source + interactive install
ccp install owner/repo -a                 # Install all items
ccp install owner/repo skills/x --link .    # Install and link to the active profile
ccp install owner/repo skills/x --link dev  # Install and link to 'dev'
```

### Link Items to Profiles

The profile name comes first, then one `type/name` path. Agents and rules keep
their `.md` extension (`agents/reviewer.md`).

```bash
ccp link default skills/coding            # Link one item to profile 'default'
ccp link default agents/reviewer.md
ccp unlink default skills/coding          # Remove one item
ccp link                                  # Interactive picker for the active profile
ccp link dev                              # Interactive picker for 'dev'
ccp hub add skills browser-skill --from-profile=default  # Move a profile-local item into the hub, linked back
```

Linking or unlinking a hook updates `settings.json`'s `hooks` right away; no
`ccp profile sync` needed.

### Settings

`settings.json` is generated from the settings template, the profile's
`settings-fragment.json` and its linked hooks. Edits made directly to
`settings.json` (by hand or by Claude Code) survive only once captured:

```bash
ccp profile capture default               # Save non-hook edits into the fragment
ccp profile sync default                  # Relink items; regenerate settings.json
ccp profile sync default --force          # Regenerate even if it drops uncaptured edits
```

Without `--force`, sync never drops uncaptured edits: it updates hooks only and
names the keys; `ccp profile capture` followed by sync then applies everything. Template switches and fragment edits apply normally,
and hooks you added yourself (by hand or via `/hooks`) are always kept.

### Sources
```bash
ccp source list                           # List installed sources
ccp source update                         # Update all sources
ccp source add owner/repo                 # Add a source
```

### Link Items to Other Agents

omp (oh-my-pi) does not read `~/.claude` unless opted in, so ccp mirrors a profile into its own config directory — but only what omp actually loads. Rules and Claude hooks are reported as inert instead of being symlinked into directories omp ignores (`RULES.md` carries the rules).

```bash
ccp omp link skills/debugging commands/ship.md  # Link hub items to omp
ccp omp link                                    # Interactive picker (no args)
ccp omp sync                                    # Reconcile omp with active profile
ccp omp list                                    # Links + missing/stale/inert vs profile
ccp omp unlink skills/debugging                 # Remove ccp links
ccp omp --profile work sync                     # Target an omp named profile
```

After the first link, `ccp use <profile> -g` mirrors the new profile additively
(hand-made links survive and are reported, not pruned) and refreshes omp's global
context (`AGENTS.md` → the profile's `CLAUDE.md`, `RULES.md` from its rules).
`ccp doctor` reports broken omp links when the hub item behind them is gone.
`ccp omp` is a hidden command.

opencode reads `~/.claude/CLAUDE.md` and `~/.claude/skills/` itself; point its
`instructions` at `"{env:HOME}/.claude/rules/*.md"` for rules. Agents and
commands need converting, which ccp does:

```bash
ccp opencode sync            # Write the active profile's agents/commands into ~/.config/opencode
```

After the first sync, `ccp use -g`, link/unlink and `ccp bootstrap` keep it
current. ccp only replaces files it wrote; hooks are not mirrored (opencode
uses plugins). `ccp opencode` is a hidden command.

## Picker Controls (fzf-style)

| Key | Normal Mode | Search Mode (after /) |
|-----|-------------|----------------------|
| `↑`/`↓` | Navigate | Navigate |
| `Tab`/`Space` | Toggle item | Toggle item |
| `a` | Select all/none | — |
| `f` | Filter checked/unchecked | — |
| `Enter` | Confirm | Confirm |
| `Esc` | — | Clear search |

## Activation Modes

| Mode | Command | How It Works |
|------|---------|--------------|
| Global | `ccp use dev -g` | ~/.claude symlink → profile dir |
| Project | `ccp use dev` | Updates mise.toml/envrc |
| Inline | env var | `CLAUDE_CONFIG_DIR=~/.ccp/profiles/dev claude` |

## Tips

- `ccp install` (no args) syncs all sources — useful for new machines
- `ccp project add` copies (not symlinks) so projects are git-committable
- `ccp doctor` diagnoses broken hub links, manifests, and hub structure; `--fix` removes links whose hub item is gone
- `ccp status` shows the active profile, hub counts and per-profile health
