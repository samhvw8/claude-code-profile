---
type: CLI Reference
title: Command flags
description: Flags for ccp commands that take more than a positional argument.
tags: [cli, reference, flags]
generated: { by: claude-code/claude-fable-5-2, at: 2026-11-24T05:00:00Z }
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
| `migrate` | `--accounts` | Run only the opt-in login move: each profile's Claude login (Keychain on macOS, `.credentials.json` on Linux) moves into `~/.ccp/accounts/<profile>`. Refused per profile while Claude Code runs on that login; idempotent ([decision](/decisions/accounts-as-activation-option.md)) |
| `doctor` | `--fix` | Fix what can be fixed: create missing hub dirs, reconcile drift, remove links (and manifest entries) whose hub item is gone. Only links directly inside the hub item dirs are checked — links inside a profile-local item, and `debug/`, `projects/` etc., are not ccp's |
| `doctor` | `--verify-omp` | Ask a live omp what its prompt carries |
| `reset` | `--force` | Skip confirmation |
| `use` | `-g, --global` | Update the global `~/.claude` symlink (default: project env) |
| `use` | `--show` | Show the current profile |
| `use` | `--accounts` | Project mode: write `.ccp/bin/claude`, put `.ccp/bin` on PATH (`mise.toml` `_.path` or `.envrc` `PATH_add`), add `.ccp/` to `.git/info/exclude`; plain `claude` then picks the profile's account with the most limit left. Not with `-g` |
| `use` | `--pool <p>` | With `--accounts`: the shim runs `ccp run --account auto:<p> --or-profile`; running `--accounts` again without it drops the pool |
| `which` | `--path` | Print only the profile directory (the account folder when an account is active) |
| `env` | `--format=shell\|mise\|direnv` | Print an export (default), or update `mise.toml` / `.envrc` |
| `account add` | `--harness claude\|agy` | Account harness: `claude` (default) or `agy` (file-mode token storage under `~/.gemini/antigravity-cli-ccp-<name>/`, no symlinks) |
| `account add` | `--pool <p>` | Put the new account in a pool (validated before the folder is created) ([decision](/decisions/account-pools.md)) |
| `account pool` | `--clear` | Take the account out of its pool instead of naming one |
| `account remove` | `--creds-only` | Delete only the login (Keychain item, `.credentials.json`, or the agy token); keep the account folder, its pool and the agy data dir ([decision](/decisions/account-concept.md)) |
| `account remove` | `--force`, `-f` | Skip the y/N question (needed with `--json` and without a terminal). It never skips the running-session check |
| `run`, `session`, `env` | `--account <name>` | Use an account folder `~/.ccp/accounts/<name>`: Claude accounts set `CLAUDE_CONFIG_DIR` to the account folder; `agy` accounts work only with `run` (inserting `--app_data_dir=...` and `SSH_CLIENT`) and are refused by `session` and `env` ([activation](/architecture/activation.md)). For `run`, put it before `--` |
| `run`, `session`, `env` | `--account auto` | Pick, among the accounts linked to the profile (the current one when none is named), the one with the most plan limit left: usage cached 5 minutes, else least recently used. On `run`, the harness comes from the command basename (`agy` vs everything else); with no command, `agy` starts when every candidate account is agy ([decision](/decisions/agy-accounts-file-token-mode.md)). `env`: shell format only, Claude-only ([decision](/decisions/accounts-as-activation-option.md)) |
| `run`, `session`, `env` | `--account auto:<pool>` | Pick among the pool's accounts with the most plan limit left. `run`: from any profile; agy accounts run as is, a Claude account linked to another profile is relinked only while no Claude Code process runs on it (fail closed), under a per-account flock released right after the start. `env`, `session`: only pool members already linked to the profile, never a relink ([decision](/decisions/account-pools.md)) |
| `run` | `--or-profile` | With `--account auto` or `auto:<pool>`: keep `CLAUDE_CONFIG_DIR` when it already names an account, and run on the profile when no candidate account is free and logged in, instead of failing. The `use --accounts` shim passes it |
| `run`, `session`, `env` | `--relink` | With `--account`: link an account that is linked to another profile (for agy, set its default profile). Close its sessions first |
| `auto` | `--path` | Print the profile path instead of its name |
| `harness list` | `-j, --json` | Output discovered custom harness plugins as JSON |

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
| `profile sync` | `--all` | All profiles (does not imply `--force`) |
| `profile sync` | `-f, --force` | Regenerate settings.json even if it drops uncaptured edits; hooks ccp did not create are kept ([settings](/reference/settings-templates.md)) |
| `profile edit` | `--add-<type>=a,b` / `--remove-<type>=a` | Add/remove skills, hooks, rules, commands |
| `profile edit` | `--template=<name>` | Set the settings template |
| `profile edit` | `-i, --interactive` | Picker (default with no flags) |
| `profile capture` | `--dry-run` | Show the fragment diff only |

# Hub, templates, plugins

| Command | Flag | Effect |
|---------|------|--------|
| `hub add` | `--from-profile=<name>` | Promote an item from a profile: copy to the hub, replace the profile copy with a link, record it in the manifest |
| `hub add` | `--replace` | Replace an existing item |
| `install`, `source install` | `-l, --link <profile>` | Also link the installed items to that profile; `.` is the active profile. Checked before anything is installed |
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
