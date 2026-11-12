---
type: Decision
title: "agy: converted copies for agents and rules, links for skills, the style in GEMINI.md"
description: "Why ccp agy sync converts agents and rules, links skills and CLAUDE.md, writes the output style to GEMINI.md, and mirrors no hooks or commands."
tags: [decision, agy, integration]
decided: 2026-11-12
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-12T12:00:00Z }
---

# Decision

`ccp agy sync` mirrors a profile into Antigravity CLI's `~/.gemini/config/` using the form each part
needs for agy to load it, and nothing agy cannot run ([agy integration](/features/agy.md)).

# Why each form

- **Agents and rules are converted copies.** agy loads a rule only with its own `trigger`
  frontmatter and an agent only with its own tool names and fields, so a link to the Claude file is
  a file agy ignores or misreads — the lesson of [link only what omp loads](/decisions/omp-link-only-what-omp-loads.md).
  Copies reuse opencode's hash-recorded ownership, so hand edits are never overwritten.
- **Skills and `CLAUDE.md` are links.** agy reads a Claude `SKILL.md` and an always-on
  `AGENTS.md` as they are; a link keeps them current with no sync and costs nothing to maintain.
- **The output style goes to `GEMINI.md`.** It is agy's other always-on, frontmatter-free file and
  `AGENTS.md` is already the profile's `CLAUDE.md`. Putting the style in a rule would compete with
  the profile's rules and be capped at 24,000 bytes. It is a carrier in the shared ledger
  ([output styles](/features/output-styles.md)), so `ccp reset` and follow already handle it.
- **Hooks and commands are not mirrored.** agy hooks use their own `hooks.json` schema and tool
  names — translating them is authoring, not a transform (refused for omp for the same reason) —
  and agy has no commands. Both are named in the report so the gap is visible.
- **Symlinked type directories are refused.** Writing through a dotfiles-managed `agents/` link
  would put ccp's files into a tree someone else owns, as with ccp's own `output-styles` rule.

# Implication

A hidden command under the existing harness pattern; no new user-facing concept.
