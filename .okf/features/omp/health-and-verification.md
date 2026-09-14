---
type: Mechanism
title: omp health checks and live verification
description: What ccp omp list and ccp doctor report about omp, and how doctor --verify-omp asks a live omp what it loaded.
resource: internal/profile/omp_probe.go
tags: [omp, doctor, diagnostics]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md, section omp (oh-my-pi) Portability (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
    last_modified: 2026-09-14
---

# ccp omp list

`ccp omp list [profile]` lists what is linked and how far omp is from a profile (the active one
by default):

| Line | Meaning |
|------|---------|
| `missing` | What `sync` would add |
| `stale` | What `sync` would remove *or re-point* — a link whose target no longer matches its name, or a rule link an older ccp left behind |
| `context:` | State of `AGENTS.md` and `RULES.md`, with the size injected into every session |
| `ignored:` | Items omp is predicted to ignore — linked anyway, with the reason |

`--json` emits `agent_dir`, `linked`, `profile`, `missing`, `stale`, `ignored`, `unreadable`.

# ccp doctor

`ccp doctor` inspects omp once ccp has linked anything. Broken links (the hub item is gone) are
listed and `ccp doctor --fix` removes them; link and context drift against the active profile
point at `ccp omp sync`. With no ccp links the check prints `OK (nothing linked)`, so users who
never opted in see nothing.

# ccp doctor --verify-omp

Everything above predicts omp's loader, and a wrong prediction is invisible. `--verify-omp` asks
omp itself: it runs `omp --mode rpc --no-session` with a `get_state` request, reads the system
prompt omp built, and checks that the text of every rule and of `AGENTS.md` reached it.

| Property | Value |
|----------|-------|
| Needs | omp with a configured model |
| Costs | No model tokens, about 2 s; starts no session |
| Side effects | Touches omp's own state, which is why it is opt-in |
| Item names | Checked as information only — omp picks skill metadata through its own filters (`skillful`, `ignoredSkills`, `includeSkills`, custom dirs) and names agents through tool schemas, not the prompt |
