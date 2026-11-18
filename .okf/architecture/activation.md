---
type: Mechanism
title: Profile activation
description: How a profile becomes active — per project through env files, globally through the ~/.claude symlink, or per command.
tags: [architecture, activation, profiles]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-17T10:00:00Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Modes

| Mode | Command | Effect |
|------|---------|--------|
| Project (default) | `ccp use dev` | Detects `mise.toml`/`.envrc` and writes `CLAUDE_CONFIG_DIR` |
| Global | `ccp use quickfix -g` | `~/.claude` → `~/.ccp/profiles/quickfix` (relative symlink) |
| Override | env var | `CLAUDE_CONFIG_DIR` beats the symlink |
| Account | `--account <name>` or `--account auto` on `run`, `session`, `env` | `CLAUDE_CONFIG_DIR` is `~/.ccp/accounts/<name>`, which links the profile but keeps its own Claude login; `auto` picks the account of the profile with the most plan limit left |

```bash
# mise (.mise.toml in the project root)
[env]
CLAUDE_CONFIG_DIR = "~/.ccp/profiles/quickfix"

# direnv (.envrc in the project root)
export CLAUDE_CONFIG_DIR="$HOME/.ccp/profiles/quickfix"

# one command
CLAUDE_CONFIG_DIR=~/.ccp/profiles/quickfix claude "fix the bug"
```

`ccp use -g` always targets `~/.claude` (`Paths.GlobalClaudeDir`), even inside a project that
sets `CLAUDE_CONFIG_DIR`. A global switch also mirrors the new profile into omp when the user
opted in ([profile switching](/features/omp/profile-switching.md)); project mode leaves omp alone.

# Accounts: one profile, several logins

Claude Code keeps one OAuth login per config folder (the Keychain entry is keyed by a hash of the
`CLAUDE_CONFIG_DIR` string; account info is `oauthAccount` in the folder's `.claude.json`). To run
one profile under several logins, `--account <name>` activates it through an account folder:

```bash
ccp run default --account work1 -- claude          # start Claude as work1
ccp run --account work1                            # the same: its own profile, claude by default
ccp run growtrics --account auto                   # the growtrics account with most limit left
eval "$(ccp env default --account work1)"          # export it in this shell
ccp env default --account work1 --format=mise      # write it into ./mise.toml
ccp session default --account work2                # a shell as work2
```

`~/.ccp/accounts/<name>/` holds `.ccp-account.toml` (the linked profile), its own `.claude.json`
(seeded once without `oauthAccount`), and absolute symlinks to every other profile entry. The
first `claude` run in a new account asks for `/login`. An account is linked to one profile at a
time; `--relink` moves it, after its sessions are closed. `ccp which` prints
`<profile> (account <name>, from CLAUDE_CONFIG_DIR)`. Rationale and details:
[accounts decision](/decisions/accounts-as-activation-option.md).

# Parallel sessions

Different terminals can run different profiles through the env override, while the
`~/.claude` symlink stays the fallback.

# Loading a profile's CLAUDE.md and rules

Claude Code loads `CLAUDE.md` and `rules/` from the active config folder (`CLAUDE_CONFIG_DIR`,
else `~/.claude`) as user memory, so no flag is needed. Verified on Claude Code 2.1.327: a
`claude -p` run with only `CLAUDE_CONFIG_DIR` set listed the profile's `CLAUDE.md` and every
linked rule.

ccp used to print `alias claude='… claude --add-dir "$CLAUDE_CONFIG_DIR"'` and write
`CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1` ([anti-pattern](/anti-patterns/add-dir-profile-folder.md)).
Since Claude Code 2.1.326 every `--add-dir` folder needs workspace trust, and a "Yes, add it"
answer lasts one session. Trust is saved only when Claude Code starts in a folder (or a non-git
parent such as `~` is trusted), in the `.claude.json` of the config folder in use, and each
profile and account folder has its own. So the alias prompted on every start unless that file
already trusted the profile folder or a parent: `~/.claude.json` usually trusts `~`, a
profile's own `.claude.json` usually does not.

Remove the alias from shell configs: it causes the prompt. The env var alone does nothing
unless you pass `--add-dir` yourself; `ccp use` no longer writes it and, since 0.53.1, removes
an old `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD` line from the `mise.toml` or `.envrc` it
updates. Set it yourself (outside those files) if you want `CLAUDE.md` loaded from folders you add.

`ccp config shell` prints the `ccp-use` function. Other entry points: `ccp session <profile>`
(a shell with the profile active), `ccp run <profile> -- <cmd>`, and `ccp auto` with
[`.ccp.yaml`](/reference/project-config.md).
