---
type: Decision
title: "Accounts are an option of Activation"
description: "One profile can run under several Claude logins through account folders, chosen with --account; ccp migrate --accounts moves existing logins. Supersedes the per-profile API key decision."
tags: [decision, activation, accounts]
decided: 2026-11-17
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-17T12:00:00Z }
---

# Decision

A profile can run under several Claude Code logins. `--account <name>` on `ccp run`, `ccp session`
and `ccp env` activates the profile through an **account folder**, `~/.ccp/accounts/<name>/`, instead
of the profile folder ([activation](/architecture/activation.md)). Accounts are a flag of
Activation, not a sixth concept ([core concepts](/product/core-concepts.md)), and there is no
account command.

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
- `.claude.json` is the account's own. It is seeded once from the profile's state (`~/.claude.json`
  when `~/.claude` resolves to the profile, else `<profile>/.claude.json`) without
  `oauthAccount`, mode 0600, and never overwritten.
- Every other profile entry, dotfiles included, is an absolute symlink into the profile. The
  account owns these itself, so they are never linked:
  - `.claude.json` and every name starting with it (`.claude.json.backup*`, locks);
  - `backups/`, where Claude Code keeps `.claude.json` backups that carry `oauthAccount`;
  - `.ccp-account.toml`;
  - `daemon*` (per-folder runtime files);
  - `.credentials.json` (the token file where there is no Keychain).

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
   With no source left and a target present, the profile is skipped as "already migrated", so
   reruns are safe. A target that holds the **same** secret as a source that still exists is a
   move that stopped before the source delete: the run finishes it (delete the source, carry
   `oauthAccount`) and reports "moved (finished an interrupted move)". A target holding a
   **different** secret is refused ("both … hold a login; remove one by hand") and nothing changes.
   An account linked to another profile is refused. A source state file that is not JSON fails the
   profile before anything is written.
3. **Running sessions.** The profile is refused, with the PIDs, while a Claude Code process uses
   the source folder: for the global login, `CLAUDE_CONFIG_DIR` unset, `~/.claude`, or the profile;
   for an env login, the profile. A Claude Code process is `argv[0]` basename `claude` (mise's
   `…/claude/<ver>/claude` included), or a JS runtime (`node`, `node22`, `bun`, `deno`) whose
   `argv[1]` or `argv[2]` has basename `claude` or contains `@anthropic-ai/claude-code` (npm
   installs). On macOS the exact argv and environment come from `sysctl kern.procargs2` over the
   `kern.proc.all` table, so paths with spaces stay whole; on Linux from `/proc/<pid>/cmdline` and
   `environ`. A process that exited while being read (ESRCH, ENOENT) is dropped; any other read
   error keeps it with an unknown environment, which counts as using every login (on macOS only
   when its executable name could be Claude: `claude` or a JS runtime). Run from inside Claude
   Code, the migration sees its own parent and refuses; that is intended.
4. **Move order.** Read the secret and its `acct` → write the target (`/usr/bin/security -i` with
   the command on stdin, `-X <hex>`, so the secret never reaches argv; a 0600 file on Linux) → read
   it back and compare bytes → copy `oauthAccount` into the account's `.claude.json` (atomic, 0600)
   → delete the source → drop `oauthAccount` from the source state (atomic, mode kept). Any failure
   before the source delete removes the target credential and restores the account state. The
   Keychain delete is scoped by service **and** `acct` and repeated until no match is left (at most
   10), because `security` deletes one match per run; an item of the same service under another
   account is left and named in a warning. `security` runs by absolute path, and its errors never
   quote the secret (only its length) or the hex command.
5. **`--dry-run`** prints the plan and writes nothing, not even the account folder.

After a global login moves, ccp prints how to start it: `ccp run <P> --account <P> -- claude`.

A refused or failed profile makes the command exit non-zero with only the error line, no usage
block.

Credential access (`CredentialStore`: Keychain or file) and process listing (`ProcessLister`:
`sysctl` on macOS, `/proc` on Linux) are interfaces so tests run on fakes and never touch the real
Keychain.

**Known limits.**

- **Linux state file.** When `~/.claude` is the profile, the credential file is the same for both
  activation paths, but the state with `oauthAccount` may be `~/.claude.json` (plain `claude`) or
  `<profile>/.claude.json` (`CLAUDE_CONFIG_DIR`). The migration takes `~/.claude.json`; Claude Code
  fills in `oauthAccount` on first run if it was the other one.
- **Time of check to time of use.** Processes are listed once, before the moves; a Claude Code
  session started between the check and a move is not seen. Close sessions and do not start new
  ones while it runs.

# Alternatives refused

- **An `account` concept or command.** It would be a sixth concept; the need is one more way to
  activate a profile, so it is a flag.
- **Copying the login between folders.** The Keychain entry is keyed by the folder path, so a copy
  is a different login slot.
- **Swapping `.claude.json` and the Keychain entry in place.** That mutates the user's real login
  state and breaks any session running on the old account.
