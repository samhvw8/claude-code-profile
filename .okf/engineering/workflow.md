---
type: Playbook
title: Development and release workflow
description: How to change ccp safely — build and test loop, common tasks, release and tagging, git rules, version tracking.
tags: [engineering, workflow, release, git]
generated: { by: claude-code/claude-opus-5-5, at: 2026-11-18T12:05:00Z }
sources:
  - id: workflow-rule
    resource: ".claude/rules/03-workflow.md (pre-migration version, 2026-09-14)"
    title: Workflow rule
  - id: claude-md
    resource: "CLAUDE.md, Before Making Changes and Common Tasks (pre-migration version)"
    title: Project CLAUDE.md
---

# Before and after every change

1. Read existing code and match the patterns in similar files.
2. `go build -o ccp ./cmd/ccp` compiles cleanly.
3. `go test ./...` passes, before and after.
4. Keep platform variants in mind: symlink code has unix and windows versions.

```bash
go build -o ccp ./cmd/ccp  # build
go test ./...              # all tests (-v for verbose)
go mod tidy                # dependencies
./ccp --help               # smoke test
```

# Common tasks

**Add a command**
1. Create `cmd/<name>.go` (or `cmd/<parent>_<child>.go`) with a Cobra command.
2. Register it in `init()` with `rootCmd.AddCommand()` or `parentCmd.AddCommand()`.
3. Add flags with `cmd.Flags()` in `init()`.
4. Implement `RunE`, returning errors.

**Add domain logic**
1. Create or extend a package in `internal/<domain>/`.
2. Define an interface only where a test needs one.
3. Add `*_test.go` tests.
4. Wire it up in the command layer.

Conventions: [code standards](/engineering/code-standards.md).

# Release

When asked to "update docs and commit" or similar:

1. Update the affected [`.okf/`](/engineering/knowledge-maintenance.md) concepts, their `index.md`, and `log.md` — plus the version in `CLAUDE.md` and `.claude-plugin/plugin.json` — in one pass.
2. Add the release to [release history](/product/release-history.md).
3. Stage and commit with a descriptive message.
4. Create an **annotated** version tag if the version was bumped: `git tag -a vX.Y.Z -m vX.Y.Z`.
5. **Do not push** — the user pushes after reviewing, with `git push --follow-tags` (or
   `git config push.followTags true` once). The Release workflow runs only when the tag reaches
   GitHub, and `--follow-tags` pushes annotated tags only: v0.46.0 got a lightweight tag that never
   left the machine, so no release was built and mise
   users stayed on v0.45.0 until v0.47.0.
6. After a push with several new tags, push them oldest first (`git push origin vX.Y.Z`) and let
   each Release run finish before the next, so the newest release is created last and keeps the
   Latest label.
7. Upgrade the local ccp once the Release run is green. A plain `mise up` can report "up to date"
   for a while, because mise reads versions from a cache and a shared versions server; on
   2026-11-18 it missed v0.52.0 and v0.53.1 that way. This reads GitHub directly:

   ```bash
   mise cache clear && MISE_USE_VERSIONS_HOST=0 mise up github:samhvw8/claude-code-profile
   ```

# Git rules

| Rule | Why |
|------|-----|
| Never `git tag -f` or `git push --tags -f` | Force-updating a tag stops GitHub CI from re-running ([anti-pattern](/anti-patterns/force-updating-git-tags.md)) |
| Always increment the version and create a new tag | Same reason |
| Never push without an explicit request | The user reviews first |

# Version tracking

| Where | How |
|-------|-----|
| Git tag | Source of truth |
| `CLAUDE.md` line 3 | `**Current version: vX.Y.Z**` — update on release |
| `.claude-plugin/plugin.json` | `"version": "X.Y.Z"` — update on release (it lagged at 0.42.0 until v0.46.0) |
| `cmd/root.go` | Set at build time via ldflags; no manual change |
