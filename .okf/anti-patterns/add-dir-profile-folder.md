---
type: Anti-Pattern
title: "Adding the profile folder with --add-dir"
description: "Passing the active profile folder to claude --add-dir to load its CLAUDE.md and rules — CLAUDE_CONFIG_DIR already loads them, and the flag triggers a trust prompt."
tags: [anti-pattern, activation]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-18T11:10:00Z }
sources:
  - id: v0-53-0
    resource: "commit 2b49138 (v0.53.0)"
    title: Drop the --add-dir claude alias and its env var
---

# Don't

Wrap `claude` in an alias or function that passes `--add-dir "$CLAUDE_CONFIG_DIR"` (or the
profile path from `ccp which --path`), or write `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD`
into project env files, to load a profile's `CLAUDE.md` and rules.

# Why not

- Claude Code loads `CLAUDE.md` and `rules/` from the active config folder as user memory, so
  the flag adds nothing ([activation](/architecture/activation.md)).
- Since Claude Code 2.1.326 every `--add-dir` folder needs workspace trust, and a "Yes, add it"
  answer lasts one session. Profiles and accounts keep their own `.claude.json`, so the prompt
  came back on every start.

# Instead

Set `CLAUDE_CONFIG_DIR` (or use the `~/.claude` symlink) and start `claude` with no extra
flag. `ccp config shell` prints only `ccp-use`, and `ccp use` removes the old env var line from
the file it updates; `cmd/use_test.go` guards all three.
