---
type: Mechanism
title: omp teardown and path resolution
description: How ccp finds every omp agent directory it may have written into, removes exactly its own paths, and resolves omp named profiles.
resource: internal/profile/omp.go
tags: [omp, reset, paths]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md, section omp (oh-my-pi) Portability (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
    last_modified: 2026-09-14
---

# Teardown

`OmpManaged()` enumerates everything ccp owns in omp — type-directory links (including rule links
an older ccp left behind), the `AGENTS.md` link, and a marker-carrying `RULES.md` — across the
default agent directory, *every* named omp profile, and a `PI_CODING_AGENT_DIR` override.
Directories are de-duplicated by resolved path, so none is removed twice. `OmpTeardown()` removes
exactly that set.

| Caller | Why |
|--------|-----|
| `ccp omp unlink --all` | Remove everything ccp set up |
| `ccp reset` | Otherwise deleting `~/.ccp` would leave every omp link dangling. Teardown runs *after* the resetter succeeds, so an aborted reset keeps the integration |

Foreign links and real files are never touched; empty directories are left in place. The opt-in
test is deliberately narrower than teardown ([profile switching](/features/omp/profile-switching.md)).

# Path resolution

`OmpAgentDir()` mirrors omp's own resolution:

| Input | Result |
|-------|--------|
| `OMP_PROFILE` (legacy fallback `PI_PROFILE`) set to a name | `~/.omp/profiles/<name>/agent`; `PI_CODING_AGENT_DIR` is ignored |
| `default`, empty, or whitespace | Default profile |
| `PI_CODING_AGENT_DIR` set, no named profile | That directory |
| Nothing set | `~/.omp/agent` |
| `PI_CONFIG_DIR` | Renames the config root (default `.omp`) |

`OMP_PROFILE=work ccp omp sync` and `ccp omp --profile work sync` both populate the `work` omp
profile; a plain run targets the default one. `omp --profile <name>` without exporting
`OMP_PROFILE` (for example through an `omp --alias` shell alias) is invisible to ccp, which is
why the flag exists. An invalid `--profile` name (not a single path element) is an error, not a
silent fallback.
