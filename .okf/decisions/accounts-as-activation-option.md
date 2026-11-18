---
type: Decision
title: "Accounts are an option of Activation"
description: "One profile can run under several Claude logins through account folders, chosen with --account (or --account auto, the one with the most plan limit left); ccp migrate --accounts moves existing logins. Supersedes the per-profile API key decision."
tags: [decision, activation, accounts]
decided: 2026-11-17
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-18T14:30:00Z }
---

# Decision

A profile can run under several Claude Code logins. `--account <name>` on `ccp run`, `ccp session`
and `ccp env` activates the profile through an **account folder**, `~/.ccp/accounts/<name>/`, instead
of the profile folder ([activation](/architecture/activation.md)). Accounts are a flag of
Activation, not a sixth concept ([core concepts](/product/core-concepts.md)), and there is no
account command. (Since 2026-11-18 Account is a concept with a `ccp account` command:
[account concept](/decisions/account-concept.md). The mechanism below is unchanged.) With a named account the profile may be left out (the account's own profile is
used), and `ccp run` defaults its command to `claude`: `ccp run --account work1`.
`--account auto` picks the account with the most plan limit left (below).

This supersedes [API keys are per-profile](/decisions/api-keys-per-profile.md).

# Why the old decision no longer holds

The old reason was that copying an API key between profiles is free. An OAuth login is not a
value you can copy. Claude Code keys it by the config folder path: on macOS the token is in the
Keychain under `Claude Code-credentials-<first 8 hex of sha256(CLAUDE_CONFIG_DIR)>` and the account
info is `oauthAccount` in that folder's `.claude.json` (`~/.claude.json` when
`CLAUDE_CONFIG_DIR` is unset). One folder is one login. A user with three company accounts would
need three copies of the same profile and would have to keep them in sync by hand.

# How it works

`profile.LinkAccount` prepares the folder each time it is used:

- `.ccp-account.toml` records the profile the account is linked to. Linking it to another
  profile fails unless `--relink` is given: a running Claude session would see its files change
  under it. Crashing beats corrupting.
- The bound profile must be a single path segment, not `shared` and not empty.
- `.claude.json` is the account's own. It is seeded once from the profile's state (`~/.claude.json`
  when `~/.claude` resolves to the profile, else `<profile>/.claude.json`) without
  `oauthAccount`, mode 0600, and never overwritten. The seed is written to a temp file and
  hard-linked into place, so a crash or a Claude Code starting at the same moment never sees an
  empty or partial file. A top-level `null` state counts as an empty object. A symlink at
  `.claude.json` or `.credentials.json` in the account is removed first, with a warning.
- Every other profile entry, dotfiles included, is an absolute symlink into the profile. The
  account owns these itself, so they are never linked:
  - `.claude.json` and every name starting with it (`.claude.json.backup*`, locks);
  - `backups/`, where Claude Code keeps `.claude.json` backups that carry `oauthAccount`;
  - `.config.json`, which Claude Code 2.1.326 prefers over `.claude.json` when it is not empty;
  - `.credentials.json` (the token file where there is no Keychain) and `.storage-write*`, its
    write lock;
  - `daemon*`, `jobs/` and any `*.lock` (per-folder runtime state);
  - `.ccp-*`: ccp's own per-folder files (`.ccp-account.toml`, `.ccp-usage.json`,
    `.ccp-last-used`).

  Linking any of them would share one login's data across accounts, or make Claude write the
  account's backups into the profile.
- Broken links, links into another profile (left from before a relink), and links with one of
  the names above (made by an older version) are removed. A real file or folder in the account is
  never replaced; ccp warns and leaves it.
- Every check (binding, seed state parses) runs before the folder is created, so a refusal or a
  corrupt state file leaves nothing behind. `--account` given with an empty name is an error, never
  the profile folder.
- The returned path is absolute and clean, because that exact string is what Claude hashes.

`ccp which` and `ccp profile list` map an account folder back to its profile
(`profile.ConfigDirProfile`), comparing absolute, clean paths (a relative `CCP_DIR`, a trailing
slash in `CLAUDE_CONFIG_DIR`).

## Keeping accounts in step with their profile

- **Every profile change relinks that profile's accounts**, whichever profile it is, at the
  point the folder changes: `Manager.LinkHubItem`/`UnlinkHubItem`/`LinkHubBundle`/
  `UnlinkHubBundle`, drift `Fix` (so `profile fix`, `doctor --fix` and bootstrap's fix of every
  profile), `ApplySettings`, and `hub remove --copy` all call `RefreshAccounts`. `install --link
  <profile>` reaches it through `LinkHubItem`. An account used only through mise, direnv or a
  shell export therefore sees new top-level entries without rerunning `--account`, before
  Claude can create a real file in their place. Only failures are printed there; entries an
  account holds as real files are reported by `run`/`session`/`env --account` and by
  `followHarnesses`, once per command.
- **Profile delete** refuses while the profile is the `~/.claude` target **or** this shell's
  current profile (`CLAUDE_CONFIG_DIR`, a profile or account folder): either way deleting it
  breaks a live configuration. It also refuses while an account is bound to it (listing
  `ccp env <other> --account <a> --relink` for each), and while **any** account's binding cannot
  be read, since that account may be bound to it.
- **Profile rename** validates the new name first (one path segment, not `shared`, not an
  existing profile), moves every bound account to it, and warns about each account whose
  binding cannot be read. It refuses when this shell's `CLAUDE_CONFIG_DIR` is the profile folder
  itself, which ccp cannot rewrite; the `~/.claude` link is swapped when it points at the profile,
  whatever `CLAUDE_CONFIG_DIR` says.
- **Bindings are validated.** A `.ccp-account.toml` naming something that cannot be a profile
  (`shared`, `../hub`) counts as unreadable, like a corrupt one.
- **`ccp doctor`** reports accounts with an unreadable binding, a missing profile, or broken links.
- **`ccp reset`** lists the account folders it deletes with `~/.ccp` and where each login is:
  the Keychain item `Claude Code-credentials-<hash>` is left orphaned on macOS, the
  `.credentials.json` file is deleted elsewhere. Logins are not moved back; the plan's
  confirmation covers them.

## Inside an account session

There `CLAUDE_CONFIG_DIR` is a real folder, so `paths.ClaudeDir` is not a symlink. The same was
already true for `CLAUDE_CONFIG_DIR=<profile folder>`; both are handled the same way:

- `ccp init` and the init migrator refuse a **real** config folder inside `~/.ccp`
  (`GuardClaudeDir`) instead of moving its entries and deleting it. A symlinked `~/.claude`, the
  normal state of an initialized machine, is not refused there and keeps init's own "already a
  symlink" messages. "Inside" is lexical or resolved, against `~/.ccp` and the resolved
  `profiles/` and `accounts/`, so a real `~/.ccp` whose `profiles/` links into a dotfiles repo is
  still covered.
- `ccp reset`, `ccp doctor` and `ccp status` work on `~/.claude` itself (`GlobalClaudeDir`);
  `status` marks a target that no longer exists as "(missing)". `profile list` takes its
  "active (symlink)" marker from it.
- **The global harnesses follow `~/.claude` only.** pi, omp, opencode and agy mirror the profile
  `~/.claude` points at (`globalActive` in `follow.go`), whatever `CLAUDE_CONFIG_DIR` says: a
  command run from an account or project session on another profile must not push that profile
  into every other harness session. Account relinking is separate and per changed profile.
- Guards for a profile in use (profile delete) use `Manager.GetCurrent` (the symlink target, or
  the profile a `CLAUDE_CONFIG_DIR` folder belongs to) together with the `~/.claude` target.
  `GetActive` still sees only symlinks, so default-profile choices elsewhere are unchanged.

# Picking an account: `--account auto`

For a pool of accounts on one profile (three company logins on `companyA`, say), `--account
auto` on `run`, `session` and `env` (shell format only: mise and direnv would freeze one pick)
picks among the accounts linked to the profile. With no profile named it uses the current one:
`CLAUDE_CONFIG_DIR` (a project's `mise.toml`, or the account folder of the running session),
else `~/.claude`. `auto` is reserved and cannot name an account. The pick is printed on stderr:
`ccp: account gt2 (7d 30%)`.

**Usage source (A).** Each logged-in account's token is read from where Claude Code keeps it
(the Keychain item of the account folder, or its `.credentials.json`) and sent to `GET
https://api.anthropic.com/api/oauth/usage` with `anthropic-beta: oauth-2025-04-20`, the
endpoint Claude Code's own `/usage` reads (also used by ccburn, claude-usage-line and others).
It is undocumented. A read in November 2026 for a team plan returned `five_hour: null`,
`seven_day.utilization` (percent) and a `limits[]` list (`weekly_all`, and `weekly_scoped` per
model, such as Fable), so ccp takes `five_hour` and `seven_day`, falls back to the
`weekly_all` limit for the week, and keeps model-scoped weeks for `ccp status` only (whether they
matter depends on the session's model). Reads run in parallel with a 5-second timeout and never
follow a redirect. ccp never refreshes a token or writes a login: an expired access token is
not sent ("token expired; Claude Code renews it when it starts").

Each account folder keeps `.ccp-usage.json`: the last good read, and the time and error of the
last failed try. A read younger than 5 minutes is reused without a request, and so is a failure:
the endpoint answers 429 to a burst and also to a token it does not accept. Errors never quote
the token or the response body.

**Ranking.** Logged-in accounts only (a credential with no `claudeAiOauth` is not a login).
A window whose `resets_at` has passed counts as 0.

1. usage known, under 80% of the 5-hour and 95% of the 7-day limit: lowest 5-hour, then 7-day;
2. usage unknown (never read, or every read failed);
3. usage known and above those marks: lowest first;
4. a limit used up (the pick still runs and says so).

Ties go to the account ccp started least recently (`.ccp-last-used`, touched by `run`,
`session` and `env` with an account), so with no usage data at all the pick rotates **(B)**.

`ccp status` lists every account with its profile, email, 5-hour and 7-day use, model-scoped
weeks, the age of the read and the last use, plus the reason a logged-in account has no live
read.

**One workspace, one pool.** `ccp use <profile> --accounts` (since 0.55.0) sets a project folder up
so plain `claude` there picks the profile's account: it writes `.ccp/bin/claude`, puts `.ccp/bin`
on PATH through `mise.toml` `_.path` (or `PATH_add` in `.envrc`) and adds `.ccp/` to
`.git/info/exclude`. The shim names no profile and runs `ccp run --account auto --or-profile --
<real claude> "$@"`. `--or-profile` keeps `CLAUDE_CONFIG_DIR` when it already names an account (a
session's child) and runs on the profile while no account of the pool is logged in, instead of
failing. The real claude comes from `mise which claude`, never a mise shim
([anti-pattern](/anti-patterns/mise-shim-in-claude-wrapper.md)). Plain `claude` elsewhere keeps the
personal login.

# Moving existing logins: `ccp migrate --accounts`

An opt-in migration moves each profile's existing login into the account of the same name
(profile `default` → account `default`). Plain `ccp migrate` and `ccp bootstrap` never run it:
moving the default login logs out plain `claude`.

Per profile (`shared` and non-profiles are skipped):

1. **Source.** The global login when `~/.claude` resolves to the profile (macOS Keychain service
   `Claude Code-credentials`, state `~/.claude.json`; Linux `~/.claude/.credentials.json`), else the
   one keyed by the profile path (`Claude Code-credentials-<sha256(profile dir)[:8]>`, state
   `<profile>/.claude.json`; Linux `<profile>/.credentials.json`). When both exist the global one
   moves and the other is left, with a warning. No credential: skipped, "no login".
2. **Target.** The account folder from `LinkAccount`; its service hashes the account folder path.
   The binding is checked first, so a dry run reports what the real run would: an account linked
   to another profile is refused, an unreadable binding fails. When a move completes, the login it
   moved is recorded in `.ccp-account.toml` as `migrated_from` (the Keychain service or
   credentials file), written **before** the source is deleted and cleared by a rollback. On a
   later run with a credential already in the account:
   - with `migrated_from` set, only that login may be finished: if it still exists with the same
     secret, the run finishes the interrupted move ("moved (finished an interrupted move)");
     otherwise the profile is "already migrated". Any other login of the profile (the env login
     a global profile also had) is left untouched with a warning, never moved or deleted;
   - without a record (a move interrupted before recording), the default source is finished when
     it holds the same secret, refused when it holds a different one ("both … hold a login; remove
     one by hand"), and "already migrated" when it has vanished, since the account then holds the
     only copy.

   Logins compare by their `claudeAiOauth` value, not the whole secret. A target credential
   holding only MCP server tokens (`mcpOAuth`, from a session run in the account before the move)
   is not a login: the move writes the source login merged with those tokens (the account's entry
   wins for a server both hold), and a rollback writes the old tokens back.

   A source state file that is not JSON fails the profile before anything is written. A source
   folder (for the global login, `~`) with a non-empty `.config.json` is refused: Claude Code
   2.1.326 reads that file instead of `.claude.json`, and ccp does not move state between them.
   `LinkAccount`'s seed warns about such a file and seeds from `.claude.json` only.

3. **Running sessions.** The profile is refused, with the PIDs, while a Claude Code process uses
   the source folder: for the global login, `CLAUDE_CONFIG_DIR` unset, `~/.claude`, or the profile;
   for an env login, the profile. Folders compare equal when they are the same once absolute and
   clean, or resolve to the same folder through symlinks. A Claude Code process is `argv[0]`
   basename `claude` (mise's `…/claude/<ver>/claude` included), a native install started through
   its resolved path (exec path under `/claude/versions/` or `/installs/claude/`, from
   `procargs2`'s exec path or `/proc/<pid>/exe`), or a JS runtime (`node`, `node22`, `bun`,
   `deno`) with any later argument whose basename is `claude` or that contains
   `@anthropic-ai/claude-code` (npm installs). On macOS the exact argv and environment come from `sysctl kern.procargs2` over the
   `kern.proc.all` table, so paths with spaces stay whole; on Linux from `/proc/<pid>/cmdline` and
   `environ`. A process that exited while being read (ESRCH, ENOENT) is dropped; any other read
   error keeps it with an unknown environment, which counts as using every login (on macOS only
   when its executable name could be Claude: `claude` or a JS runtime). Run from inside Claude
   Code, the migration sees its own parent and refuses; that is intended. A Claude Code process
   whose `CLAUDE_CONFIG_DIR` is the **target** account folder also refuses the profile: a session
   there, even one waiting at `/login`, would write its own login under the move.
4. **Move order.** Read the secret and its `acct` → write the target (`/usr/bin/security -i` with
   the command on stdin, `-X <hex>`, so the secret never reaches argv; a 0600 file on Linux) → read
   it back and compare bytes → copy `oauthAccount` into the account's `.claude.json` (atomic, 0600)
   → delete the source → drop `oauthAccount` from the source state (atomic, mode kept). Any failure
   before the source delete removes the target credential and restores the account state. The
   Keychain delete is scoped by service **and** `acct` and repeated until no match is left (at most
   10), because `security` deletes one match per run; an item of the same service under another
   account is left and named in a warning. `security` runs by absolute path, and its errors never
   quote the secret (only its length) or the hex command.

   Reading follows Claude Code: `find-generic-password -a <current user> -s <service>` first,
   and only when that finds nothing, `-s <service>` alone with the account it reports. Writing
   goes through `security -i` on stdin unless the command line would pass 4000 bytes:
   `security -i` reads lines into a 4096-byte buffer, so a large login (MCP OAuth tokens push it
   past about 2 KB, and hex doubles it) goes through argv instead, exactly as Claude Code itself
   does past 4032 bytes. The hex is then visible to `ps` while `security` runs; that is Claude
   Code's own trade-off. State files whose top level is `null` count as empty objects. A
   rollback of a resumed move also removes `oauthAccount` from the account, which the interrupted
   run had already written: an account never keeps account info without its credential.
5. **`--dry-run`** prints the plan and writes nothing, not even the account folder.

After a global login moves, ccp prints how to start it: `ccp run <P> --account <P> -- claude`.

A refused or failed profile makes the command exit non-zero with only the error line, no usage
block.

Credential access (`CredentialStore`: Keychain or file) and process listing (`ProcessLister`:
`sysctl` on macOS, `/proc` on Linux) are interfaces so tests run on fakes and never touch the real
Keychain.

**Known limits.**

- **Seeded caches.** The seed copies the profile's whole `.claude.json` except `oauthAccount`,
  so org-scoped caches of the other login come along; Claude Code refreshes them for the new
  login.
- **A failed write leaves a seeded folder.** `LinkAccount` runs before the credential write; if
  the write fails and is rolled back, the account folder stays, seeded and linked but with no
  login. That is harmless: the next run moves into it, or `/login` fills it.
- **Why the migration spans several files.** [Over-modularized migrations](/anti-patterns/over-modularized-migrations.md)
  warns against a file per migration type. There is one migration here (`account_migrator.go`);
  `credentials.go` (Keychain and file stores) and `processes.go` (the Claude process listers) are
  adapters behind interfaces that tests replace, and `processes_darwin.go`/`processes_other.go`
  exist because the `sysctl` calls only build on macOS. Merging them would put build-tagged
  code into the migration and hide the two seams the tests depend on.

- **Linux state file.** When `~/.claude` is the profile, the credential file is the same for both
  activation paths, but the state with `oauthAccount` may be `~/.claude.json` (plain `claude`) or
  `<profile>/.claude.json` (`CLAUDE_CONFIG_DIR`). The migration takes `~/.claude.json`; Claude Code
  fills in `oauthAccount` on first run if it was the other one.
- **Time of check to time of use.** Processes are listed once, before the moves; a Claude Code
  session started between the check and a move is not seen. Close sessions and do not start new
  ones while it runs.

# Alternatives refused

- **An `account` concept or command.** It would be a sixth concept; the need is one more way to
  activate a profile, so it is a flag. Reversed on 2026-11-18 by
  [account concept](/decisions/account-concept.md): the flag alone left accounts undiscoverable.
- **Copying the login between folders.** The Keychain entry is keyed by the folder path, so a copy
  is a different login slot.
- **Swapping `.claude.json` and the Keychain entry in place.** That mutates the user's real login
  state and breaks any session running on the old account.
- **Usage from the statusline (C).** Claude Code passes `rate_limits` to a statusline command,
  which needs no extra request, but only while a session runs: an idle account's data would be
  hours old, and it needs a wrapper chained before the user's own statusline. The usage endpoint
  with a 5-minute cache costs one request per account per pick at most.
- **A gateway or proxy that switches accounts per request.** Sessions would change login mid-way;
  ccp picks once, at start. Remuda (`ya-luotao/remuda`) is a standalone TUI for the same
  multi-login layout.
