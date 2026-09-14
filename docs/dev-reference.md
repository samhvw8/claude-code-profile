# Developer Reference

Technical reference for ccp internals. For coding style and workflow, see `CLAUDE.md`. For product spec, see `docs/ccp-spec.md`.

## Architecture

```
internal/
├── config/     # Path resolution, types, CcpConfig (ccp.toml)
├── errors/     # Custom error types
├── hub/        # Hub scanning, item management, settings templates
├── source/     # Unified source system (providers, registries, installer)
├── profile/    # Profile CRUD, manifest, settings generation, sync, drift
├── symlink/    # Platform-specific symlink operations
├── migration/  # YAML→TOML migration, flatten migration, rollback
└── picker/     # Bubble Tea multi-select TUI

cmd/            # Cobra commands (one file per command/subcommand)
```

## Key Types

```go
// internal/config/paths.go
type Paths struct {
    CcpDir          string // ~/.ccp (ccp data directory)
    ClaudeDir       string // ~/.claude or $CLAUDE_CONFIG_DIR (may be project-specific)
    GlobalClaudeDir string // ~/.claude (always global, ignores CLAUDE_CONFIG_DIR)
    HubDir          string // ~/.ccp/hub
    ProfilesDir     string // ~/.ccp/profiles
    SharedDir       string // ~/.ccp/profiles/shared
    StoreDir        string // ~/.ccp/store (shared downloadable resources)
}

type HubItemType string      // skills, agents, hooks, rules, commands
type DataItemType string     // tasks, todos, history, etc.
type PluginStoreItem string  // marketplaces, cache, known_marketplaces.json

// internal/profile/manifest.go
type Manifest struct {
    Version           int           // 3 = current, 2 = TOML, 1 = YAML
    Name, Description string
    SettingsTemplate  string        // Optional settings template name
    Created, Updated  time.Time
    Hub               HubLinks      // What hub items to link
}

// internal/source/types.go
type Source struct {
    Registry, Provider string  // e.g., "github", "git"
    URL, Path          string  // Clone URL and local path
    Ref, Commit        string  // Git ref and pinned commit
    Installed          []string // Installed items (skills/foo, agents/bar)
}

// internal/config/ccp_config.go
type CcpConfig struct {
    DefaultRegistry string         // "skills.sh" or "github"
    GitHub          GitHubConfig   // topics, per_page
    SkillsSh        SkillsShConfig // base_url, limit
}

// internal/hub/template.go
type Template struct {
    Name     string                 // Directory name
    Settings map[string]interface{} // Complete settings.json content (hooks excluded)
}

// internal/profile/generator.go
// Single function replaces previous processor interfaces:
func GenerateSettings(manifest *Manifest, hubDir string) (map[string]interface{}, error)
// Diff/fragment functions:
func DiffSettings(base, current map[string]interface{}) map[string]interface{}
func UpdateFragment(paths *Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error)
func PreviewFragment(paths *Paths, profileDir string, manifest *Manifest) (map[string]interface{}, error)
```

## Settings Templates

Complete `settings.json` templates stored in the hub. Profiles reference a template by name.

```bash
ccp template list                          # List available templates
ccp template show <name>                   # Display template JSON
ccp template create <name>                 # Create new (opens $EDITOR or --from-file)
ccp template extract <name> --from <profile>  # Extract from existing profile's settings
ccp template delete <name>
ccp template edit <name>                   # Edit in $EDITOR

# Use with profiles
ccp profile create <name> --template opus-full
ccp profile edit <name> --template minimal
```

Storage: `~/.ccp/hub/settings-templates/<name>/settings.json`

Hooks are always overlaid from hub hooks, not stored in templates.

### Fragment Capture

Capture manual edits to `settings.json` as a per-profile fragment so they survive regeneration:

```bash
ccp profile capture              # Capture from active profile
ccp profile capture dev          # Capture from named profile
ccp profile capture --dry-run    # Show diff without saving
```

Computes `DiffSettings(base_template, current_settings)` — strips hooks, saves only keys that differ from the base template as `settings-fragment.json`. If no template is set, all non-hook keys become the fragment. If no diff exists, removes any stale fragment file.

## Bundles

An atomic, non-separable group of hub items (skills, agents, hooks, rules, commands). Members live *inside* the bundle directory, so they can only be linked or removed as a unit — never individually.

```bash
ccp bundle create <name>                      # Interactive multi-select composer
ccp bundle create <name> --skill a --hook b   # Non-interactive (repeatable flags)
ccp bundle list
ccp bundle show <name>
ccp bundle remove <name>

# Link/unlink as a unit (expands to per-member symlinks)
ccp link <profile> bundles/<name>
ccp unlink <profile> bundles/<name>
```

Storage: `~/.ccp/hub/bundles/<name>/` holds `bundle.yaml` plus the members under `skills/`, `agents/`, `hooks/`, etc. The composer **copies** selected hub items in (originals untouched). The profile manifest records only the bundle name (`[hub] bundles = [...]`); linking materializes per-member symlinks into the profile's leaf dirs and merges any bundle hooks into the generated `settings.json`. Bundles are a *composite* type, intentionally excluded from `config.AllHubItemTypes()` so leaf-item loops (scan, drift, settings) never treat one as a leaf.

Member names are validated when the bundle loads (`hub.ValidateItemName`, the same rule `LoadManifest` applies to manifest items): a name must be a relative path that stays inside its item directory. Names become link names inside the profile and inside each harness, so a name that climbs with `..` would send that write outside the tree ccp owns — and a bundle manifest can be pulled from a source registry, so it is not ccp's own input. Nested names (`group/tone.md`, how the scanner flattens rule directories) stay valid.

## Project Setup

Copy hub items into a project's `.claude/` directory for project-scoped Claude Code config. Items are copied (not symlinked) so the project is self-contained and git-committable.

```bash
ccp project add skills/coding agents/reviewer   # Copy from hub
ccp project add -i                               # Interactive picker
ccp project install owner/repo skills/my-skill   # Install from source directly
ccp project install owner/repo --all             # Install all from source
ccp project install owner/repo -i                # Interactive source install
ccp project list                                 # List project's .claude/ items
ccp project remove skills/coding                 # Remove from project
```

Detects project root via `.git/` directory (walk up from cwd). Override with `--dir`. Valid types: skills, agents, hooks, rules, commands.

`project add` copies from the local hub. `project install` fetches from a source (GitHub/skills.sh) directly into the project — items are not tracked in the registry and overwrite existing items.

> ccp is the authoring tool, `.claude/` is the distribution format. One person runs `ccp project add`, commits `.claude/`, and the team uses Claude Code without needing ccp.

## Source System

Unified source management for skills, agents, and plugins:

```bash
ccp find <query>                        # Search skills.sh (shows PACKAGE + SKILL columns)
ccp find -r github <query>              # Search GitHub repos
ccp install                             # Sync all from ccp.toml (for machine migration)
ccp install <owner/repo>                # Auto-add + interactive install (recommended)
ccp install <owner/repo> skills/<name>  # Install specific skill directly
ccp install <owner/repo> -a             # Auto-add + install all items
ccp install <github-url>/blob/<ref>/SKILL.md  # Auto-add repo + install that one skill
ccp source add <owner/repo>             # Add source only (falls back to GitHub if not on skills.sh)
ccp source list                         # List installed sources
ccp source update [name]                # Update sources
ccp source remove <name>                # Remove source
```

### Source Workflow

1. `find` searches skills.sh by default, shows PACKAGE (owner/repo) and SKILL separately
2. `install <owner/repo>` auto-adds source if not found, then shows interactive picker
3. `install <owner/repo> skills/<name>` installs a specific skill directly without picker
4. `install <github-blob-url>` to a `SKILL.md` auto-adds the repo (at the URL's ref) and installs just that skill via `InstallPath` — `ParseGitWebURL` handles `/blob/`, `/tree/`, raw, and GitLab `/-/blob/` URLs
5. `install` (no args) syncs all sources from ccp.toml - clones missing sources and reinstalls items
6. `source add` tries skills.sh first, falls back to GitHub with default branch

### Bootstrap (chezmoi sync)

```bash
ccp bootstrap          # Pull: sync sources + fix all profiles (run after chezmoi apply)
ccp bootstrap --push   # Push: add local-only hub items to chezmoi (skip source-installed)
```

Designed for multi-machine sync via chezmoi. Source-installed items (tracked in `ccp.toml`) are re-downloaded, not synced — only local-only hub items, profiles, and `ccp.toml` go through chezmoi.

**New machine:** `chezmoi apply && ccp bootstrap`
**After changes:** `ccp bootstrap --push`

## Hooks Format

Hooks use the official Claude Code `hooks.json` format for plugin compatibility:

```
~/.ccp/hub/hooks/{name}/
├── hooks.json         # Official format
└── scripts/           # Scripts directory
    └── script.sh
```

### hooks.json Structure

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/scripts/session-start.sh",
            "timeout": 60
          }
        ]
      }
    ]
  }
}
```

### Hook Types

```go
// internal/config/hooks.go
type HooksJSON struct {
    Hooks map[HookType][]HookEntry `json:"hooks"`
}

type HookEntry struct {
    Matcher string        `json:"matcher,omitempty"`
    Hooks   []HookCommand `json:"hooks"`
}

type HookCommand struct {
    Type    string `json:"type"`              // Always "command"
    Command string `json:"command"`           // Path or ${CLAUDE_PLUGIN_ROOT}/...
    Timeout int    `json:"timeout,omitempty"`
}
```

### Hook Event Types

- `SessionStart` - Session startup, resume, clear, compact
- `UserPromptSubmit` - User submits a prompt
- `PreToolUse` / `PostToolUse` - Before/after tool execution (use `matcher`)
- `Stop` / `SubagentStop` - Session or subagent stops

### Backward Compatibility

Legacy `hook.yaml` format still supported for reading. `GetHookManifest()` tries `hooks.json` first, falls back to `hook.yaml`.

## Codex Skill Portability

Cross-tool compatibility with OpenAI Codex CLI. Skills use the same SKILL.md format — zero conversion needed.

### Source Discovery

`DiscoverItems()` scans Codex directory layouts alongside Claude Code layouts:

| Path | Format | Priority |
|------|--------|----------|
| `skills/` | Root-level (shared) | First |
| `.claude/skills/` | Claude Code plugin | Second |
| `.agents/skills/` | Codex cross-tool standard | Third |
| `.codex/skills/` | Codex project-level | Fourth |
| `.codex-plugin/plugin.json` | Codex plugin manifest | Fifth |
| `SKILL.md` at repo root | Bare skill repo (whole repo = one skill) | Last (fallback) |

`resolveItemPaths()` falls back through these directories when resolving items.

A **bare skill repo** has `SKILL.md` at the repository root with no `skills/<name>/` wrapper (e.g. `colbymchenry/frontend-audit-skill`). `DiscoverItems()` detects the root `SKILL.md`, derives the item name from its frontmatter `name:` field (falling back to the source dir name), and installs the whole repo as `skills/<name>`. `CopyDir()` skips `.git` so VCS metadata is never copied into the hub item.

Codex *consumption* of the hub is not implemented: there is no `ccp codex` command, and no `[codex]` config section. Codex reads `.agents/skills/` at the project level, and omp reads project `.claude/` itself, so `ccp project add` covers the project-scoped case for both without a per-harness command.

## omp (oh-my-pi) Portability

omp reads its own user config from `~/.omp/agent/` (skills, agents, rules, commands, hooks, tools, settings) and only loads *foreign* user-level config — including `~/.claude` — when its `enabledProviders` setting opts in (default: empty). A ccp profile symlinked at `~/.claude` is therefore invisible to omp by default. ccp links hub items into omp's own directories instead, which omp always loads.

### What Is Linked

`skills/`, `agents/` and `commands/` are linked one directory down, in the shape
omp discovers. `rules/` is **not** a link type: omp surfaces a rule only when its
frontmatter routes it somewhere (a description into the rulebook, `alwaysApply`
into every session, a condition into TTSR), so a link whose frontmatter omp does
not honor is a rule nobody loads — silently, with no error and no trace. Every
profile rule therefore travels in `RULES.md`, which ccp generates and omp injects
into every session: always on, exactly as Claude Code treats a rule.

`ompLoadabilityOf()` still encodes omp's loader rules (checked against omp
18.1.19), but it is a **reporter**: its verdict is printed as a note and nothing
else depends on it. An item omp ignores is linked anyway and reported with its
reason — a wrong guess about another program's loader must never take
configuration away.

| Hub item | omp location | Reported as ignored when |
|----------|--------------|--------------------------|
| `skills/<name>` | `~/.omp/agent/skills/<name>` | no `SKILL.md`, or no non-empty `description`, or `enabled: false` |
| `commands/<name>.md` | `~/.omp/agent/commands/<name>.md` | not a `.md` file |
| `agents/<name>.md` | `~/.omp/agent/agents/<name>.md` | no `name`, no `description`, or `model: inherit` (omp resolves no such model, so the agent fails to spawn) |

Values are tested, not key presence: an empty `description` is no description.
Frontmatter parsing mirrors omp's own permissiveness (line-based fallback for
prose that breaks YAML, BOM tolerated, CRLF fences tolerated).

`hooks/` is not a link type at all. omp imports hook *modules* from `hooks/pre`
and `hooks/post` — where real omp hooks live — and a Claude `hooks.json` can
never be imported, so ccp neither links hooks nor scans that directory.
`settings-templates/` is excluded for the same class of reason: `config.yml` and
`settings.json` are different schemas.

Link identity is resolved, never textual: a recorded target is joined against the
link's *resolved* parent (`resolveCcpLink`), matching how `ompCreateLink` writes
it, so a dotfiles-managed `~/.omp` whose symlink points at a deeper directory
cannot make ccp disown the links it wrote itself. A broken link is still
recognized as ccp's — its target names the hub item — and `ccp doctor --fix`
prunes it once that item is gone.

Every agent directory ccp may have written into — the default one, each
`profiles/<name>/agent`, and a `PI_CODING_AGENT_DIR` override — comes from one
function, de-duplicated by resolved path, so teardown covers a tree the current
environment does not select and never removes the same directory twice. The
opt-in test (`OmpOptedIn`) asks about the tree ccp is about to write into, not
every tree it ever wrote into: opting into omp in one named profile is not
consent to start writing into the default one.

Two things are never overwritten, because they may hold content ccp did not
create: a path that already exists and is not a ccp link, and a profile item that
cannot be read. `sync` reports both and carries on; `link` fails on the specific
item it was asked for. An unreadable rule is skipped and reported, never fatal.

Bundle members resolve inside their bundle directory; a bundle copy of an item
the profile also links directly is the same item.

### Verifying What omp Actually Loaded

Everything above predicts omp's loader, and a prediction that is wrong is
invisible: the link is in place, omp never reads it, and nothing fails. `ccp
doctor --verify-omp` asks omp itself — it runs `omp --mode rpc --no-session`
with a `get_state` request and reads the system prompt omp built — then checks
that the text of every rule and of `AGENTS.md` reached it. That probe needs a
configured model, starts no session and spends no model tokens, and touches
omp's own state, so it is opt-in.

Items are only checked by name, and a name omp never mentions is reported as
information, never as a fault: omp picks skill metadata through its own filters
(`skillful`, `ignoredSkills`, `includeSkills`, custom directories) and names its
agents through tool schemas rather than the prompt.

### Commands

```bash
ccp omp link skills/debugging commands/ship.md  # Symlink hub items into omp
ccp omp link                                    # Interactive selection picker
ccp omp sync                                    # Mirror the active profile to omp
ccp omp sync dev                                # Mirror a named profile
ccp omp list                                    # Show links and diff vs active profile
ccp omp unlink skills/debugging                 # Remove ccp links from omp
ccp omp unlink                                  # Interactive selection picker
ccp omp unlink --all                            # Remove everything ccp set up there
ccp omp --profile work sync                     # Target an omp named profile
ccp doctor --verify-omp                         # Ask a live omp what its prompt carries
```

`ccp omp` is a hidden command (power-user tier, like `run`/`env`/`usage`).

`sync` is manifest-driven (bundle members included) and therefore owns omp's skills/, agents/ and commands/ directories: links the profile does not use are removed, along with rule links older ccp versions created, while foreign links and real files are left untouched. A link is never removed because ccp thinks omp will not load it — that verdict only produces a note. `--profile <name>` is exactly "as if you had exported `OMP_PROFILE`", which is what omp itself reads; a name that is not a single path element is an error rather than a silent fallback to the default tree.

### Following Profile Switches

Once ccp owns anything in omp, `ccp use <profile> -g` mirrors the newly active profile: it links what the new profile wants and **retires the items the outgoing profile wanted**, read from the outgoing profile's manifest *before* the symlink moves. A link no profile asked for (made with `ccp omp link`) is never touched by a switch — it is reported instead:

```
Note: 2 omp link(s) are not in profile 'dev' — 'ccp omp sync' would prune them
```

`profile.OmpFollowProfile()` is a no-op while ccp owns nothing in the tree it would write into, so the opt-in is the act of linking *there*; the check covers the context files too, so a profile whose only trace is `AGENTS.md`/`RULES.md` still follows. A failed sync warns instead of failing the switch, since the switch already happened.

Project-scoped `ccp use <profile>` (no `-g`) does not touch omp: omp has no per-directory user-level config, and its project roots (`<project>/.claude/`, `<project>/.omp/`) already cover that case. `ccp bootstrap` mirrors the active profile only when ccp already owns something in omp, and otherwise prints a one-line hint — writing a global context file into a harness the user never linked would silently change every omp session.

### Scope: Project Config Already Works

omp's `enabledProviders` gate only covers *foreign user-level* roots; project roots load by default. A project set up with `ccp project add` (items copied into `<project>/.claude/`) is therefore already visible to omp in that project — verified with a project-level `.claude/skills/` skill. `ccp omp` exists for the other half: user-level content, where omp otherwise sees nothing because `~/.claude` is opt-in.

### Global Context

`sync` also sets up the two user-level files omp reads in every session, sourced from the profile:

| File | Source | Notes |
|------|--------|-------|
| `~/.omp/agent/AGENTS.md` | symlink → `<profile>/CLAUDE.md` | omp's user context file. The next sync re-points it when the active profile changes, so one rule file per harness |
| `~/.omp/agent/RULES.md` | generated from **every** rule in the profile | omp's sticky always-apply rule: the whole file is injected into every session's system prompt, so keep the profile's rule set small (a 14 KB / 6-rule set is injected verbatim). Rules are never linked into omp's `rules/` directory, because omp surfaces a rule only when its frontmatter routes it somewhere — a rule that fails to route would be dropped without a word. Nothing is injected twice |

`RULES.md` carries a `<!-- generated by ccp … -->` marker; a file without it is never overwritten, and a profile with no rules has ccp's file removed. This mirrors what the active profile injects on the Claude Code side, and it is the one carrier a rule needs: whether the user's own `~/.omp/agent/rules/` already holds a file of that name changes nothing.

### Health

`ccp omp list [profile]` lists what is linked and how far omp is from a profile — the active one by default, or a named one. `missing` is what `sync` would add; `stale` is what it would remove *or re-point* (a link whose target no longer matches its name counts as drift, as does a rule link an older ccp left behind); `context:` lines report the two profile-level files (`AGENTS.md`, `RULES.md`, with the size injected into every session); `ignored:` lines report items omp ignores — linked anyway, with the reason a prediction gave, since only a guess says omp will not read them. `--json` emits `agent_dir`, `linked`, `profile`, `missing`, `stale`, `ignored`, `unreadable`.

`ccp doctor` inspects omp once ccp has linked anything: broken links (the hub item behind them is gone) are listed and `ccp doctor --fix` removes them; link drift and context drift against the active profile are reported as pointers to `ccp omp sync`. Without any ccp links the check prints `OK (nothing linked)`, so users who never opted in see nothing.

### Teardown

`OmpManaged()` enumerates everything ccp owns in omp — the type-directory links (including rule links an older ccp left behind), the `AGENTS.md` link, and a marker-carrying `RULES.md` — across the default agent directory, *every* named omp profile, and a `PI_CODING_AGENT_DIR` override, de-duplicated by resolved path so no directory is removed twice. `OmpTeardown()` removes exactly that. Two commands use it: `ccp omp unlink --all` and `ccp reset`, which would otherwise delete `~/.ccp` and leave every omp link dangling. Foreign links and real files are never touched; empty directories are left in place.

The opt-in test is narrower than teardown on purpose: `OmpOptedIn()` asks whether ccp owns anything in the directory it is about to write into, so a profile switch never starts writing into a tree the user did not opt into — including the default tree when they opted in through a named profile.

### Path Resolution

`OmpAgentDir()` mirrors omp's own resolution: a named profile (`OMP_PROFILE`, falling back to the legacy `PI_PROFILE`) relocates the user base to `~/.omp/profiles/<name>/agent` and ignores `PI_CODING_AGENT_DIR`; `default`, empty, and whitespace select the default profile. Otherwise `PI_CODING_AGENT_DIR` overrides `~/.omp/agent`. The config root name comes from `PI_CONFIG_DIR` (default `.omp`).

So `OMP_PROFILE=work ccp omp sync` — or its equivalent, `ccp omp --profile work sync` — populates the `work` omp profile, and a plain run targets the default profile. Launching `omp --profile <name>` without exporting `OMP_PROFILE` (for example via a shell alias created with `omp --alias`) is invisible to ccp, which is why the flag exists.

## Configuration

Global config at `~/.ccp/ccp.toml`:

```toml
default_registry = "skills.sh"

[github]
topics = ["agent-skills", "claude-code", "claude-skills"]
per_page = 10

[skillssh]
base_url = "https://skills.sh"
limit = 10

# Installed sources (auto-managed by ccp source commands)
# Paths are relative to ~/.ccp/ for portability
[sources.'owner/repo']
registry = 'github'
provider = 'git'
url = 'https://github.com/owner/repo.git'
path = 'sources/owner--repo'
ref = 'main'
commit = 'abc123...'
installed = ['skills/my-skill', 'agents/my-agent']
```

Generate default config: `ccp config init`

## Directory Structure

```
~/.ccp/
├── hub/                        # Human-configurable (ccp-managed)
│   ├── skills/
│   ├── agents/
│   ├── hooks/
│   ├── rules/
│   ├── commands/
│   ├── settings-templates/     # Complete settings.json templates
│   └── bundles/                # Atomic groups: skill+agent+hook linked together
├── store/                      # Shared downloadable resources
│   └── plugins/
│       ├── marketplaces/       # Downloaded marketplace repos
│       ├── cache/              # Plugin cache
│       ├── known_marketplaces.json
│       └── install-counts-cache.json
├── sources/                    # Cloned source repositories
├── profiles/
│   ├── shared/                 # Shared runtime data (all data dirs default here)
│   │   ├── tasks/
│   │   ├── todos/
│   │   ├── paste-cache/
│   │   └── projects/
│   └── {name}/                 # Individual profile
│       ├── profile.toml        # Profile manifest
│       ├── skills/ → hub/skills/{linked}
│       ├── agents/ → hub/agents/{linked}
│       ├── plugins/
│       │   ├── marketplaces → store/plugins/marketplaces
│       │   ├── cache → store/plugins/cache
│       │   └── installed_plugins.json  # Profile-specific
│       └── ...
└── ccp.toml                    # Config + installed sources
```

### Data Classification

| Type | Category | Location | Sharing |
|------|----------|----------|---------|
| Hub items (skills, agents, hooks) | Human-config | `~/.ccp/hub/` | Linked per profile |
| Plugin cache (marketplaces, cache) | Human-config | `~/.ccp/store/plugins/` | Shared via symlinks |
| Runtime data (tasks, todos, history) | Runtime | `~/.ccp/profiles/shared/` | Always shared |
| Plugin state (installed_plugins.json) | Runtime | Profile `plugins/` | Isolated |
