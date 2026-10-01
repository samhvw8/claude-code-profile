---
type: Feature
title: "opencode integration"
description: "Mirroring a profile into opencode: what opencode already reads, what ccp converts and writes, ownership, and when it follows automatically."
resource: internal/profile/opencode.go
tags: [feature, integration, opencode]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-01T09:00:00Z }
sources:
  - id: opencode-rules-docs
    resource: https://github.com/anomalyco/opencode/blob/dev/packages/web/src/content/docs/rules.mdx
    title: opencode rules and Claude Code compatibility
  - id: opencode-agent-docs
    resource: https://github.com/anomalyco/opencode/blob/dev/packages/core/src/plugin/skill/customize-opencode.md
    title: opencode agent file format
---

# What reaches opencode, and how

| Profile part | How opencode gets it |
|--------------|----------------------|
| `CLAUDE.md`, skills | opencode reads `~/.claude/CLAUDE.md` and `~/.claude/skills/` itself, so they follow the active profile with no ccp work[^opencode-rules-docs] |
| Rules | The user points opencode's `instructions` at `"{env:HOME}/.claude/rules/*.md"` once; the glob follows the profile |
| Agents, commands | `ccp opencode sync` writes converted copies into `~/.config/opencode/agent/` and `command/` |
| Hooks | Not mirrored: opencode runs hooks as TypeScript plugins, which ccp cannot generate; sync names the profile's hooks so the gap is visible |

The directory is `$XDG_CONFIG_HOME/opencode` when set, else `~/.config/opencode` — the same rule
opencode uses for its config path.

# Conversion

Copies, not links, because the frontmatter differs.[^opencode-agent-docs] opencode passes unknown
frontmatter fields to the model provider as options, so ccp writes only fields opencode understands:

| Claude Code field | opencode output |
|-------------------|-----------------|
| `description` | kept; an agent without one is skipped and reported |
| — | `mode: subagent` added |
| `model` | kept only when already a `provider/model` ID; `inherit`, `opus`, `sonnet` are dropped so the agent uses opencode's configured model |
| `tools: Read, Write, …` | an allowlist becomes a `tools:` map — listed tools `true`, the other built-ins `false` |
| `color` | kept only as `#rrggbb` |
| `name`, others | dropped (the filename is the name) |

Commands keep `description` and a `provider/model` `model`; `allowed-tools` and `argument-hint`
have no opencode equivalent. Bodies are unchanged — both harnesses use `$ARGUMENTS`.

# Ownership

ccp records each file it writes, with its hash, in `~/.ccp/state/opencode.json`, and only ever
replaces or removes those:

* a file ccp did not write where a copy would go is left alone and reported as occupied;
* a ccp file edited by hand since is left alone and reported, and stops being ccp's once the item
  leaves the profile;
* a file identical to what ccp would write is adopted.

# Following the profile

The first `ccp opencode sync` opts in (the state file exists). After that ccp re-syncs, quietly
unless something changed, on `ccp use -g`, on link/unlink, interactive link, `profile edit` and
`profile sync` of the active profile, and on `ccp bootstrap`. `ccp opencode` is hidden
([command surface](/decisions/command-surface.md)).

[^opencode-rules-docs]: opencode rules docs — Claude Code compatibility section.
[^opencode-agent-docs]: opencode agent docs — allowed frontmatter fields; unknown fields route into `options`.
