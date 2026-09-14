---
type: Mechanism
title: What ccp links into omp
description: The item types ccp links, why rules and hooks are not linked, the reporter-only loadability check, link identity, and what is never overwritten.
resource: internal/profile/omp.go
tags: [omp, linking]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md, section omp (oh-my-pi) Portability (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
    last_modified: 2026-09-14
---

# Linked types

`skills/`, `agents/` and `commands/` are linked one directory down, in the shape omp
discovers. `sync` is manifest-driven (bundle members included) and owns those three
directories: links the profile does not use are removed, as are rule links older ccp versions
created, while foreign links and real files are left alone.

**`rules/` is not a link type.** omp surfaces a rule only when its frontmatter routes it
somewhere — a description into the rulebook, `alwaysApply` into every session, a condition into
TTSR — so a link whose frontmatter omp does not honor is a rule nobody loads, silently. Every
profile rule travels in `RULES.md` instead ([global context](/features/omp/global-context.md)).
Why: [rules travel in RULES.md](/decisions/omp-rules-travel-in-rules-md.md).

**`hooks/` is not a link type and is never scanned.** omp imports hook *modules* from
`hooks/pre` and `hooks/post` — where real omp hooks live — and cannot run a Claude
`hooks.json`. `settings-templates/` is excluded too: `config.yml` and `settings.json` are
different schemas.

# The loadability check is a reporter

`ompLoadabilityOf()` encodes omp's loader rules (checked against omp 18.1.19), but nothing
depends on its verdict except a printed note. An item omp is predicted to ignore is linked anyway
and reported with the reason — a wrong guess about another program's loader must never take
configuration away ([anti-pattern](/anti-patterns/predicting-another-programs-loader.md)).

| Hub item | omp location | Reported as ignored when |
|----------|--------------|--------------------------|
| `skills/<name>` | `~/.omp/agent/skills/<name>` | no `SKILL.md`, no non-empty `description`, or `enabled: false` |
| `commands/<name>.md` | `~/.omp/agent/commands/<name>.md` | not a `.md` file |
| `agents/<name>.md` | `~/.omp/agent/agents/<name>.md` | no `name`, no `description`, or `model: inherit` (omp resolves no such model, so the agent fails to spawn) |

Values are tested, not key presence: an empty `description` is no description. Frontmatter
parsing mirrors omp's permissiveness: line-based fallback for prose that breaks YAML, BOM and
CRLF fences tolerated.

# Link identity

Identity is resolved, never textual: a recorded target is joined against the link's *resolved*
parent (`resolveCcpLink`), matching how `ompCreateLink` writes it. A dotfiles-managed
`~/.omp` whose symlink points at a deeper directory cannot make ccp disown its own links. A
broken link is still recognized as ccp's — its target names the hub item — and
`ccp doctor --fix` prunes it once that item is gone.

# Never overwritten

Two things may hold content ccp did not create, so they are never overwritten: a path that exists
and is not a ccp link, and a profile item that cannot be read. `sync` reports both and carries
on; `link` fails on the item it was asked for. An unreadable rule is skipped and reported, never
fatal. Bundle members resolve inside their bundle directory; a bundle copy of an item the profile
also links directly is the same item.
