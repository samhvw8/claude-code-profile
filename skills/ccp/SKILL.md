---
name: ccp
description: "Use and manage ccp — the Claude Code Profile Manager. Use when: switching profiles, creating profiles, installing skills from sources, managing hub items, linking/unlinking items, setting up ccp for the first time, activating profiles per-project. Actions: switch, create, install, link, unlink, init, find, list, update profiles and hub items. Keywords: ccp use, ccp init, profile switch, hub link, source add, install skill, settings template, find skills, profile create. Also: Claude accounts and account pools — add a Claude account or login, several accounts or seats for one client or workspace, share seats across profiles with a pool, auto-pick the account with the most limit left, load-balance logins, ccp account, ccp account pool, --account auto, --account auto:<pool>, ccp account use, ccp use --accounts [--pool] (deprecated), move or remove an account, make plain claude use an account by default. Also: project-only skills — link a team or private repo's skills into one project for this machine only, ccp project install --link, a project ccp.toml, ccp project update. Casual triggers: 'add another Claude account', 'my company gave me more accounts', 'which account has limit left', 'switch to my dev profile', 'install a skill', 'set up ccp', 'find skills on github', 'which profile am I using', 'add this skill to my profile', 'add these skills only to this project', 'link my company repo's skills into this repo', 'how do I use ccp'."
---

# ccp — Claude Code Profile Manager

Manage multiple Claude Code configurations via a central hub of reusable components.

## Installation

Before using ccp commands, check if ccp is installed. If not, install it automatically:

```bash
# Check if ccp exists
command -v ccp >/dev/null 2>&1

# If not installed, install via mise (preferred)
# Step 1: Install mise if needed
if ! command -v mise >/dev/null 2>&1; then
  curl https://mise.jdx.dev/install.sh | sh
  # Add to shell (user may need to restart shell)
  eval "$(~/.local/bin/mise activate)"
fi

# Step 2: Install ccp via mise (prebuilt release binary)
mise use -g github:samhvw8/claude-code-profile
# or build from source: go install github.com/samhvw8/claude-code-profile/cmd/ccp@latest
```

After install, initialize: `ccp init`

## Core Concepts

| Concept | What |
|---------|------|
| **Hub** | Central directory of reusable items (~/.ccp/hub/) |
| **Profile** | Named config referencing hub items via symlinks |
| **Source** | External repo providing installable items |
| **Settings Template** | Complete settings.json stored in hub |
| **Activation** | How a profile becomes active (global symlink or env var), optionally under an account |

## Common Workflows

### Switch Profiles
```bash
ccp use dev -g              # Global: ~/.claude → ~/.ccp/profiles/dev
ccp use quickfix            # Project: updates mise.toml or .envrc
ccp which                   # Show active profile
```

### Find and Install Skills
```bash
ccp find <query>                          # Search skills.sh
ccp find -r github <query>               # Search GitHub
ccp install owner/repo                    # Add source + interactive install
ccp install owner/repo -a                 # Install all items
ccp install owner/repo skills/x --link .    # Install and link to the active profile
ccp install owner/repo skills/x --link dev  # Install and link to 'dev'
ccp install owner/repo skills/x --clone     # Editable local copy instead of a shadow link
```

### Link Items to Profiles

The profile name comes first, then one `type/name` path. Agents and rules keep
their `.md` extension (`agents/reviewer.md`).

```bash
ccp link default skills/coding            # Link one item to profile 'default'
ccp link default agents/reviewer.md
ccp unlink default skills/coding          # Remove one item
ccp link                                  # Interactive picker for the active profile
ccp link dev                              # Interactive picker for 'dev'
ccp hub add skills browser-skill --from-profile=default  # Move a profile-local item into the hub, linked back
```

Linking or unlinking a hook updates `settings.json`'s `hooks` right away; no
`ccp profile sync` needed.

### Settings

`settings.json` is generated from the settings template, the profile's
`settings-fragment.json` and its linked hooks. Edits made directly to
`settings.json` (by hand or by Claude Code) survive only once captured:

```bash
ccp profile capture default               # Save non-hook edits into the fragment
ccp profile sync default                  # Relink items; regenerate settings.json
ccp profile sync default --force          # Regenerate even if it drops uncaptured edits
```

Without `--force`, sync never drops uncaptured edits: it updates hooks only and
names the keys; `ccp profile capture` followed by sync then applies everything. Template switches and fragment edits apply normally,
and hooks you added yourself (by hand or via `/hooks`) are always kept.

### Sources
```bash
ccp source list                           # List installed sources
ccp source update                         # Update all sources
ccp source add owner/repo                 # Add a source
```

### Link Items to Other Agents

omp (oh-my-pi) does not read `~/.claude` unless opted in, so ccp mirrors a profile into its own config directory — but only what omp actually loads. Rules and Claude hooks are reported as inert instead of being symlinked into directories omp ignores (`RULES.md` carries the rules).

```bash
ccp omp link skills/debugging commands/ship.md  # Link hub items to omp
ccp omp link                                    # Interactive picker (no args)
ccp omp sync                                    # Reconcile omp with active profile
ccp omp list                                    # Links + missing/stale/inert vs profile
ccp omp unlink skills/debugging                 # Remove ccp links
ccp omp --profile work sync                     # Target an omp named profile
```

After the first link, `ccp use <profile> -g` mirrors the new profile additively
(hand-made links survive and are reported, not pruned) and refreshes omp's global
context (`AGENTS.md` → the profile's `CLAUDE.md` when not overridden by a
harness-tailored `AGENTS.md`, plus `RULES.md` from its rules).
`ccp doctor` reports broken omp links when the hub item behind them is gone, and
`ccp doctor --verify-omp` asks a live `omp` process what reached its system prompt.
`ccp omp` is a hidden command.

opencode reads `~/.claude/CLAUDE.md` and `~/.claude/skills/` itself; point its
`instructions` at `"{env:HOME}/.claude/rules/*.md"` for rules. Agents and
commands need converting, which ccp does:

```bash
ccp opencode sync            # Write the active profile's agents/commands into ~/.config/opencode
```

After the first sync, `ccp use -g`, link/unlink and `ccp bootstrap` keep it
current. ccp only replaces files it wrote; hooks are not mirrored (opencode
uses plugins). `ccp opencode` is a hidden command.

Antigravity CLI (agy) reads nothing from `~/.claude`, so ccp sets up
`~/.gemini/config` from the profile: `AGENTS.md` links to `CLAUDE.md`, the
output style goes to `GEMINI.md`, rules and agents become converted copies
(agy triggers and tool names), and skills are linked:

```bash
ccp agy sync                 # Mirror the active profile into ~/.gemini/config
```

After the first sync, `ccp use -g`, link/unlink and `ccp bootstrap` keep it
current. ccp only replaces files it wrote and never writes through a symlinked
`rules/`, `agents/` or `skills/`; hooks and commands are not mirrored. `ccp agy`
is a hidden command.

### Zero-LLM Sync + Skill Policy Adaptation

Syncing across harnesses is zero-LLM by default, with this skill used only when
adding or changing tool policy:

1. **Zero-LLM mechanical sync & live RPC verification (`ccp` CLI — default):**
   - Keep `<profile>/CLAUDE.md`, `hub/rules/`, and `hub/skills/` as the single
     source of truth (naming both Claude Code and `omp` tool bindings where they
     differ).
   - Run `ccp omp sync && ccp opencode sync` to link `AGENTS.md -> <profile>/CLAUDE.md`,
     `skills/`, `agents/`, `commands/`, `RULES.md`, and `APPEND_SYSTEM.md`.
   - Run `ccp doctor --verify-omp` to verify in 0 LLM tokens via
     `omp --mode rpc --no-session` (`get_state`) and `~/.omp/agent/config.yml`
     (`ignoredSkills`, `agentModelOverrides`) that every rule, context file, and
     active item loaded cleanly.
2. **Policy & tool-matching adaptation (via this skill when onboarding tools):**
   When asked to adapt `<profile>/CLAUDE.md`, `AGENTS.md`, or hub skills to new
   tools or MCP servers on the active harness:
   - **Query the current harness's live tools and MCPs:**
     - On `omp`: inspect built-in tools in `eval` (`Object.keys(tool)` — `eval`,
       `task`, `find`, `grep`, `glob`, `read`, `edit`, `write`, `bash`,
       `web_search`, `ask`, `todo`, `wait`), `eval` prelude helpers
       (`await tool.mcp__<server>_<tool>(args)`, `await tool.<name>(args)`,
       `workpool`, `agent`, `judge`, `judge_batch`, `browser`), and mounted
       `xd://` devices (`xd://lsp`, `xd://ast_edit`, `mcp__codegraph_*`,
       `mcp__parallax_*`, `mcp__cua_*`, `mcp__notebooklm_*`, `mcp__rea_*`).
     - On Claude Code: inspect `.claude/settings.json` (`mcpServers`,
       `enabledPlugins`) and built-in tools (`Agent`, `Skill`, `ToolSearch`,
       `Read`, `Edit`, `Write`, `Grep`, `Glob`, `Bash`, `WebSearch`, `WebFetch`).
   - **Update the single source of truth (`<profile>/CLAUDE.md`, `hub/rules/`, `hub/skills/`):**
     - Map abstract capabilities to each harness's tools:
       - **Batch/chain 2+ MCP or tool calls:** `eval` (`await tool.mcp__*`,
         `await tool.*`, `workpool`, `judge`, `browser`) on `omp`.
       - **Web search & fetch:** `omp` `web_search` and `read` (URLs) alongside
         Parallax MCP on `omp`; Parallax MCP over `WebSearch`/`WebFetch` on
         Claude Code.
       - **Delegation & live teams:** `task` (`name` + `write agent://<id>`) and
         `eval` `workpool`/`agent` on `omp`; `Agent`
         (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`) on Claude Code.
   - **Verify:** Run `ccp omp sync && ccp opencode sync && ccp doctor --verify-omp`
     (plus `mise run dot:check` when dotfiles-managed).

### Output styles

`output-styles` is a hub item type. Link a style, then select it with
`outputStyle` in the profile's settings (template or `settings-fragment.json`):

```bash
ccp link default output-styles/ste-lite.md
ccp pi sync                  # pi: write the active style into ~/.pi/agent/APPEND_SYSTEM.md
```

omp gets it in `APPEND_SYSTEM.md` on `ccp omp sync`. opencode gets
`~/.config/opencode/output-style.md` on `ccp opencode sync`; add that path to
`instructions` once. ccp refuses to link into a profile type directory that is a
symlink, such as a hand-made `~/.claude/output-styles` link: remove that link
first.

## Picker Controls (fzf-style)

| Key | Normal Mode | Search Mode (after /) |
|-----|-------------|----------------------|
| `↑`/`↓` | Navigate | Navigate |
| `Tab`/`Space` | Toggle item | Toggle item |
| `a` | Select all/none | — |
| `f` | Filter checked/unchecked | — |
| `Enter` | Confirm | Confirm |
| `Esc` | — | Clear search |

## Activation Modes

| Mode | Command | How It Works |
|------|---------|--------------|
| Global | `ccp use dev -g` | ~/.claude symlink → profile dir |
| Project | `ccp use dev` | Updates mise.toml/envrc |
| Inline | env var | `CLAUDE_CONFIG_DIR=~/.ccp/profiles/dev claude` |
| Account | `--account <name>\|auto\|auto:<pool>`, `ccp account use <name>\|auto\|auto:<pool>` | One profile, several Claude logins (see below) |

## Accounts and pools

An **account** is one Claude login: a folder `~/.ccp/accounts/<name>/` with its own login that
links everything else (skills, settings, history) to one profile. **The profile's accounts** are
the accounts linked to it; `--account auto` picks among them, never from another profile.

```text
profile companyA ── ca1, ca2, ca3     (the companyA workspace's accounts)
profile acme      ── acme1, acme2      (another client's accounts)
profile default   ── (no accounts: plain claude, personal login)
```

A **pool** is an optional name on an account (`pool = "company"` in its `.ccp-account.toml`), for
seats shared across profiles. `--account auto:<pool>` picks the pool's account with the most limit
left **whatever profile it is linked to**: `ccp run` relinks a Claude account to the requested
profile, but only while no Claude Code session runs on it; agy accounts run on any profile as is.
`ccp env` and `ccp session` never relink: they only take pool members already linked to the
profile. An account is in at most one pool.

Rules that shape every recipe:

- Claude Code keeps one login per config folder (the Keychain entry is keyed by the folder path;
  on Linux it is `<folder>/.credentials.json`). A login cannot be copied or shared between
  folders, so each account folder needs its own `/login` once.
- A Claude account is linked to one profile at a time. To use one seat from several profiles,
  put it in a pool and start it with `--account auto:<pool>` (it moves when free); or add a
  second account (another name, the other profile) and log in again.
- `/login` approves with the claude.ai account signed in to the browser. To log in a
  **different** account, sign out of claude.ai first, or open the login URL Claude Code shows in
  an Incognito window. Otherwise every account gets the same login.

### See the accounts

```bash
ccp account          # every account (HARNESS, PROFILE, POOL, usage) + read-only "Other logins (not accounts)" (Claude global/profile, macOS plain agy)
ccp account --json   # managed accounts only, for scripts
ccp status           # also lists managed accounts
```

### Add accounts to a profile

```bash
ccp account add ca4 companyA                  # Claude account (default); profile defaults to current
ccp run --account ca4                          # Claude Code on ca4: type /login once
ccp account add agy1 companyA --harness agy   # agy account (file token storage in ~/.gemini/antigravity-cli-ccp-agy1)
ccp run --account agy1                         # agy on agy1: sign in once
ccp run companyA --account auto -- agy        # auto-pick among the profile's agy accounts
ccp run --account agy1 -- --model <model>      # flags only after --: they go to the account's harness (agy here)
ccp account add ca5 companyA --pool company   # ...and put it in pool company at once
ccp account pool ca1 company                   # put an existing account in a pool
ccp account pool ca1 --clear                   # take it out
ccp account remove ca4                         # log ca4 out: delete only its login, keep folder and pool; asks y/N
ccp account remove ca4 --all                   # delete the login, the account folder and (agy) its data dir
ccp account remove ca4 --force                 # no question (needed without a terminal, and with --json); never skips the running-session check
ccp account                                    # check: shows HARNESS, PROFILE, POOL, email, and usage
```

Pick short names that say the profile (`ca1`, `acme1`). `auto` is reserved. `ccp env`, `ccp session`,
and `ccp account use` are Claude-only; start `agy` accounts with `ccp run`.

### Make plain `claude` use a profile's accounts (`ccp account use`)

`ccp use` picks the **profile** a place loads; `ccp account use` picks the **account** that profile
runs on, wherever it loads (globally or in any project that uses it):

```bash
ccp account use default --profile default     # profile default -> account default
ccp account use auto --profile acme           # acme -> its freest logged-in account
ccp account use auto:company --profile acme   # acme -> the pool's freest, relinked when free
ccp account use                               # show the current profile's rule
ccp account use --clear --profile acme        # plain claude on acme runs on the profile again
```

Without `--profile` the rule goes to the current profile (the project's `CLAUDE_CONFIG_DIR`, else
`~/.claude`). A named account must be a Claude account linked to that profile. The rule is
`account = "..."` in the profile's `profile.toml`; `ccp account` lists every rule.

The first `ccp account use` writes the shim `~/.ccp/bin/claude`. Put `~/.ccp/bin` first on PATH
once (ccp prints the line, for example mise `[env] _.path = ["~/.ccp/bin"]`). Plain `claude` then
runs `ccp run --profile-rule`:

- a session already on an account keeps it (sub-agents and nested `claude` stay on it);
- no rule: claude runs exactly as without the shim;
- no usable account, or a rule that no longer holds: a line on stderr, then claude runs as without
  the shim (never on another login).

`ccp use <p> --accounts [--pool <pool>]` is deprecated: it still writes a project shim
`.ccp/bin/claude`, but the profile's account rule replaces it.

### A new profile for a new client or new accounts

```bash
ccp profile create acme --from companyA   # same skills and settings, its own accounts
ccp account add acme1 acme && ccp run --account acme1    # /login
ccp account add acme2 acme && ccp run --account acme2    # /login
ccp account use auto --profile acme
cd ~/workspace/clients/acme && ccp use acme
```

Accounts you buy for yourself: link them to the profile you use them with (for example
`default`), or to a profile of their own. Seats used from several profiles: one pool
(`ccp account pool <name> <pool>`) and `--account auto:<pool>`.

### Choose the account by hand

```bash
ccp run --account ca2                         # one run on ca2 (its own profile)
ccp run companyA --account auto -- claude -c # one run, auto pick, with claude flags after --
ccp run companyA --account auto:company      # one run, the pool's freest (relinked if free)
eval "$(ccp env --account ca2)"               # this terminal only
ccp session --account ca2                     # a sub-shell on ca2
ccp env companyA --account ca2 --format=mise # pin a folder to one account
```

Everything after `--` is the command. When the first word after `--` starts with `-`, the words are arguments for the default command (the account's harness, else `claude`).

### How auto picks

1. Usage comes from Claude's usage endpoint (or `agy -p /usage` for `agy` accounts), cached 5 minutes
   in `<account>/.ccp-usage.json` (failures are cached too). ccp never refreshes a token.
2. Order: accounts under 80% of the 5-hour and 95% of the weekly limit, lowest 5-hour use first
   then lowest weekly; then accounts with unknown usage; then busy ones; then exhausted ones.
   A window whose reset time has passed counts as 0%.
3. Ties go to the account started least recently (`.ccp-last-used`), so with no usage data
   the pick rotates.
4. The pick happens once, at start. A running session never switches; when its account runs
   out, exit and start `claude` again.

### Move an account to another profile

Close its Claude sessions, then:

```bash
ccp run acme --account ca3 --relink   # ca3 is now one of acme's accounts (same login)
```

### Remove an account

There is no remove command yet. Confirm with the user first, close its sessions, then delete
its login and its folder:

```bash
# Claude account:
d=$HOME/.ccp/accounts/ca4
security delete-generic-password -s "Claude Code-credentials-$(printf %s "$d" | shasum -a 256 | cut -c1-8)"  # macOS only
rm -rf "$d"

# agy account:
rm -rf "$HOME/.ccp/accounts/agy1" "$HOME/.gemini/antigravity-cli-ccp-agy1"
```

### Move existing logins into accounts

`ccp migrate --accounts [--dry-run]` moves each profile's current login into the account of the
same name (profile `default` → account `default`). It acts on **every** profile, including the
personal `default`, and refuses a profile while Claude Code runs on its login. After the global
login moves, plain `claude` is logged out. Prefer `ccp account add` plus a fresh `/login`.

### Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Every account shows the same email | The browser approved each `/login` with the same claude.ai account. Sign out of claude.ai (or use Incognito), then `/logout` and `/login` in that account |
| Logged in, but `ccp account` says "(not logged in)" | The session ran in another folder. Check `claude auth status`: `configDirectory` must be the account folder. A hand-written wrapper that calls the mise shim causes this; use `ccp account use` and its shim |
| `none of these accounts is logged in` | Log in one account (`ccp run --account <name>`, `/login`), or start through the `ccp account use` shim, which falls back to running claude unchanged |
| `every account of pool … is linked to another profile and …` | Each pooled seat runs a Claude session on another profile (or sessions cannot be checked). Close one, or use `--or-profile` / the shim, which falls back to the profile |
| `Warning: mcp-needs-auth-cache.json is a real file …` | Claude Code replaced that link with a file. Harmless |
| An MCP server is gone after switching account | `claude mcp add` writes only the account in use. Put shared servers in `~/.ccp/profiles/<profile>/mcp.json` (`ccp doctor --fix` creates it from the profile's servers); `ccp run` passes it to every account. `ccp run` and `ccp doctor` list servers that exist on one account only |
| The status line shows another account's usage | The status line tool caches usage globally; trust `ccp account` |

## Project-only skills (linked)

Use this when skills belong to one project and should stay current, but must not go into the
synced `~/.ccp/ccp.toml` (a private team repo) or into the project's git.

```bash
ccp project install https://github.com/<org>/<repo> skills/a skills/b --link   # in the project
ccp project list            # "(dir, link: <org>/<repo>)" per linked item
ccp project update          # pull the project's sources; links follow
ccp project install         # no args: recreate the links (new machine, deleted link)
ccp project remove skills/a # drops the link only; the clone stays
```

- Links are absolute symlinks into `~/.ccp/sources/<id>`; the source and items go in
  `<project>/ccp.toml`, the commit in `<project>/ccp.lock`. ccp adds both files and each link to
  `.git/info/exclude`.
- Prefer the full URL: the `owner/repo` form looks the repo up in skills.sh and GitHub first.
- A real folder in the way (one the team's own sync wrote) is never replaced: ccp reports it.
- When `~/.ccp/ccp.toml` also lists the source, its lock wins and `ccp source update` updates it.
- `ccp source remove` refuses a clone that a project still links (listed in `~/.ccp/state/project-links.toml`) and names the projects: run `ccp project remove` there first.
- A `ccp.toml` already in the project root that ccp did not write (no marker first line) is left alone; `--link` refuses until it is moved.
- Profile-scoped instead: `ccp install <source> <items> --link <profile>` (recorded in `~/.ccp/ccp.toml`).

## Tips

- `ccp install` (no args) syncs all sources in `ccp.toml` at the commits `ccp.lock` pins — useful for new machines
- Installed items are shadows (symlinks into `~/.ccp/sources/`), so `ccp source update` updates them in place. To edit one, use `--clone` first; `source update` refuses a clone with local edits
- `ccp project add` copies (not symlinks) so projects are git-committable
- `ccp project install <source> <items> --link` links instead, for this machine only: the source goes in the project's own `ccp.toml`/`ccp.lock` (not `~/.ccp/ccp.toml`), both files and the links go in `.git/info/exclude`; `ccp project install` (no args) recreates the links, `ccp project update` updates them
- `ccp doctor` diagnoses broken hub links, manifests, hub structure, and (macOS) orphan Claude Keychain logins; `--fix` removes links whose hub item is gone
- `ccp status` shows the active profile, hub counts and per-profile health
