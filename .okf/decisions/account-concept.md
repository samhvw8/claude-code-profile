---
type: Decision
title: "Account is the sixth concept; the budget is seven for now"
description: "2026-11-18: the user-facing concept budget rises from five to seven, and Account becomes the sixth concept with a visible ccp account command (list, add). The --account activation mechanism is unchanged."
tags: [decision, concepts, accounts, command-surface]
decided: 2026-11-18
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-18T14:30:00Z }
---

# Decision

The budget of user-facing concepts is **seven**, "for now" (the user's words), up from five. Six
are in use: Hub, Profile, Settings Template, Source, Activation and **Account**
([core concepts](/product/core-concepts.md)). An eighth still requires removing one.

Account gets a visible command:

| Command | What |
|---|---|
| `ccp account` / `ccp account list [--json]` | Every account with its profile, email, 5-hour and weekly use, model-scoped weeks, age of the read and last use. With none, it prints how to add one. |
| `ccp account add <name> [<profile>]` | Creates the account folder for a profile (default: the current one). It starts logged out; `ccp run --account <name>` then `/login` logs it in once. An existing account is refused. |

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
