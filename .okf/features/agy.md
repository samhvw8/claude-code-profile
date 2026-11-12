---
type: Feature
title: "agy (Antigravity CLI) integration"
description: "Mirroring a profile into Antigravity CLI's ~/.gemini/config: context link, GEMINI.md style, converted rules and agents, skill links, ownership, and following."
resource: internal/profile/agy.go
tags: [feature, integration, agy]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-12T12:00:00Z }
sources:
  - id: agy-loader-tests
    resource: "Hands-on tests of agy v1.5.6's global config loading (orchestrator session, 2026-11-12)"
    title: agy v1.5.6 config behavior, tested by hand
---

# What reaches agy, and how

agy reads none of `~/.claude`, so `ccp agy sync` sets everything up in `~/.gemini/config/`
([why each part takes the form it does](/decisions/agy-copies-links-and-gemini-md.md)):[^agy-loader-tests]

| Profile part | agy file | Form |
|--------------|----------|------|
| `CLAUDE.md` | `AGENTS.md` | Link to `<profile>/CLAUDE.md`; agy loads it always-on with no frontmatter |
| Output style | `GEMINI.md` | The selected style's body, owned through the [carrier ledger](/features/output-styles.md); removed when no style is selected |
| Rules | `rules/<name>.md` | Converted copy (below) |
| Agents | `agents/<name>.md` | Converted copy (below) |
| Skills | `skills/<name>` | Link to the hub item behind the profile's skill (bundle members resolve inside their bundle, as for omp) |
| Hooks, commands | — | Not mirrored and named in the report: agy hooks use their own `hooks.json` schema and tool names, and agy has no commands |

`AGENTS.md` follows omp's rules: only a link resolving into `~/.ccp` is replaced; a real file or a
foreign link is reported as occupied; ccp's link is removed when the profile has no `CLAUDE.md`.

# Rule conversion

agy loads a file in `rules/` only with `trigger` frontmatter, so a link to a Claude rule would be
ignored. The body is kept; the frontmatter becomes:

| Claude Code rule | agy rule |
|------------------|----------|
| `paths:` (string or list) | `trigger: glob` and `globs:` — one comma-separated string; brace sets expanded (`**/*.{ts,tsx}` → `**/*.ts, **/*.tsx`); relative globs prefixed with `**/` because agy matches absolute paths |
| anything else | `trigger: always_on`, as Claude Code loads it |
| `description` | kept |
| `alwaysApply`, other keys | dropped |

agy truncates a rule file at 24,000 bytes; a larger rule is still written and reported.

# Agent conversion

| Claude Code field | agy output |
|-------------------|------------|
| `name` | kept, else the file stem; a built-in agy name (`self`, `browser`, `research`, `deepagent`, `review-agent`) gets a `claude-` prefix. The file is `agents/<name>.md` |
| `description` | kept; an agent without one is skipped and reported |
| — | `subagent: true`, `mainAgent: false` |
| `tools` (comma string or list) | mapped and deduplicated: Read/Glob/Grep/LS/NotebookRead → `read_file`; Edit/MultiEdit/NotebookEdit → `edit_file`; Write → `write_file`; Bash/BashOutput/KillShell → `run_command` + `manage_task`; WebSearch/WebFetch → `search_web`; the rest (Task, TodoWrite, Skill, `mcp__*`) dropped — agy adds MCP and messaging tools to every agent |
| no `tools` | all six agy tools, minus those a bare-name `disallowedTools` entry removes (Read, Edit, Write, Bash; `search_web` only when both WebSearch and WebFetch are disallowed). `Bash(rm:*)` narrows a tool and removes nothing |
| `model` | `haiku` → `flash`, `opus` → `pro`; anything else omitted |
| `color`, `effort`, `maxTurns`, others | dropped |

`tools` is always written, even as `[]`: an agy agent without the field gets a small read-only
default instead of what the Claude agent had. Frontmatter that is not valid YAML (prose
descriptions) is read line by line, as for omp.

# Ownership and safety

Copies are recorded with their hash in `~/.ccp/state/agy.json` and handled exactly like
[opencode's](/features/opencode.md): only ccp's unedited copies are replaced or removed; others are
reported as edited or occupied. Links are ccp's when they resolve into `~/.ccp`; ccp prunes its
skill links the profile no longer has and leaves hand-made entries alone. A skill whose `SKILL.md`
has no `description` is skipped, since agy requires one.

If `rules/`, `agents/` or `skills/` is itself a symlink or not a directory, ccp does not write
through it: that part is refused with "remove the `<x>` link first", its recorded copies are kept,
and the rest of the sync runs. `ccp reset` removes ccp's skill and `AGENTS.md` links (they would
dangle) and `GEMINI.md` through the carrier ledger.

# Following the profile

The first `ccp agy sync` opts in (the state file exists). After that `followHarnesses` re-syncs agy
alongside opencode and pi on `ccp use -g`, link/unlink, `profile edit`/`profile sync` of the active
profile, hub-changing commands, and `ccp bootstrap`. `ccp agy` is hidden
([command surface](/decisions/command-surface.md)).

[^agy-loader-tests]: agy v1.5.6 loads `AGENTS.md`/`GEMINI.md` always-on without frontmatter, `rules/*.md` only with `trigger` frontmatter, `skills/<name>/SKILL.md` with a non-empty description, and `agents/<name>.md` with `name`, `description`, `tools`, `subagent`, `mainAgent`, `model`.
