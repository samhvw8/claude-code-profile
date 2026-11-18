---
type: Anti-Pattern
title: "Starting claude through a mise shim from an account wrapper"
description: "A project claude wrapper that execs ~/.local/share/mise/shims/claude loses the account: the mise shim reapplies the folder's mise.toml, which resets CLAUDE_CONFIG_DIR to the profile."
tags: [anti-pattern, activation, accounts, mise]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-18T15:40:00Z }
---

# Don't

In a folder whose `mise.toml` sets `CLAUDE_CONFIG_DIR` to a profile, start the real `claude` from a
wrapper (or from `ccp run --account …`) through the mise shim,
`~/.local/share/mise/shims/claude`.

# Why not

A mise shim runs the tool with the environment of the current folder's mise config. That
environment sets `CLAUDE_CONFIG_DIR` back to the profile folder, so the account folder that
`ccp run --account` set is dropped. On 2026-11-18 this sent three `/login`s meant for three account
folders into the growtrics profile's one login: every pane showed the same account, and the
account folders stayed logged out. `claude auth status` shows it: `configDirectory` names the
profile, not the account.

# Instead

Resolve the binary with `mise which claude` (it prints the install path and applies no env), or,
without mise, take the next `claude` on PATH that is neither the wrapper's folder nor a
`mise/shims` folder. The shim that `ccp use --accounts` writes does both
([accounts decision](/decisions/accounts-as-activation-option.md)); `TestAccountShim_Exec` guards it.
