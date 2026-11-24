---
type: Decision
title: "agy accounts use file-based token storage with --app_data_dir and SSH_CLIENT"
description: "2026-11-20: Account bindings gain harness = \"agy\" (default claude). Each agy account runs in ~/.gemini/antigravity-cli-ccp-<name>/ with SSH_CLIENT set, merged settings.json, and quota read via agy -p /usage."
tags: [decision, agy, accounts, activation]
decided: 2026-11-20
generated: { by: claude-code/claude-fable-5-2, at: 2026-11-24T05:00:00Z }
sources:
  - id: agy-auth-re
    resource: research/261119-agy-accounts/agy-auth-re.md
    title: agy 1.5.11–1.5.13 auth and quota reverse engineering and live verification
---

# Decision

Account bindings in `.ccp-account.toml` gain `harness = "agy"`; absent means `"claude"`. Each
`agy` account keeps its ccp metadata in `~/.ccp/accounts/<name>/` (`.ccp-account.toml`,
`.ccp-usage.json`, `.ccp-last-used`, with **no symlinks**) and its Antigravity state in
`~/.gemini/antigravity-cli-ccp-<name>/` (derived by `profile.AgyAppDataDirName`).

`profile.AgyLaunch` always returns both `--app_data_dir=antigravity-cli-ccp-<name>` and
`SSH_CLIENT=ccp 0 0` together:

- `ccp account add <name> [<profile>] --harness agy` creates both folders and writes the merged
  `settings.json`.
- `ccp run --account <name>` defaults to `agy`, inserts `--app_data_dir=...` immediately after the
  executable, sets `SSH_CLIENT=ccp 0 0`, keeps `CLAUDE_CONFIG_DIR` on the profile folder (the one
  named, else the bound one),
  refreshes `settings.json`, and touches `.ccp-last-used`. `--account auto` derives the candidates'
  harness from the command basename (`agy` vs everything else); with no command, see
  [harness detection](#harness-detection-for-auto-2026-11-24).
- `ccp env` and `ccp session` refuse `agy` accounts because shell/mise/direnv exports cannot pass
  `--app_data_dir`. `ccp use --accounts` stays Claude-only.
- Before each run (and at creation), `~/.gemini/antigravity-cli-ccp-<name>/settings.json` is written
  atomically (`0600`) from `~/.gemini/antigravity-cli/settings.json` while keeping extra account
  items in `permissions.allow`, `permissions.deny`, `permissions.ask`, and `trustedWorkspaces`.
- Usage (`internal/accountusage`) checks whether `antigravity-oauth-token` exists and is non-empty
  (without reading it) and runs `agy --app_data_dir=... -p /usage --output-format json` in `$HOME`
  under a 30s hard timeout, mapping `"Gemini Models"` to `five_hour`/`seven_day` (`(1 - remaining_fraction) * 100`)
  and other groups' weekly buckets to model-scoped entries.

# Any profile (2026-11-23)

An agy account shares nothing with the profile folder (no links, its own data dir), so it runs on
**any** profile. Running it with a profile other than its bound one needs no `--relink` and does
not rewrite the binding; the binding's profile is only the **default** used when no profile is
named (a missing default falls back to the current profile, an error when there is none).
`--relink` still rebinds explicitly. `ccp profile delete` no longer refuses for agy accounts
bound to the profile and prints a note instead. A binding still needs a non-empty profile. Pooled
picks (`--account auto:<pool> -- agy`) take agy members from every profile
([account pools](/decisions/account-pools.md)).

# Harness detection for auto (2026-11-24)

A named account always started its own harness. `ccp run --account auto` and `auto:<pool>` now do
the same in one case: when the user gave **no command** and **every readable candidate account**
(the pool's members, or the accounts bound to the profile) is agy, `ccp run` starts `agy` and
prints `ccp: every account of <pool X | profile Y> is agy; starting agy`. Mixed, all-Claude and
empty sets stay on `claude`, and an explicit command (`-- claude`, `-- agy`) is never changed, so
the `use --accounts` shim (which passes the real `claude`) behaves as before.

Rejected: picking the freest account across both harnesses. The program that starts would then
change from run to run with the quota, and Claude Code and agy are not interchangeable.

# Verified facts (agy 1.5.13)[^agy-auth-re]

1. `agy` stores its Google login in a single macOS Keychain item (`gemini`/`antigravity`) across all
   processes; no flag or env var selects another Keychain item.
2. When `SSH_CLIENT` (or `SSH_CONNECTION`/`SSH_TTY`) is set, `agy` bypasses the Keychain and stores
   the token at `<appDataDir>/antigravity-oauth-token` (`0600`).
3. Hidden flag `--app_data_dir=<name>` (relative under `~/.gemini`) moves `appDataDir` while leaving
   shared config (`~/.gemini/config`) shared.
4. Running `agy` with `SSH_CLIENT` and separate `--app_data_dir` values keeps multiple Google logins
   isolated concurrently; a keyring-mode run in the same `appDataDir` can purge the file token on
   refresh (agy issue #1514).
5. `agy --app_data_dir=<name> -p "/usage" --output-format json` returns quota groups and buckets in
   about 3s when signed in, but hangs ignoring `--print-timeout` when signed out.
6. `agy` writes `settings.json` via atomic rename, replacing symlinks on first write.
7. Setting `SSH_CLIENT` does not interfere with `harness-ops` (`termctl` reads no `SSH_*` vars).

# Alternatives rejected

- **Swapping the shared Keychain item** (e.g., `XiaoXioe/antigravity-cli-multi-account`,
  `PrimeFox59/agy-resilience-toolkit`): a running `agy` session holds its token in memory and writes
  it back on hourly refresh, silently reverting the swap.
- **`HOME` per account** (with its own Keychain): relocates config and credentials for every child
  tool `agy` spawns (`git`, shells, MCP servers).
- **ADC / API keys**: switches billing away from the subscription plan.

# Known limits

- Conversations, history, and MCP sign-ins live inside `~/.gemini/antigravity-cli-ccp-<name>/`, so
  `agy -c` continues only within one account.
- `SSH_CLIENT` is inherited by every child command `agy` runs.
- Depends on undocumented `agy` flags and behavior (`--app_data_dir`, SSH file-token trigger,
  `/usage` JSON schema); upstream issue `#1610` tracks official multi-account support.

[^agy-auth-re]: `research/261119-agy-accounts/agy-auth-re.md` and live verification on agy 1.5.13.
