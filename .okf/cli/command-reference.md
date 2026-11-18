---
type: CLI Reference
title: Command reference
description: Every ccp command group with a one-line description and an example; hidden commands are marked.
tags: [cli, reference, commands]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-17T10:00:00Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md, CLI Command Reference (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: hidden-cmds
    resource: "cmd/*.go files declaring Hidden: true"
    title: Hidden command declarations
    last_modified: 2026-09-14
---

# Visibility

Power-user commands are hidden from `--help` but still work ([decision](/decisions/command-surface.md)).
Hidden as of 2026-09-14:[^hidden-cmds] `auto`, `env`, `hub outdated`, `hub prune`, `hub protect`,
`hub unprotect`, `hub rename`, `agy`, `omp`, `opencode`, `pi`, `profile check`, `profile clone`, `profile diff`, `run`,
`session`, `usage`. Flags: [command flags](/cli/command-flags.md).

# Core

| Command | Description | Example |
|---------|-------------|---------|
| `ccp init` | Migrate an existing `~/.claude` to hub + default profile | `ccp init` |
| `ccp migrate` | Run migrations from older ccp versions; `--accounts` (opt-in) moves each profile's Claude login into the account of the same name | `ccp migrate --accounts --dry-run` |
| `ccp reset` | Undo ccp and restore `~/.claude`; the plan lists account folders and their orphaned logins | `ccp reset` |
| `ccp use <name>` | Set the project profile (auto-detects mise.toml/.envrc) | `ccp use dev` |
| `ccp use <name> --accounts` | Set the project profile, and make plain `claude` here pick the profile's account with the most limit left (`.ccp/bin/claude` shim) | `ccp use companyA --accounts` |
| `ccp use <name> -g` | Set the global profile (`~/.claude` symlink) | `ccp use quickfix -g` |
| `ccp use --show` | Show the current global profile | `ccp use --show` |
| `ccp which [--path]` | Show the active profile (and account), or its directory | `ccp which --path` |
| `ccp account [list] [--json]` | List accounts with profile, email, 5-hour and weekly use and last use; with none, how to add one | `ccp account` |
| `ccp account add <name> [<profile>]` | Create an account for a profile (default: the current one); it starts logged out, so log in with `ccp run --account <name>` and `/login` | `ccp account add gt1 companyA` |
| `ccp status` | Status and health; with accounts, each one's profile, email, 5-hour and 7-day use and last use | `ccp status` |
| `ccp doctor` | Diagnose (including accounts with a missing profile or broken links), and with `--fix` repair (including hub copies of source items → shadows) | `ccp doctor --fix` |
| `ccp usage` | Hub item usage across profiles (hidden) | `ccp usage` |
| `ccp env [<profile>]` | Configure a project env for a profile, optionally under an account (`auto`: shell format only) (hidden) | `ccp env default --account work1 --format=mise` |
| `ccp config init` | Write a default `ccp.toml` | `ccp config init` |
| `ccp config shell` | Print the `ccp-use` shell function | `ccp config shell >> ~/.zshrc` |
| `ccp bootstrap [--push]` | New-machine setup: sync sources, fix profiles; `--push` adds to chezmoi ([details](/features/bootstrap.md)) | `ccp bootstrap` |

# Profiles

| Command | Description | Example |
|---------|-------------|---------|
| `ccp profile create <name>` | Create a profile | `ccp profile create quickfix` |
| `ccp profile list` | List profiles | `ccp profile list` |
| `ccp profile check <name>` | Validate against the manifest (hidden) | `ccp profile check quickfix` |
| `ccp profile fix [name]` | Reconcile to the manifest | `ccp profile fix quickfix --dry-run` |
| `ccp profile delete <name>` | Delete; refused while an account is linked to it | `ccp profile delete quickfix` |
| `ccp profile rename <old> <new>` | Rename (the new name is validated); linked accounts move with it | `ccp profile rename dev development` |
| `ccp profile clone <src> <new>` | Clone (hidden) | `ccp profile clone default dev` |
| `ccp profile diff <a> [b]` | Compare two profiles (hidden) | `ccp profile diff dev prod` |
| `ccp profile sync [name]` | Regenerate symlinks and settings.json | `ccp profile sync --all` |
| `ccp profile edit [name]` | Add/remove hub items | `ccp profile edit -i` |
| `ccp profile capture [name]` | Save manual settings edits as a fragment ([details](/reference/settings-templates.md)) | `ccp profile capture --dry-run` |

# Hub

| Command | Description | Example |
|---------|-------------|---------|
| `ccp hub list [type]` | List hub contents | `ccp hub list skills` |
| `ccp hub update [type/name]` | Update items from their source | `ccp hub update --all` |
| `ccp hub add <type> <path>` | Add an item | `ccp hub add skills ./my-skill` |
| `ccp hub add <type> <name> --from-profile` | Promote a profile item | `ccp hub add skills my-skill --from-profile=default` |
| `ccp hub show [type/name] [-i]` | Show item details and users | `ccp hub show skills/git-basics` |
| `ccp hub edit <type>/<name>` | Edit in `$EDITOR` | `ccp hub edit rules/tone.md` |
| `ccp hub remove [type/name] [-i]` | Remove (offers copy to profiles) | `ccp hub remove skills/old-skill` |
| `ccp hub rename <type>/<name> <new>` | Rename (hidden) | `ccp hub rename skills/old new` |
| `ccp hub prune` | Remove orphans (hidden) | `ccp hub prune -i` |
| `ccp hub protect [type/name...]` | Protect from pruning (hidden) | `ccp hub protect skills/debug` |
| `ccp hub unprotect [type/name...]` | Remove protection (hidden) | `ccp hub unprotect skills/debug` |

# Bundles

| Command | Description | Example |
|---------|-------------|---------|
| `ccp bundle create <name>` | Compose a bundle (interactive or `--skill`/`--hook`… flags) | `ccp bundle create impeccable` |
| `ccp bundle list` / `show <name>` / `remove <name>` | Inspect and remove | `ccp bundle show impeccable` |

Link a bundle with `ccp link <profile> bundles/<name>` ([bundles](/features/bundles.md)).

# Sources

| Command | Description | Example |
|---------|-------------|---------|
| `ccp find <query>` | Search skills.sh (`-r github` for GitHub) | `ccp find testing` |
| `ccp install [source] [items]` | Install from a source as shadow links, or replay `ccp.toml` + `ccp.lock`; `--link <profile>` also links it (`.` = active); `--clone` makes an editable local copy | `ccp install owner/repo skills/x --link .` |
| `ccp source add/list/update/remove` | Manage sources | `ccp source update` |

Workflow and discovery layouts: [source system](/features/source-system.md).

# Project, plugins, templates, linking, workflow

| Command | Description | Example |
|---------|-------------|---------|
| `ccp project add [items...] [-i]` | Copy hub items into the project's `.claude/` | `ccp project add skills/coding` |
| `ccp project install [source] [items...]` | Install from a source into the project | `ccp project install owner/repo skills/x` |
| `ccp project list` / `remove [items...]` | Inspect and remove project items | `ccp project remove skills/coding` |
| `ccp plugin list <owner/repo>` | List marketplace plugins | `ccp plugin list EveryInc/compound-engineering-plugin` |
| `ccp plugin add <source>` | Install a plugin | `ccp plugin add owner/repo@plugin-name` |
| `ccp plugin update [name]` | Update plugins | `ccp plugin update --all` |
| `ccp template list/show/create/extract/edit/delete` | Settings templates ([details](/reference/settings-templates.md)) | `ccp template extract opus --from default` |
| `ccp link [profile] [path]` | Link a hub item (interactive without args) | `ccp link quickfix skills/vue-dev` |
| `ccp unlink <profile> <path>` | Unlink a hub item | `ccp unlink quickfix skills/vue-dev` |
| `ccp auto [--path]` | Profile from `.ccp.yaml` (hidden) | `ccp auto --path` |
| `ccp session [<profile>]` | Shell with the profile active, optionally under an account (`auto` picks) (hidden) | `ccp session --account work2` |
| `ccp run [<profile>] [--account <name>\|auto] [-- <cmd>]` | Run a command (default `claude`) with the profile, optionally under an account; a named account brings its own profile, `auto` picks the freest (hidden) | `ccp run companyA --account auto` |

# omp (hidden)

| Command | Description | Example |
|---------|-------------|---------|
| `ccp omp link <items...>` | Link skills, agents, commands into `~/.omp/agent/` (picker without args) | `ccp omp link skills/debugging` |
| `ccp omp unlink <items...>` | Remove ccp links (picker without args, `--all` for everything) | `ccp omp unlink --all` |
| `ccp omp sync [profile]` | Mirror a profile and regenerate `RULES.md` | `ccp omp sync dev` |
| `ccp omp list [profile] [--json]` | Links and diff (missing/stale/ignored) | `ccp omp list` |
| `ccp omp --profile <name> <cmd>` | Target an omp named profile | `ccp omp --profile work sync` |
| `ccp doctor --verify-omp` | Ask a live omp what its prompt carries | `ccp doctor --verify-omp` |

Behavior: [omp integration](/features/omp/overview.md).

# opencode (hidden)

| Command | Description | Example |
|---------|-------------|---------|
| `ccp opencode sync [profile]` | Write converted copies of a profile's agents and commands, and its output style, into `~/.config/opencode` (default: active profile); opts in to following | `ccp opencode sync` |

Behavior: [opencode integration](/features/opencode.md).

# agy (hidden)

| Command | Description | Example |
|---------|-------------|---------|
| `ccp agy sync [profile]` | Link `AGENTS.md` and skills, write converted rules and agents and the output style (`GEMINI.md`) into `~/.gemini/config` (default: active profile); opts in to following | `ccp agy sync` |

Behavior: [agy integration](/features/agy.md).

# pi (hidden)

| Command | Description | Example |
|---------|-------------|---------|
| `ccp pi sync [profile]` | Write a profile's output style into pi's `APPEND_SYSTEM.md` (default: active profile); opts in to following | `ccp pi sync` |

Behavior: [output styles](/features/output-styles.md).

[^hidden-cmds]: Hidden command declarations
