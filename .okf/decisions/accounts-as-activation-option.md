---
type: Decision
title: "Accounts are an option of Activation"
description: "One profile can run under several Claude logins through account folders, chosen with --account; it supersedes the per-profile API key decision."
tags: [decision, activation, accounts]
decided: 2026-11-17
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-17T10:00:00Z }
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

# Alternatives refused

- **An `account` concept or command.** It would be a sixth concept; the need is one more way to
  activate a profile, so it is a flag.
- **Copying the login between folders.** The Keychain entry is keyed by the folder path, so a copy
  is a different login slot.
- **Swapping `.claude.json` and the Keychain entry in place.** That mutates the user's real login
  state and breaks any session running on the old account.
