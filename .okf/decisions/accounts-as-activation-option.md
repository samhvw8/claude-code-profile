---
type: Decision
title: "Accounts are an option of Activation"
description: "One profile can run under several Claude logins through account folders, chosen with --account; ccp migrate --accounts moves existing logins. Supersedes the per-profile API key decision."
tags: [decision, activation, accounts]
decided: 2026-11-17
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-17T11:00:00Z }
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
- Every other profile entry, dotfiles included, is an absolute symlink into the profile. Skipped:
  `.claude.json`, `.ccp-account.toml`, `daemon*` (per-folder runtime files) and `.credentials.json`
  (the token file where there is no Keychain, which would share one login across accounts).
- Broken links and links into another profile (left from before a relink) are removed. A real
  file in the account folder is never replaced; ccp warns and leaves it.
- The returned path is absolute and clean, because that exact string is what Claude hashes.

`ccp which` and `ccp profile list` map an account folder back to its profile
(`profile.ConfigDirProfile`).

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
2. **Target.** The account folder from `LinkAccount`; its service hashes the account folder path. A
   target that already has a credential is skipped as "already migrated", so reruns are safe. An
   account linked to another profile is refused.
3. **Running sessions.** The profile is refused, with the PIDs, while a Claude Code process
   (`argv[0]` basename `claude`, including mise's `…/claude/<ver>/claude`) uses the source folder: for
   the global login, `CLAUDE_CONFIG_DIR` unset, `~/.claude`, or the profile; for an env login, the
   profile. A process whose environment cannot be read counts as using it. Run from inside Claude
   Code, the migration sees its own parent and refuses; that is intended.
4. **Move order.** Read the secret and its `acct` → write the target (`security -i` with the
   command on stdin, `-X <hex>`, so the secret never reaches argv; a 0600 file on Linux) → read it
   back and compare bytes → copy `oauthAccount` into the account's `.claude.json` (atomic, 0600) →
   delete the source → drop `oauthAccount` from the source state (atomic, mode kept). Any failure
   before the source delete removes the target credential and restores the account state.
5. **`--dry-run`** prints the plan and writes nothing, not even the account folder.

After a global login moves, ccp prints how to start it: `ccp run <P> --account <P> -- claude`.

Credential access (`CredentialStore`: Keychain or file) and process listing (`ProcessLister`: `ps`
on macOS, `/proc` on Linux) are interfaces so tests run on fakes and never touch the real Keychain.

# Alternatives refused

- **An `account` concept or command.** It would be a sixth concept; the need is one more way to
  activate a profile, so it is a flag.
- **Copying the login between folders.** The Keychain entry is keyed by the folder path, so a copy
  is a different login slot.
- **Swapping `.claude.json` and the Keychain entry in place.** That mutates the user's real login
  state and breaks any session running on the old account.
