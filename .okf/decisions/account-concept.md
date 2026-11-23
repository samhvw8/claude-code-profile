---
type: Decision
title: "Account is the sixth concept; the budget is seven for now"
description: "2026-11-18: the user-facing concept budget rises from five to seven, and Account becomes the sixth concept with a visible ccp account command (list and add; pool added 2026-11-23). Extended 2026-11-20 with read-only display of unmanaged other logins."
tags: [decision, concepts, accounts, command-surface]
decided: 2026-11-18
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-23T08:00:00Z }
---

# Decision

The budget of user-facing concepts is **seven**, "for now" (the user's words), up from five. Six
are in use: Hub, Profile, Settings Template, Source, Activation and **Account**
([core concepts](/product/core-concepts.md)). An eighth still requires removing one.

Account gets a visible command:

| Command | What |
|---|---|
| `ccp account` / `ccp account list [--json]` | Every account with its profile, email, 5-hour and weekly use, model-scoped weeks, age of the read and last use. With none, it prints how to add one. |
| `ccp account add <name> [<profile>] [--pool <p>]` | Creates the account folder for a profile (default: the current one). It starts logged out; `ccp run --account <name>` then `/login` logs it in once. An existing account is refused. |
| `ccp account pool <name> <pool>\|--clear` (2026-11-23) | Sets or clears an existing account's pool ([account pools](/decisions/account-pools.md)). A subcommand, not a flag on `add`, because `add` refuses an account that already exists. |

Everything else in [accounts are an option of Activation](/decisions/accounts-as-activation-option.md)
stands: the account folder layout, `--account <name>|auto` on `run`, `session` and `env`, the
picker, the usage reads and `migrate --accounts`. Only its "no account concept or command" part
is replaced by this decision.

# Why

The first accounts release (v0.52.0) had no way to discover accounts. `ccp` help listed none of
the account flags (they sit on hidden commands), `ccp status` showed an Accounts table only when
an account already existed, and there was no command to list them. The user looked for an
account list and found nothing. The five-concept rule had pushed a real, user-visible thing
(a login you choose) into a flag, and the flag was invisible.

# Alternatives considered

- **Keep five concepts; show an empty Accounts section in `status` and a line in root help.**
  Cheaper on the budget, but the user chose a real command.
- **Make Account replace another concept.** None of the five is redundant with it.
- **`ccp account remove`.** Left out for now: removing an account must also delete its Keychain
  login (macOS) or `.credentials.json` (Linux), and refuse while a session runs on it. Add it
  as its own change when needed.

# Consequences

- `.claude/rules/01-project-identity.md`, `02-design-principles.md` and the
  [design principles](/engineering/design-principles.md) say seven (six in use).
- Earlier concepts that say "not a sixth concept" (bundles, output styles, omp) were judged
  against the old budget of five; they are still not concepts.

# Other logins (added 2026-11-20)

`ccp account` (text output only) also lists unmanaged logins under **Other logins (not accounts)**
when they exist: Claude global (`Claude Code-credentials` on macOS, `~/.claude/.credentials.json` on
Linux), Claude profile-dir logins (`Claude Code-credentials-<sha256(profileDir)[:8]>` /
`<profile>/.credentials.json`), and plain `agy` on macOS (only when an attribute-only Keychain
lookup finds `gemini`/`antigravity`).

- **Read-only, never picked by `--account auto`, never in `status` or `--json`.** Adding unmanaged logins to `ccp status`
  would add ~3s cold `agy` latency and extra `/api/oauth/usage` calls to a quick health check.
  Putting shared/global logins into `--account auto` would burn personal quota from a profile's accounts
  and race with plain sessions. Keeping `ccp account --json` as the managed-only array preserves
  callers that pass `.name` to `--account`.
- **Cached in `~/.ccp/state/usage/<key>.json` (`0600`).** Reads reuse the 5-minute fresh window and
  failure cache without writing inside profile folders (which may be dotfiles-linked, and a global
  profile can hold two Claude logins while plain `agy` has no profile folder).
- **Guards on plain `agy`.** Before spawning `agy -p /usage --output-format json` (without
  `--app_data_dir`), `SSH_CLIENT=ccp 0 0` is stripped so calls inside a `ccp run --account <agy>`
  session do not trigger file-token mode; real SSH sessions (`SSH_CLIENT`, `SSH_CONNECTION`,
  `SSH_TTY`) run only when `~/.gemini/antigravity-cli/antigravity-oauth-token` is non-empty.
- **Footer hints by kind, never a blanket `ccp migrate --accounts`.** `AccountMigrator` refuses
  `agy` and migrates every profile at once (including the global login, logging out plain `claude`).
  Claude profile logins suggest `ccp migrate --accounts --dry-run` with that caveat; plain `agy`
  suggests `ccp account add <name> --harness agy`.
- **Orphan Keychain items in `ccp doctor`.** On macOS, `ccp doctor` checks `security dump-keychain`
  (attribute-only, never `-d`) for `Claude Code-credentials-<8 hex>` items whose hash matches no
  profile dir, account dir, `~/.claude`, or their symlink-resolved targets. Each is an informational
  `WARN` (never `FAIL`, never deleted by `--fix`) because a `CLAUDE_CONFIG_DIR` outside ccp may be
  intentional.
