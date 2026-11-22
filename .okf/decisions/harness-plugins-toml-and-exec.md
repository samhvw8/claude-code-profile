---
type: Decision
title: "Custom harness plugins use declarative TOML manifests or JSON stdin/stdout executables"
description: "2026-11-22: Extend ccp to new harnesses without an embedded JS VM via ~/.ccp/harnesses/<id>.toml or executable plugins (~/.ccp/harnesses/<id> or ccp-harness-<id> on PATH) returning a validated HarnessPlan."
tags: [decision, harness, plugins, architecture]
decided: 2026-11-22
generated: { by: antigravity/gemini-4-neon, at: 2026-11-22T00:46:00Z }
---

# Decision

Custom harnesses extend `ccp` through a hidden `ccp harness` command (`list`, `sync`, `check`,
`remove`) backed by two non-JS plugin formats that both produce one shared `HarnessPlan`:

1. **Declarative TOML manifests** (`~/.ccp/harnesses/<id>.toml`) for standard path/copy/link/merge
   harnesses (`~` and `${env:NAME:-default}` expansion, `[context]`, `[skills]` (`link` only),
   `[commands]` (`link`|`copy`), `[agents]` (`link`|`copy`), `[rules]` (`link`|`copy`|`merge`), and
   `[output_style]`). Unknown TOML fields, unset `${env:NAME}` without a default, and relative
   roots are rejected.
2. **Executable stdin/stdout JSON plugins** (`~/.ccp/harnesses/<id>` or `ccp-harness-<id>` on
   absolute `PATH` directories) for harnesses needing custom frontmatter or format transforms.
   `ccp` pipes a read-only `HarnessInput` (`api_version: 1`) on `stdin` and reads a strict
   `HarnessPlan` (`api_version: 1`, unknown fields rejected) on `stdout` under a 10-second timeout
   (`WaitDelay` 2s) and 16 MiB stdout cap.

Built-in harness IDs (`claude`, `omp`, `opencode`, `pi`, `agy`) are reserved.

# Safety and ownership

Plugins never touch disk. `ccp` validates every returned path and executes the plan itself:

- `root` must be absolute and cannot be `/`, `$HOME`, inside `~/.ccp`, or inside `~/.claude`.
- Every relative path in `links`, `copies`, and `style_file` must stay inside `root` (no `..`, no
  absolute paths, no `\`) with no duplicate normalized keys or ancestor/descendant overlaps.
- Every link target must resolve strictly inside `~/.ccp` after `EvalSymlinks`.
- Every intermediate parent directory under `root` is checked with `os.Lstat`; symlinked or
  non-directory parents are refused for links, copies, `style_file`, and `harness remove`.
- Copies reuse `syncCopies` with a per-harness hash ledger at `~/.ccp/state/harnesses/<id>.json`
  (also the opt-in marker for `followHarnesses`). Changing `root` retires old copies and links from
  the previous root; removing or changing `style_file` retires the previous carrier under group
  `harness:<id>:`.
- `ccp reset` re-checks and removes ccp-owned links recorded for opted-in custom harnesses;
  `ccp doctor` reports invalid TOML/ambiguous plugins (without executing exec plugins).

# Alternatives rejected

- **Embedded QuickJS or pure-Go JS VM (`sobek`/`goja`):** QuickJS breaks `CGO_ENABLED=0` static
  builds; even a pure-Go JS VM adds permanent Host API and two-language maintenance cost that is
  unjustified while `ccp` can delegate complex transforms to external executables over JSON.
- **TOML-only DSL:** Pushing frontmatter transforms or custom logic into TOML turns config into an
  ad-hoc programming language.
