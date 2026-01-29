# ccp (Claude Code Profile) — Product Specification

**Version:** 1.3
**Date:** 2026-01-29
**Status:** Draft

---

## Problem Statement

Claude Code's 20 skill limit forces users to manually reconfigure for different work modes (development, quick-fix, documentation, etc.). No mechanism exists to save, switch, or share configurations. Duplicating skills across setups creates maintenance burden and configuration drift.

**Pain frequency:** Daily — every project/task switch
**Pain severity:** High — manual reconfiguration is tedious, error-prone, and causes context window bloat that degrades Claude Code performance

**Market signal:** Multiple requests in Claude Code GitHub issues with no official solution planned.

---

## Solution

A local CLI tool (`ccp`) that manages a central hub of reusable components and multiple profiles. Each profile is a complete Claude Code configuration directory, activated via `CLAUDE_CONFIG_DIR`. Hub items are symlinked into profiles to ensure single-source-of-truth maintenance.

---

## Architecture

```
~/.ccp/                               # CCP data directory
├── hub/                              # Single source of truth (Lego box)
│   ├── skills/
│   ├── agents/
│   ├── rules/
│   ├── hooks/
│   ├── md-fragments/
│   └── commands/
│
├── profiles/
│   ├── default/                      # Migrated from original ~/.claude
│   │   ├── CLAUDE.md
│   │   ├── settings.json
│   │   ├── skills/                   # Symlinks → hub/skills/*
│   │   ├── agents/                   # Symlinks → hub/agents/*
│   │   ├── hooks/                    # Symlinks → hub/hooks/*
│   │   ├── rules/                    # Symlinks → hub/rules/*
│   │   ├── tasks/                    # Local OR symlink → shared/tasks
│   │   ├── todos/
│   │   ├── history.jsonl
│   │   ├── file-history/
│   │   └── profile.yaml              # Manifest
│   │
│   ├── quickfix/                     # Purpose-specific profile
│   │   └── ... (complete structure)
│   │
│   ├── dev-fullstack/
│   │   └── ... (complete structure)
│   │
│   └── shared/                       # Shared data namespace
│       ├── tasks/
│       ├── todos/
│       └── paste-cache/

~/.claude → ~/.ccp/profiles/default   # Symlink to active profile
```

**Activation (two modes):**

1. **Default (symlink):** `~/.claude` is a symlink to active profile
   ```bash
   ccp use quickfix
   # → ~/.claude → ~/.ccp/profiles/quickfix
   ```

2. **Override (env):** `CLAUDE_CONFIG_DIR` takes precedence over symlink
   ```bash
   # Via mise (.mise.toml in project root)
   [env]
   CLAUDE_CONFIG_DIR = "~/.ccp/profiles/quickfix"

   # Via direnv (.envrc in project root)
   export CLAUDE_CONFIG_DIR="$HOME/.ccp/profiles/quickfix"

   # Via inline command
   CLAUDE_CONFIG_DIR=~/.ccp/profiles/quickfix claude "fix the bug"
   ```

**Parallel execution:** Different terminals can use different profiles via env override while `~/.claude` symlink remains the fallback default.

---

## User Stories

### US-1: Initialize Hub from Existing Setup

**As a** Claude Code user with an existing ~/.claude configuration
**I want to** migrate my current setup into a hub + default profile structure
**So that** I can start managing multiple profiles without losing my current configuration

### US-2: Create New Profile

**As a** user with an initialized hub
**I want to** create a new profile by selecting items from my hub
**So that** I can have a purpose-specific Claude Code configuration

### US-3: Link Hub Item to Profile

**As a** user with multiple profiles
**I want to** add a hub item to an existing profile
**So that** I can evolve my profile configurations over time

### US-4: Validate Profile State

**As a** user who may have manually edited profile directories
**I want to** check if my profile directory matches its profile.yaml manifest
**So that** I can detect and fix configuration drift

### US-5: Switch Profiles via Environment

**As a** user working on different projects with different needs
**I want to** auto-load profiles based on project directory (via mise/direnv)
**So that** the right configuration activates without manual intervention

### US-6: Set Default Active Profile

**As a** user who wants a fallback profile when no env override is set
**I want to** set which profile `~/.claude` points to
**So that** Claude Code works without requiring env configuration in every terminal

### US-7: Reset ccp and Restore Original Setup

**As a** user who wants to uninstall ccp
**I want to** reset ccp and restore my original ~/.claude directory
**So that** I can go back to a standard Claude Code configuration

### US-8: Diagnose Configuration Issues

**As a** user experiencing issues with profiles or symlinks
**I want to** run diagnostics to identify problems
**So that** I can fix broken configurations

### US-9: Auto-Select Profile by Project

**As a** user working on different projects
**I want to** have profiles automatically selected based on project configuration
**So that** I don't need to manually switch profiles when changing projects

### US-10: Compare Profile Configurations

**As a** user managing multiple profiles
**I want to** compare two profiles to see their differences
**So that** I can understand what makes each profile unique

---

## User Journeys

### Journey 1: First-Time Setup

| Step | Actor | Action | System Response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp init` | - |
| 2 | System | Scans ~/.claude | Identifies hub-eligible items |
| 3 | System | Presents migration plan | "Moving 31 skills, 5 hooks, 3 rules to hub" |
| 4 | User | Confirms | - |
| 5 | System | Creates hub structure | ~/.claude/hub/ populated |
| 6 | System | Creates default profile | ~/.claude/profiles/default/ with symlinks |
| 7 | System | Creates shared namespace | ~/.claude/profiles/shared/ |
| 8 | System | Outputs guidance | Shell configuration instructions |

**Success criteria:** User's Claude Code works exactly as before, but hub/profile structure exists.

**Failure paths:**
- ~/.claude doesn't exist → Error with setup instructions
- Permission denied → Error with specific path
- Partial migration fails → Rollback, report which items failed

---

### Journey 2: Create Purpose-Specific Profile

| Step | Actor | Action | System Response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp profile create quickfix` | - |
| 2 | System | Prompts for hub items | Interactive picker or reads flags |
| 3 | User | Selects skills, hooks, rules | - |
| 4 | System | Prompts for data sharing | tasks: shared/isolated? etc. |
| 5 | User | Configures sharing | - |
| 6 | System | Creates profile directory | Full structure with symlinks |
| 7 | System | Generates profile.yaml | Manifest of selections |
| 8 | System | Outputs activation instructions | CLAUDE_CONFIG_DIR example |

**Success criteria:** Profile ready, activates correctly via CLAUDE_CONFIG_DIR.

**Failure paths:**
- Hub not initialized → Error: "Run ccp init first"
- Profile name exists → Error or --force flag
- Selected hub item doesn't exist → Error with available items

---

### Journey 3: Validate and Fix Configuration Drift

| Step | Actor | Action | System Response |
|------|-------|--------|-----------------|
| 1 | User | Runs `ccp profile check quickfix` | - |
| 2 | System | Reads profile.yaml | Parses manifest |
| 3 | System | Compares against directory | Detects differences |
| 4 | System | Reports drift | Missing, extra, broken, mismatched |
| 5 | User | Runs `ccp profile fix quickfix` | - |
| 6 | System | Reconciles directory | Creates/removes symlinks |
| 7 | System | Reports changes | List of actions taken |

**Success criteria:** Profile directory matches manifest exactly.

---

## Acceptance Criteria

### AC-1: Init Command

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

### AC-2: Profile Create Command

```gherkin
GIVEN hub is initialized
WHEN user runs `ccp profile create <name>` with item selections
THEN tool creates ~/.ccp/profiles/<name>/ directory
AND tool inherits directory permissions from current ~/.claude
AND tool creates profile.yaml with selected items and sharing config
AND tool creates symlinks for all selected hub items
AND tool creates CLAUDE.md (composed or template)
AND tool creates settings.json (default or template)
AND data directories are created per sharing config (local dir or symlink to shared/)
```

### AC-3: Profile Link Command

```gherkin
GIVEN profile exists
WHEN user runs `ccp link <profile> skills/<skill-name>`
THEN tool creates symlink in profile's skills/ directory
AND tool updates profile.yaml to include the item
```

### AC-4: Profile Unlink Command

```gherkin
GIVEN profile exists with linked hub item
WHEN user runs `ccp unlink <profile> skills/<skill-name>`
THEN tool removes symlink from profile's skills/ directory
AND tool updates profile.yaml to remove the item
```

### AC-5: Profile Check Command

```gherkin
GIVEN profile exists with profile.yaml
WHEN user runs `ccp profile check <name>`
THEN tool compares yaml manifest against directory state
AND tool reports: missing, extra, broken, mismatched items
AND tool exits 0 if valid, non-zero if drift detected
```

### AC-6: Profile Fix Command

```gherkin
GIVEN profile has configuration drift
WHEN user runs `ccp profile fix <name>`
THEN tool reconciles directory to match profile.yaml
AND tool reports all changes made
AND user can pass --dry-run to preview without changes
```

### AC-7: Profile List Command

```gherkin
GIVEN profiles exist
WHEN user runs `ccp profile list`
THEN tool outputs all profile names
AND tool indicates which profile is currently active (via CLAUDE_CONFIG_DIR)
```

### AC-8: Environment Activation

```gherkin
GIVEN profile exists at ~/.claude/profiles/quickfix
WHEN CLAUDE_CONFIG_DIR is set to that path
THEN Claude Code loads configuration from that profile
AND symlinked hub items are resolved correctly
AND env override takes precedence over ~/.claude symlink
```

### AC-9: Default Profile Switching (ccp use)

```gherkin
GIVEN profile exists at ~/.claude/profiles/quickfix
WHEN user runs `ccp use quickfix`
THEN ~/.claude becomes a symlink to ~/.claude/profiles/quickfix
AND Claude Code uses quickfix profile when no CLAUDE_CONFIG_DIR is set
```

### AC-10: Show Current Default Profile

```gherkin
GIVEN ~/.claude is a symlink to a profile
WHEN user runs `ccp use --show`
THEN tool outputs the name of the currently linked profile
```

### AC-11: Reset Command

```gherkin
GIVEN ccp is initialized with ~/.claude as symlink
WHEN user runs `ccp reset` and confirms
THEN tool copies active profile contents to ~/.claude (replacing symlink)
AND tool preserves directory permissions from the profile
AND tool removes ~/.ccp directory entirely
AND Claude Code continues working with restored ~/.claude directory
```

### AC-12: Doctor Command

```gherkin
GIVEN ccp may have configuration issues
WHEN user runs `ccp doctor`
THEN tool checks: initialization, ~/.claude symlink, hub structure, profile manifests, broken symlinks
AND tool reports status for each check (OK/FAIL/WARN)
AND tool provides remediation instructions for failures
```

### AC-13: Status Command

```gherkin
GIVEN ccp is initialized
WHEN user runs `ccp status`
THEN tool displays: active profile, hub item counts, profile health, overall system health
AND tool indicates profiles with drift or broken links
```

### AC-14: Auto Profile Selection

```gherkin
GIVEN .ccp.yaml exists in current or parent directory with profile: <name>
WHEN user runs `ccp auto`
THEN tool outputs the profile name
AND with --path flag, outputs full profile path
```

### AC-15: Session Command

```gherkin
GIVEN profile exists
WHEN user runs `ccp session <profile>`
THEN tool starts new shell with CLAUDE_CONFIG_DIR set to profile path
AND Claude Code commands in that shell use the specified profile
```

### AC-16: Run Command

```gherkin
GIVEN profile exists
WHEN user runs `ccp run <profile> -- <command> [args]`
THEN tool executes command with CLAUDE_CONFIG_DIR set to profile path
AND command inherits the profile's Claude Code configuration
```

### AC-17: Profile Clone Command

```gherkin
GIVEN source profile exists
WHEN user runs `ccp profile clone <source> <new-name>`
THEN tool creates new profile with copied manifest configuration
AND new profile has same hub links and data sharing config as source
```

### AC-18: Profile Diff Command

```gherkin
GIVEN two profiles exist
WHEN user runs `ccp profile diff <a> <b>`
THEN tool compares hub item links between profiles
AND tool reports items only in A, only in B, and data sharing differences
```

### AC-19: Profile Sync Command

```gherkin
GIVEN profile exists with hub hooks or symlinks
WHEN user runs `ccp profile sync [name]`
THEN tool regenerates symlinks for all hub items in manifest
AND tool removes symlinks not in manifest
AND tool regenerates settings.json with hook configurations
AND each hook includes interpreter prefix and $HOME-based paths
AND supports --all flag to sync all profiles
```

### AC-20: Profile Edit Command

```gherkin
GIVEN profile exists
WHEN user runs `ccp profile edit [name]`
THEN tool allows adding/removing hub items via flags or interactive picker
AND --add-<type>=name adds items to profile
AND --remove-<type>=name removes items from profile
AND -i/--interactive opens tabbed picker with current selections
AND tool syncs symlinks and regenerates settings.json after changes
```

### AC-21: Hub Add Command

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

### AC-22: Hub Remove Command

```gherkin
GIVEN hub item exists
WHEN user runs `ccp hub remove <type>/<name>`
THEN tool warns if item is used by profiles
AND tool removes item from hub after confirmation (or with --force)
```

### AC-23: Hub Show Command

```gherkin
GIVEN hub item exists
WHEN user runs `ccp hub show <type>/<name>`
THEN tool displays item path, type (file/directory), contents or file list
AND tool shows which profiles use this item
```

### AC-24: Usage Command

```gherkin
GIVEN hub and profiles exist
WHEN user runs `ccp usage`
THEN tool displays orphaned items (not used by any profile)
AND tool displays missing items (referenced but not in hub)
AND tool displays shared items (used by multiple profiles)
```

---

## Rejection Criteria (Explicit Non-Goals for MVP)

The system explicitly does NOT:

| Non-Goal | Rationale |
|----------|-----------|
| Provide a web UI or registry | Hub is local filesystem only; community sharing is Phase 2+ |
| Share profiles across machines | No sync, no cloud; user can use git if needed |
| Auto-detect project type | Profile selection is manual or via mise/direnv |
| Support profile inheritance/extends | Flat composition only; inheritance adds complexity |
| Enforce the 20 skill limit | User's responsibility; tool may warn but won't block |
| Manage Claude Code internals | cache/, debug/, telemetry/, statsig/, ide/ are ignored |
| Version control hub items | User can put ~/.claude in git themselves |
| Provide skill/hook authoring | Tool manages organization, not creation |
| Handle conflicts automatically | Broken symlinks are reported, not auto-fixed |
| Merge settings.json files | Copy/template only; no smart merging |

---

## Data Schemas

### profile.yaml

```yaml
# ~/.ccp/profiles/quickfix/profile.yaml

name: quickfix
description: "Minimal bug-fixing configuration"
created: 2025-01-28T10:00:00Z
updated: 2025-01-28T10:00:00Z

# Hub items to link (symlinks created in profile directory)
hub:
  skills:
    - debugging-core
    - git-basics
  hooks:
    - pre-commit-lint
  rules:
    - minimal-change
  commands:
    - quick-test
  md-fragments:
    - base-rules.md

# Data directory sharing configuration
# "shared" = symlink to ~/.ccp/profiles/shared/<name>
# "isolated" = local directory within profile
data:
  tasks: shared
  todos: shared
  paste-cache: shared
  history: isolated
  file-history: isolated
  session-env: isolated
  projects: shared
  plans: isolated

# Hook configuration for settings.json integration
# Specifies hook type (when to run) for each linked hook
hooks:
  - name: pre-commit-lint
    type: PreToolUse           # SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop, SubagentStop
    matcher: "Bash"            # Optional: tool matcher for PreToolUse/PostToolUse
    timeout: 60                # Optional: timeout in seconds (default: 60)
  - name: session-init
    type: SessionStart
```

### Hook Types

| Type | Description |
|------|-------------|
| `SessionStart` | Runs when Claude Code session starts |
| `UserPromptSubmit` | Runs before processing user input |
| `PreToolUse` | Runs before a tool is executed (use `matcher` to filter) |
| `PostToolUse` | Runs after a tool is executed (use `matcher` to filter) |
| `Stop` | Runs when Claude Code session stops |
| `SubagentStop` | Runs when a subagent stops |

### Hub Directory Structure

```
hub/
├── skills/
│   ├── debugging-core/
│   │   └── SKILL.md
│   ├── git-basics/
│   │   └── SKILL.md
│   └── vue-development/
│       └── SKILL.md
├── hooks/
│   ├── pre-commit-lint.sh
│   └── post-task-notify.sh
├── rules/
│   ├── minimal-change.md
│   └── test-first.md
├── commands/
│   ├── quick-test/
│   └── deploy-staging/
└── md-fragments/
    ├── base-rules.md
    ├── code-style.md
    └── documentation-standards.md
```

### Project Config (.ccp.yaml)

Project-level configuration file for automatic profile selection.

```yaml
# .ccp.yaml (in project root)

profile: dev
```

When `ccp auto` is run, it searches for `.ccp.yaml` or `.ccp.yml` in the current directory and parent directories. Use with shell integration:

```bash
# In .bashrc/.zshrc
export CLAUDE_CONFIG_DIR=$(ccp auto --path 2>/dev/null || echo ~/.claude)
```

---

## CLI Command Reference

### Core Commands

| Command | Description | Example |
|---------|-------------|---------|
| `ccp init` | Migrate existing ~/.claude to hub + default profile | `ccp init` |
| `ccp reset` | Undo ccp initialization and restore ~/.claude | `ccp reset` |
| `ccp use <n>` | Set default profile (~/.claude symlink) | `ccp use quickfix` |
| `ccp use --show` | Show current default profile | `ccp use --show` |
| `ccp which` | Show currently active profile | `ccp which` |
| `ccp status` | Show ccp status and health | `ccp status` |
| `ccp doctor` | Diagnose and fix common issues | `ccp doctor` |
| `ccp usage` | Show hub item usage across profiles | `ccp usage` |
| `ccp env <profile>` | Configure project env for a profile | `ccp env dev --format=mise` |

### Profile Commands

| Command | Description | Example |
|---------|-------------|---------|
| `ccp profile create <name>` | Create new profile | `ccp profile create quickfix` |
| `ccp profile list` | List all profiles | `ccp profile list` |
| `ccp profile check <name>` | Validate profile against manifest | `ccp profile check quickfix` |
| `ccp profile fix <name>` | Reconcile profile to match manifest | `ccp profile fix quickfix --dry-run` |
| `ccp profile delete <name>` | Delete a profile | `ccp profile delete quickfix` |
| `ccp profile clone <src> <new>` | Clone an existing profile | `ccp profile clone default dev` |
| `ccp profile diff <a> [b]` | Compare two profiles | `ccp profile diff dev prod` |
| `ccp profile sync [name]` | Regenerate symlinks and settings.json | `ccp profile sync --all` |
| `ccp profile edit [name]` | Add/remove hub items from profile | `ccp profile edit -i` |

### Hub Commands

| Command | Description | Example |
|---------|-------------|---------|
| `ccp hub list [type]` | List hub contents | `ccp hub list skills` |
| `ccp hub add <type> <path>` | Add item to hub | `ccp hub add skills ./my-skill.md` |
| `ccp hub add <type> <name> --from-profile` | Promote profile item to hub | `ccp hub add skills my-skill --from-profile=default` |
| `ccp hub show <type>/<name>` | Show hub item details | `ccp hub show skills/git-basics` |
| `ccp hub edit <type>/<name>` | Edit hub item in $EDITOR | `ccp hub edit hooks/pre-commit.sh` |
| `ccp hub remove <type>/<name>` | Remove item from hub | `ccp hub remove skills/old-skill` |
| `ccp hub rename <type>/<name> <new>` | Rename hub item | `ccp hub rename skills/old new` |

### Link Commands

| Command | Description | Example |
|---------|-------------|---------|
| `ccp link <profile> <path>` | Add hub item to profile | `ccp link quickfix skills/vue-dev` |
| `ccp unlink <profile> <path>` | Remove hub item from profile | `ccp unlink quickfix skills/vue-dev` |

### Workflow Commands

| Command | Description | Example |
|---------|-------------|---------|
| `ccp auto` | Auto-select profile from .ccp.yaml | `ccp auto --path` |
| `ccp session <profile>` | Start shell with profile active | `ccp session dev` |
| `ccp run <profile> -- <cmd>` | Run command with profile | `ccp run minimal -- claude "fix bug"` |

### Command Flags

**`ccp init`**
- `--dry-run` — Show migration plan without executing
- `--force` — Overwrite existing hub structure

**`ccp reset`**
- `--force` — Skip confirmation prompt

**`ccp env`**
- `--format=shell` — Print shell export command (default)
- `--format=mise` — Update mise.toml with CLAUDE_CONFIG_DIR
- `--format=direnv` — Update .envrc with export CLAUDE_CONFIG_DIR

**`ccp profile create`**
- `--skills=a,b,c` — Skills to include
- `--hooks=x,y` — Hooks to include
- `--rules=p,q` — Rules to include
- `--from=<profile>` — Copy configuration from existing profile
- `--interactive` — Interactive picker mode (default if no flags)

**`ccp profile fix`**
- `--dry-run` — Show changes without executing

**`ccp profile sync`**
- `--all` — Sync all profiles

**`ccp profile edit`**
- `--add-skills=a,b` — Add skills to profile
- `--add-hooks=x,y` — Add hooks to profile
- `--add-rules=p,q` — Add rules to profile
- `--add-commands=c,d` — Add commands to profile
- `--add-md-fragments=m,n` — Add md-fragments to profile
- `--remove-skills=a` — Remove skills from profile
- `--remove-hooks=x` — Remove hooks from profile
- `--remove-rules=p` — Remove rules from profile
- `--remove-commands=c` — Remove commands from profile
- `--remove-md-fragments=m` — Remove md-fragments from profile
- `-i, --interactive` — Interactive picker mode (default if no flags)

**`ccp auto`**
- `--path` — Output profile path instead of name

**`ccp hub add`**
- `--from-profile=<name>` — Promote item from profile to hub
- `--replace` — Replace existing hub item if it exists

**`ccp hub remove`**
- `--force` — Skip confirmation and usage check

---

## Assumptions & Dependencies

| Assumption | Impact if Wrong | Mitigation |
|------------|-----------------|------------|
| `CLAUDE_CONFIG_DIR` works with arbitrary paths | Core activation mechanism breaks | Fall back to symlink swap approach |
| Claude Code resolves symlinks for skills/hooks | Hub linking breaks | Use copy instead of symlink (more maintenance) |
| User has mise or direnv for auto-activation | Manual export required | Document manual activation clearly |
| ~/.claude structure is stable across Claude Code versions | Migration logic may break | Version detection + migration paths |
| Symlinks work on user's OS | Windows users may have issues | Document Windows symlink requirements or use junctions |

---

## Open Questions (Deferred to Future Phases)

1. **CLAUDE.md composition** — Should tool support building CLAUDE.md from md-fragments, or just copy a complete file?

2. **settings.json merging** — Same question for settings. Fragment-based composition or complete file?

3. **Profile templates** — Predefined starter profiles (minimal, full-stack, docs-writer)?

4. **Hub item metadata** — Should hub items have their own manifest (description, tags, dependencies)?

5. **Dependency resolution** — If skill A requires hook B, should tool auto-link?

6. **Profile export/import** — Package a profile for sharing (without hub dependency)?

---

## Glossary

| Term | Definition |
|------|------------|
| **Hub** | Central repository of reusable Claude Code components (skills, hooks, rules, etc.) |
| **Profile** | Complete Claude Code configuration directory that can be activated |
| **Shared data** | Data directories (tasks, todos, etc.) that are symlinked across profiles |
| **Isolated data** | Data directories that are local to a specific profile |
| **Manifest** | profile.yaml file that declares what a profile should contain |
| **Drift** | State where profile directory doesn't match its manifest |
| **Hook Type** | Event trigger for hook execution (SessionStart, PreToolUse, etc.) |
| **Project Config** | .ccp.yaml file in project root for automatic profile selection |
| **Session** | Shell environment with CLAUDE_CONFIG_DIR set to a specific profile |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.3 | 2026-01-29 | — | Added: profile edit command (add/remove hub items via flags or picker), enhanced profile sync (regenerates symlinks and settings.json, --all flag), hub add --from-profile (promote profile items to hub), --replace flag for hub add. Hook migration preserves interpreter prefix and uses $HOME-based paths. Reset command rewrites settings.json hook paths. |
| 1.2 | 2026-01-29 | — | Added: permission preservation for init, profile create, and reset commands. Fixed paths in AC-1, AC-2 (was ~/.claude, now ~/.ccp). |
| 1.1 | 2026-01-29 | — | Added: reset, status, doctor, which, auto, session, run, usage commands. Hub CRUD (add, show, edit, remove, rename). Profile clone, diff, sync commands. Hook type configuration for settings.json. Tabbed picker for interactive profile creation. Project config (.ccp.yaml) for auto profile selection. |
| 1.0 | 2025-01-28 | — | Initial specification |
