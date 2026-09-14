---
type: CLI Reference
title: Command flags
description: Flags for ccp commands that take more than a positional argument.
tags: [cli, reference, flags]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md, Command Flags (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Notes

Migrated from the spec. The `ccp skills update` flags were dropped: that command group was
removed in v0.28 ([release history](/product/release-history.md)). Commands:
[command reference](/cli/command-reference.md).

# Setup and health

| Command | Flag | Effect |
|---------|------|--------|
| `init` | `--dry-run` | Show the migration plan only |
| `init` | `--force` | Overwrite an existing hub structure |
| `migrate` | `--dry-run` | Show what would migrate |
| `doctor` | `--fix` | Fix what can be fixed (missing hub dirs, broken symlinks) |
| `doctor` | `--verify-omp` | Ask a live omp what its prompt carries |
| `reset` | `--force` | Skip confirmation |
| `use` | `-g, --global` | Update the global `~/.claude` symlink (default: project env) |
| `use` | `--show` | Show the current profile |
| `which` | `--path` | Print only the profile directory |
| `env` | `--format=shell\|mise\|direnv` | Print an export (default), or update `mise.toml` / `.envrc` |
| `auto` | `--path` | Print the profile path instead of its name |

# Profiles

| Command | Flag | Effect |
|---------|------|--------|
| `profile create` | `--skills=a,b` / `--hooks=x,y` / `--rules=p,q` | Items to include |
| `profile create` | `--from=<profile>` | Copy another profile's configuration |
| `profile create` | `--template=<name>` | Use a settings template |
| `profile create` | `-e, --empty` | No hub items |
| `profile create` | `-i, --interactive` | Picker (default with no flags) |
| `profile fix` | `--dry-run` | Preview |
| `profile fix` | `-f, --force` | Drop non-existent hub items from the manifest without asking |
| `profile fix` | `--all` | All profiles; skips hub_missing prompts (add `--force` to auto-remove) |
| `profile sync` | `--all` | All profiles |
| `profile edit` | `--add-<type>=a,b` / `--remove-<type>=a` | Add/remove skills, hooks, rules, commands |
| `profile edit` | `--template=<name>` | Set the settings template |
| `profile edit` | `-i, --interactive` | Picker (default with no flags) |
| `profile capture` | `--dry-run` | Show the fragment diff only |

# Hub, templates, plugins

| Command | Flag | Effect |
|---------|------|--------|
| `hub add` | `--from-profile=<name>` | Promote an item from a profile |
| `hub add` | `--replace` | Replace an existing item |
| `hub remove` | `-i` | Picker |
| `hub remove` | `--force` | Skip confirmation and usage check |
| `hub remove` | `--copy` | Copy into affected profiles first, no prompt |
| `hub show` | `-i` | Picker |
| `hub protect` | `-i` / `-l, --list` | Picker / list protected items |
| `hub prune` | `-f` / `-i` / `--type=<type>` | Remove all orphans / pick / one type; protected items are skipped |
| `hub update` | `--all` / `--force` / `--dry-run` | All items / override local changes / preview |
| `template create` | `--from-file=<path>` | From a JSON file (otherwise `$EDITOR`) |
| `template extract` | `--from=<profile>` | Source profile (default: active) |
| `plugin add` | `--select` | Choose components interactively |
| `plugin update` | `--all` / `--force` / `--dry-run` | All plugins / override local changes / preview |
| `omp` | `--profile=<name>` | Target an omp named profile |
| `omp list` | `--json` | Machine-readable output |
| `omp unlink` | `--all` | Remove everything ccp set up in omp |
