---
type: Decision
title: "Account pools: auto:<pool> picks across profiles"
description: "2026-11-23: An account may name one pool; --account auto:<pool> picks the pool's freest account whatever profile it is linked to. ccp run relinks a free Claude account (process scan plus a flock); env and session never relink; agy accounts run on any profile."
tags: [decision, accounts, activation, pools]
decided: 2026-11-23
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-23T08:00:00Z }
---

# Decision

`.ccp-account.toml` gets an optional `pool = "<name>"` (same name rule as accounts, `auto`
reserved). `--account auto:<pool>` picks, among the accounts of that pool with the command's
harness, the one with the most plan limit left (the same ranking as plain `auto`), **whatever
profile each one is linked to**. Plain `auto` is unchanged: the accounts linked to the profile.
Account names cannot contain `:`, so `auto:<pool>` never collides with a name. One parser,
`profile.ParseAutoAccount`, reads the value everywhere; `auto:` with no pool is an error.

A pool is an attribute of an [Account](/decisions/account-concept.md), not a new concept: the
budget stays at six of seven.

Set it with `ccp account add <name> [<profile>] --pool <p>` or `ccp account pool <name> <p>`
(`--clear` removes it). Every binding writer (`LinkAccount`, the agy relink) carries the
existing binding forward and changes only the profile and harness, so a relink keeps the pool
(and `migrated_from`).

# How a pick works

- **agy accounts** run on any profile without a relink ([agy accounts](/decisions/agy-accounts-file-token-mode.md)),
  so a pooled agy pick is just the pick.
- **Claude accounts, `ccp run` only.** A member linked to the requested profile is usable as
  today, even while a session runs on it (parallel sessions on one account and profile). A member
  linked to another profile is usable only when no running Claude Code process has
  `CLAUDE_CONFIG_DIR` equal to its folder (absolute-clean or symlink-resolved, `profile.SameDir`).
  The scan is the one `migrate --accounts` uses (`migration.NewProcessLister`: sysctl on macOS,
  `/proc` on Linux). It fails closed: no lister, a failed scan, or any process whose environment
  cannot be read make every cross-profile member busy. Busy members are dropped before the usage
  read.
- **Race safety.** The pick takes an exclusive `flock` on `<account>/.ccp-relink.lock`, re-reads
  the binding and re-scans under it, relinks with `LinkAccount(relink=true)` when needed, starts
  the process, then releases the lock and waits. The lock file is opened close-on-exec, so the
  session never holds it. A failed re-check releases the lock and tries the next candidate. The
  stderr line names the pool and any relink: `ccp: account gt2 (pool company, relinked from
  companyA, 7d 30%)`. A member whose lock cannot be taken, whose re-check fails, or whose
  relink fails is a lost candidate: the next is tried, and with none left the error is the
  "no usable account" one that `--or-profile` falls back on.
- **Every Claude link takes the same lock.** Named `--account`, plain `auto`, `ccp env` and
  `ccp session` hold the account's lock around `LinkAccount` (`ccp run` through the start, env
  and session only around the link), so they never interleave with a pooled relink; under the
  lock `LinkAccount` re-reads the binding and keeps its refusal of an account linked to another
  profile. `ccp account pool` rewrites the binding under it too. An account folder that does not
  exist yet is not locked. The lock uses `flock` on darwin, linux and the BSDs; elsewhere
  (Windows, Solaris, illumos, AIX) pooled Claude members count as lost and other callers run
  unlocked, as before pools.
- **What the scan sees.** A process counts as Claude Code by its argv (see
  `migration.isClaudeProcess`), including an npm install right after its `#!/usr/bin/env node`
  shebang (`env`, its options and assignments are skipped). Other wrappers (`bash -c claude`,
  `npx`) are not seen through. A `CLAUDE_CONFIG_DIR` that is not absolute cannot be compared
  (it is relative to that process's directory) and counts as busy.
- **Bad pool values.** A pool value that is not a valid name makes the binding unreadable.
  `ccp account pool <name> <pool>` or `--clear` repairs a binding whose only fault is the pool,
  and pool errors name every account whose binding cannot be read.
- **`ccp env` and `ccp session` never relink across profiles**: only pool members already linked
  to the requested profile are candidates, and the error says so when there are none.
- `ccp use <profile> --accounts --pool <p>` writes the shim with `ccp run --account auto:<p>
  --or-profile`; with every member busy or logged out it runs on the profile.

For pooled Claude accounts the binding's profile now means "the profile it was last linked to",
so plain `auto`, `AccountsBoundTo`, the PROFILE column and profile delete/rename see them move
([accounts decision](/decisions/accounts-as-activation-option.md)).

# Known limits

- **A session ccp cannot see.** A shell or `mise.toml` that exports `CLAUDE_CONFIG_DIR` pinned to
  a pooled account, with no Claude Code process running yet, is invisible to the scan: a pooled
  pick from another profile can relink the account under it, and the next `claude` started
  there runs with the other profile's links. Pin a folder to a pooled account only if it is not
  shared with another profile, or use the shim (`ccp use --accounts --pool`).
- The binding is written before the links on a relink (as before pools).
- `migrate --accounts` writes `migrated_from` without the lock.

# Why

A user with company seats used across several client profiles had to create one account per
profile or relink by hand. A pool lets one set of logins serve every profile while keeping the
rule that matters: never change an account folder's links under a running session.

- **A single pool string, not a tag list.** Overlapping pools let a pick for one client spend a
  seat meant for another, and no workflow needs an account in two pools.
- **A process scan plus a flock, not a lock alone.** Sessions started through mise, direnv or a
  plain export never pass through ccp and hold no lock; only the scan sees them. The lock closes
  the window between two ccp launches.
- **Relink only in `ccp run`.** `env` and `session` write a folder into a shell or file that
  outlives the command, so a relink there would move an account under a session ccp cannot see
  start.

# Alternatives rejected

- **One folder per account and profile pair.** Each pair needs its own `/login`, its own
  Keychain item and its own usage read; the user would log in N×M times.
- **Tags (several pools per account).** See above.
- **Lock only.** Misses every session not started by ccp.
