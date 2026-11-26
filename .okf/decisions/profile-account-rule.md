---
type: Decision
title: "A profile's account rule: plain claude picks the account wherever the profile loads"
description: "2026-11-26: ccp account use binds an account rule (<name>, auto, auto:<pool>) to a profile's profile.toml; the global shim ~/.ccp/bin/claude runs plain claude on it, globally and in projects. Deprecates ccp use --accounts."
tags: [decision, accounts, activation]
decided: 2026-11-26
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-26T13:30:00Z }
---

# Decision

Two commands, one job each:

| Command | Decides |
|---|---|
| `ccp use <p> [-g]` | Which **profile** a place loads (globally through `~/.claude`, or a project through `CLAUDE_CONFIG_DIR`) |
| `ccp account use <rule> [--profile p] [--clear]` | Which **account** that profile runs on, wherever it loads |

The rule is one field in the profile's manifest, `account = "<rule>"`
([profile.toml](/reference/profile-manifest.md)):

| Rule | Plain `claude` runs on |
|---|---|
| `<name>` | That account. It must exist, be a Claude account and be linked to the profile; `account use` refuses anything else. |
| `auto` | The profile's account with the most plan limit left |
| `auto:<pool>` | The pool's account with the most limit left, relinked to the profile when no session runs on it ([account pools](/decisions/account-pools.md)) |
| none | The profile, exactly as before |

Without `--profile` the rule goes to the current profile (`Manager.GetCurrent`: the
`CLAUDE_CONFIG_DIR` folder's profile, else the `~/.claude` target), the same rule `ccp run
--account auto` uses. `ccp account use` with no value prints the rule; `ccp account` lists every
profile's rule under **Account rules**.

# How plain claude follows it

`ccp account use` writes the shim `~/.ccp/bin/claude` (`globalShimScript`, the same template as
the project shim). The user puts `~/.ccp/bin` first on PATH once; ccp prints a mise `_.path` line
when `claude` does not resolve to the shim, and never edits shell or mise files. The shim runs
`ccp run --profile-rule -- <real claude> "$@"`. `--profile-rule` is hidden and takes no profile,
`--account`, `--relink` or `--or-profile`. `applyProfileRule`:

1. **Inside an account session** (`CLAUDE_CONFIG_DIR` is an account folder): keep it, as
   `--or-profile` does. Sub-agents and `!` commands stay on the session's login.
2. **No ccp profile, or no rule:** run the command with the environment unchanged.
3. **A rule that no longer holds** (the named account was removed or relinked elsewhere): a line
   on stderr, then run unchanged. Never silent, never a failed start.
4. **`auto` / `auto:<pool>`:** `--or-profile` semantics, except that with no usable account the
   command runs with the environment **unchanged** (`plainFallback`), with a line on stderr. The
   old `--or-profile` fallback set `CLAUDE_CONFIG_DIR` to the profile folder, which for the global
   profile would switch Claude Code from the global login to the profile-path login.
5. **A named account:** the normal `--account <name>` path, on the rule's profile.

The real claude is found as in the project shim: `mise which claude`, else the next `claude` on
PATH that is not in the shim's folder, a mise shims folder or any `.ccp/bin`
([anti-pattern](/anti-patterns/mise-shim-in-claude-wrapper.md)).

# Why

- `ccp use -g --accounts` was proposed first and rejected by the user: one command would switch the
  profile and the account, which mixes two concepts. Profile and account now each have their own
  `use`.
- A per-folder account setting is not needed: a folder already chooses its profile, and the
  profile chooses its account. A folder that needs other accounts gets its own profile.
- The rule lives in `profile.toml`, not a new file: no new concept (Account and Activation
  already exist) and no new sync step.

# Consequences

- `ccp use --accounts` and `--pool` are deprecated (hidden, with a cobra deprecation note). They
  still write a project shim, so existing projects keep working.
- `profile.toml` may be shared through dotfiles while account logins are per machine. On a
  machine without the named account, plain claude runs unchanged with a line on stderr.
- A ccp older than this change ignores the `account` key when it reads `profile.toml` but drops it
  when it rewrites the file (for example on a hub link). Install the release before setting rules.
- The shim adds one `ccp run` per `claude` start; usage reads stay cached 5 minutes per account.
