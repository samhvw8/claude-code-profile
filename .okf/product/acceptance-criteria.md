---
type: Requirements
title: Acceptance criteria
description: Gherkin acceptance criteria AC-1 to AC-24 for ccp's commands.
tags: [product, requirements, acceptance]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: ccp-spec
    resource: "docs/ccp-spec.md (removed 2026-09-14 by the OKF migration)"
    title: ccp product specification
---

# Notes

Migrated from the spec. `profile.yaml` references are updated to `profile.toml`, the
manifest format since v0.12. Command details: [CLI reference](/cli/command-reference.md).

# Initialization and profiles

**AC-1: init**
```gherkin
GIVEN user has existing ~/.claude with skills, hooks, rules, commands
WHEN user runs `ccp init`
THEN tool creates ~/.ccp/hub/ with all hub-eligible items moved
AND tool creates ~/.ccp/profiles/default/ with symlinks to hub items
AND tool preserves original ~/.claude directory permissions for profile
AND tool creates ~/.ccp/profiles/shared/ directory
AND existing Claude Code behavior is unchanged (default profile mirrors original)
AND tool outputs next steps for shell configuration
```

**AC-2: profile create**
```gherkin
GIVEN hub is initialized
WHEN user runs `ccp profile create <name>` with item selections
THEN tool creates ~/.ccp/profiles/<name>/ directory
AND tool inherits directory permissions from current ~/.claude
AND tool creates profile.toml with selected items
AND tool creates symlinks for all selected hub items
AND tool creates CLAUDE.md (composed or template)
AND tool generates settings.json from settings template (if set) and hooks
AND data directories are symlinked to shared/
```

**AC-3: link** — GIVEN a profile, WHEN `ccp link <profile> skills/<name>`, THEN a symlink is
created in the profile's `skills/` AND `profile.toml` includes the item.

**AC-4: unlink** — GIVEN a profile with a linked item, WHEN `ccp unlink <profile> skills/<name>`,
THEN the symlink is removed AND `profile.toml` no longer lists the item.

**AC-5: profile check**
```gherkin
GIVEN profile exists with profile.toml
WHEN user runs `ccp profile check <name>`
THEN tool compares the manifest against directory state
AND tool reports: missing, extra, broken, mismatched items
AND tool exits 0 if valid, non-zero if drift detected
```

**AC-6: profile fix**
```gherkin
GIVEN profile has configuration drift
WHEN user runs `ccp profile fix <name>`
THEN tool reconciles directory to match profile.toml
AND tool reports all changes made
AND user can pass --dry-run to preview without changes

GIVEN multiple profiles exist
WHEN user runs `ccp profile fix --all`
THEN tool fixes all profiles, skipping hub_missing items without prompting
AND --force auto-removes hub_missing items from manifests
AND per-profile errors are warned but do not stop iteration
```

**AC-7: profile list** — outputs all profile names and marks the active one.

**AC-17: profile clone** — `ccp profile clone <src> <new>` creates a profile with the copied
manifest: the same hub links as the source.

**AC-18: profile diff** — `ccp profile diff <a> <b>` reports hub items only in A and only in B.

**AC-19: profile sync**
```gherkin
GIVEN profile exists with hub hooks or symlinks
WHEN user runs `ccp profile sync [name]`
THEN tool regenerates symlinks for all hub items in manifest
AND tool removes symlinks not in manifest
AND tool regenerates settings.json from settings template and hook configurations
AND each hook includes interpreter prefix and $HOME-based paths
AND supports --all flag to sync all profiles
```

**AC-20: profile edit**
```gherkin
GIVEN profile exists
WHEN user runs `ccp profile edit [name]`
THEN tool allows adding/removing hub items via flags or interactive picker
AND --add-<type>=name adds items to profile
AND --remove-<type>=name removes items from profile
AND -i/--interactive opens tabbed picker with current selections
AND picker supports scrolling (max 10 visible items), fuzzy search (/ key), and tab-to-toggle
AND tool syncs symlinks and regenerates settings.json after changes
```

# Activation

**AC-8: environment activation**
```gherkin
GIVEN profile exists at ~/.ccp/profiles/quickfix
WHEN CLAUDE_CONFIG_DIR is set to that path
THEN Claude Code loads configuration from that profile
AND symlinked hub items are resolved correctly
AND env override takes precedence over ~/.claude symlink
```

**AC-9: `ccp use`**
```gherkin
GIVEN profile exists at ~/.ccp/profiles/dev
WHEN user runs `ccp use dev` in a directory with mise.toml
THEN mise.toml is updated with CLAUDE_CONFIG_DIR and CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD
AND Claude Code uses dev profile in that project

GIVEN profile exists at ~/.ccp/profiles/quickfix
WHEN user runs `ccp use quickfix -g`
THEN ~/.claude becomes a symlink to ~/.ccp/profiles/quickfix
AND Claude Code uses quickfix profile globally when no CLAUDE_CONFIG_DIR is set

GIVEN no mise.toml or .envrc exists
WHEN user runs `ccp use dev` and mise command is available
THEN tool prompts to create mise.toml with Claude env vars
```

**AC-10: show current** — with `~/.claude` a symlink, `ccp use --show` prints the linked profile name.

**AC-14: auto** — with `.ccp.yaml` (`profile: <name>`) in the current or a parent directory,
`ccp auto` prints the name, and `--path` prints the full profile path.

**AC-15: session** — `ccp session <profile>` starts a shell with `CLAUDE_CONFIG_DIR` set to the profile.

**AC-16: run** — `ccp run <profile> -- <command> [args]` runs the command with `CLAUDE_CONFIG_DIR` set.

# Health and lifecycle

**AC-11: reset**
```gherkin
GIVEN ccp is initialized with ~/.claude as symlink
WHEN user runs `ccp reset` and confirms
THEN tool copies active profile contents to ~/.claude (replacing symlink)
AND tool preserves directory permissions from the profile
AND tool removes ~/.ccp directory entirely
AND Claude Code continues working with restored ~/.claude directory
```

**AC-12: doctor**
```gherkin
GIVEN ccp may have configuration issues
WHEN user runs `ccp doctor`
THEN tool checks: initialization, ~/.claude symlink, hub structure, profile manifests, broken symlinks
AND tool reports status for each check (OK/FAIL/WARN)
AND tool provides remediation instructions for failures

GIVEN ccp has fixable issues (missing hub dirs, broken symlinks)
WHEN user runs `ccp doctor --fix`
THEN tool automatically creates missing hub directories
AND tool runs drift detection and fix for all profiles
AND tool reports count of issues fixed
```

**AC-13: status** — `ccp status` shows the active profile, hub item counts, profile health and
overall health, and flags profiles with drift or broken links.

**AC-24: usage** — `ccp usage` shows orphaned items (used by no profile), missing items
(referenced but not in the hub) and shared items (used by several profiles).

# Hub

**AC-21: hub add**
```gherkin
GIVEN hub is initialized
WHEN user runs `ccp hub add <type> <path>`
THEN tool copies file or directory to hub/<type>/
AND item is available for linking to profiles

GIVEN hub is initialized and profile exists
WHEN user runs `ccp hub add <type> <name> --from-profile=<profile>`
THEN tool copies item from profile to hub/<type>/
AND --replace flag allows overwriting existing hub items
AND tool suggests linking the item back to the profile
```

**AC-22: hub remove** (see [the copy-to-profile decision](/decisions/hub-remove-copy-to-profile.md))
```gherkin
GIVEN hub item exists
WHEN user runs `ccp hub remove <type>/<name>`
AND item is used by profiles
THEN tool shows three-choice prompt: copy to profiles / delete anyway / cancel
AND "copy" copies hub item into each affected profile as local item, removes hub link from manifest
AND "delete" removes from hub leaving broken links (backward-compatible with old "y")
AND "cancel" aborts removal
AND --copy flag copies to all affected profiles without prompting
AND --force skips all checks and deletes immediately
```

**AC-23: hub show** — `ccp hub show <type>/<name>` shows the item path, whether it is a file or
directory, its contents or file list, and which profiles use it.
