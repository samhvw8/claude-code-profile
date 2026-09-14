---
type: Changelog
title: Release history
description: What changed in each ccp release that the spec recorded, newest first.
tags: [product, releases, history]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md revision history (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
  - id: git-log
    resource: "git log of this repository"
    title: Commit messages for v0.44.0 and v0.45.0
---

# Releases

The spec's history stopped at 0.43.0; the two rows above it come from the commit log.[^git-log]
The authoritative version is the latest git tag (also stated in `CLAUDE.md`).

| Version | Date | Changes |
|---------|------|---------|
| 0.45.0 | 2026-07-11 | Fixed: drift detection resolves bundle member symlinks during `profile fix`. |
| 0.44.0 | 2026-07-03 | Added: `ccp bootstrap` for multi-machine chezmoi sync ([bootstrap](/features/bootstrap.md)). |
| 0.43.0 | 2026-07-02 | Fixed: source installer discovers items from the `.claude/` layout, checked before `.agents/`/`.codex/`. Added `.claude-plugin/plugin.json` marketplace manifest. Updated skills with installation and picker docs. Clearer `source update` output. |
| 0.42.0 | 2026-07-02 | TUI picker uses fzf-style fuzzy search (consecutive chars, word boundaries, start-of-string) sorted best-first; Tab toggles in normal and search modes; arrows navigate while searching; context-aware help. Fixed `ccp doctor` resolving relative symlink targets against the symlink's parent dir, not CWD. |
| 0.39.0 | 2026-06-20 | Install from bare skill repos (`SKILL.md` at repo root, name from frontmatter, `CopyDir` skips `.git`). Install-by-URL for a `SKILL.md` blob URL via `InstallPath`; `ParseGitWebURL` handles `/blob/`, `/tree/`, raw, and GitLab `/-/blob/`. |
| 0.32.0 | 2026-04-15 | `hub remove` offers copy-to-profile: three-choice prompt replaces "Remove anyway?"; `--copy` for scripting. |
| 0.31.0 | 2026-04-03 | `profile fix --all`; without `--force` hub_missing items are skipped, with it auto-removed; per-profile errors warn and continue. |
| 0.30.0 | 2026-04-01 | `hub remove`/`hub show` resolve single-file items (exact path, then .md/.yaml/.sh/.json) and gain `-i`; both default to interactive without args. |
| 0.29.1 | 2026-04-01 | `FragmentMigrator` in `ccp migrate` merges legacy `hub/setting-fragments/*.yaml` into a `migrated-fragments` template. |
| 0.29.0 | 2026-04-01 | `ccp project` group: `add` copies hub items into a project's `.claude/`, `list`, `remove`; project root via `.git/` walk-up. README rewritten for v0.28+. |
| 0.28.0 | 2026-03-31 | **Simplification release** — removed engines, contexts, setting-fragments, linked-dirs, per-data-type sharing; single `GenerateSettings()`; 13 power-user commands hidden; `ccp skills` commands removed; ~6K LOC removed; concepts 14 → 5. See [the decision](/decisions/v0-28-simplification.md). |
| 0.27.0 | 2026-03-18 | Settings templates replace setting-fragments (`ccp template …`, `--template`). `GlobalClaudeDir` added; `ccp use -g` always targets `~/.claude`. |
| 0.26.0 | 2026-03-08 | Two-layer composition (engine + context) and CLAUDE.md linked dirs — removed again in 0.28. |
| 0.25.2 | 2026-02-05 | `profile fix` prompts to drop non-existent hub items; `--force`; drift type `hub_missing`. |
| 0.25.0 | 2026-02-03 | `source update` only touches the registry timestamp when git has new commits. |
| 0.24.0 | 2026-02-02 | `--json` for profile list, hub list, source list, status; first-run guidance. |
| 0.17.0 | 2026-01-31 | `profile rename`, `profile create --empty`, interactive `link`. |
| 0.16.0 | 2026-01-31 | Shared plugin store at `~/.ccp/store/plugins/`, symlinked into profiles. |
| 0.14.0 | 2026-01-31 | Relative symlinks for cross-machine portability; `ccp migrate` converts absolute ones. |
| 0.13.0 | 2026-01-31 | `owner/repo@ref` sources; plugin discovery in `plugins/` and `external_plugins/`; `source install -i`. |
| 0.12.0 | 2026-01-31 | `ccp migrate` (profile.yaml → profile.toml, source.yaml → registry.toml); `ccp init` writes ccp.toml. |
| 0.9.0 | 2026-01-30 | GitHub source tracking; `hub update`, `skills update`, `plugin update`; hooks installed as folder units. |
| 0.8.5 | 2026-01-29 | `ccp hub prune`. |
| 0.8.4 | 2026-01-29 | `ccp plugin list` and `plugin add` from marketplace repos. |
| 0.8.3 | 2026-01-29 | `ccp skills find` / `skills add` (removed in 0.28). |
| 0.8.2 | 2026-01-29 | Dynamic shell completions; drift warning on `use`. |
| 0.8.1 | 2026-01-29 | `doctor --fix`; migration test coverage 71.6%; actionable error messages. |
| 0.8.0 | 2026-01-29 | Removed md-fragments hub type. |
| 0.7.0 | 2026-01-29 | setting-fragments hub type (later removed); picker scrolling and search. |
| 0.6.0 | 2026-01-29 | `profile edit`, richer `profile sync`, `hub add --from-profile`, `--replace`; hook paths `$HOME`-based. |
| 0.5.0 | 2026-01-29 | Permission preservation for init, profile create, reset. |
| 0.4.0 | 2026-01-29 | reset, status, doctor, which, auto, session, run, usage; hub CRUD; profile clone/diff/sync; `.ccp.yaml`. |
| 0.1.0 | 2025-01-28 | Initial specification. |

[^git-log]: Commit messages for v0.44.0 and v0.45.0
