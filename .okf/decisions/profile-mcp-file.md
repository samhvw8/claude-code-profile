---
type: Decision
title: "Shared MCP servers: a profile mcp.json passed with --mcp-config"
description: "2026-11-23: A profile's mcp.json holds the MCP servers every account of the profile gets; ccp run passes it as --mcp-config=<file>. ccp never writes Claude Code's .claude.json; servers added on one account are only reported."
tags: [decision, accounts, activation, mcp]
decided: 2026-11-23
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-23T10:10:00Z }
---

# Problem

Claude Code keeps user-scope MCP servers (`claude mcp add`) in `.claude.json` inside
`CLAUDE_CONFIG_DIR`. Each [account](/decisions/accounts-as-activation-option.md) owns its
`.claude.json` (it carries `oauthAccount`), seeded once from the profile and never
overwritten. So a server added on one account is missing on the others, and an account
relinked to another profile by a [pool](/decisions/account-pools.md) keeps the old profile's
servers.

# Decision

- `profiles/<name>/mcp.json` (Claude Code's `--mcp-config` format, `{"mcpServers": {...}}`)
  is the profile's shared server list. It is a profile entry, not a new concept.
- `ccp run` adds `--mcp-config=<abs path>` right after the program when the command is
  `claude` (or a native `.../claude/versions/<ver>` path) and the file exists: with an
  account, a pooled pick, the `--or-profile` fallbacks, or no account. agy and other programs
  are untouched. The `=` form is required: the spaced form is variadic and took `mcp list`
  and a prompt as more config files (tested on 2.1.330).
- On a name in both places Claude Code connects once, to the `--mcp-config` entry
  (`system/init` shows `"source":"dynamic"`; tested on 2.1.330, not documented). The file
  wins; servers only in the account still load.
- ccp only reads `.claude.json` (and a non-empty `.config.json`): `ccp run` and
  `ccp doctor` report servers an account has that `mcp.json` lacks.
  `ccp doctor --fix` creates a missing `mcp.json` (0600, `O_EXCL`) from the servers in the
  profile's own state (`~/.claude.json` when globally active, else `<profile>/.claude.json`).

# Rejected

- **Merge `mcpServers` into each account's `.claude.json` at launch.** Nicer: `claude mcp
  add` would just work. But ccp would write a file Claude Code owns and rewrites (with its own
  lock, a format that moved to `.config.json` once already); a running session could overwrite
  the merge, and the failure is a silently missing server. A 3-way merge with a ledger per
  account is also the most code to keep.
- **A local plugin whose `.mcp.json` lists the servers**, enabled through the shared
  `settings.json` (`extraKnownMarketplaces` + `enabledPlugins`). It loads with no flag, also
  in `ccp env` shells, but plugin servers are renamed `plugin:<plugin>:<name>`, which breaks
  `mcp__<server>__*` permission rules, hooks and instructions.

# Known limits

- Only `ccp run` (and the `ccp use --accounts` shim, which goes through it) passes the file.
  A plain `claude` from a `ccp env` or `ccp session` shell, mise or direnv does not.
- Local-scope servers (`projects[<path>].mcpServers`) are not covered; project servers belong in
  the repo's `.mcp.json`, which every account already reads.
- `claude mcp add` still writes to the account in use; move the server into `mcp.json` (ccp
  reports it until then). ccp never removes it from the account.
- The file often holds tokens (`env`, `headers`): keep it out of git. A profile file is only
  in dotfiles when it is linked there by hand.
