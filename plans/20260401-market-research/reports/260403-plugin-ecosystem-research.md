# Research Report: Claude Code Plugin/Extension Ecosystem

**Date**: 2026-04-03
**Sources consulted**: 12+ (official docs, GitHub repos, skills.sh, community repos)
**Key search terms**: claude code plugin format, .claude-plugin, marketplace.json, skills.sh, plugin ecosystem patterns

---

## Executive Summary

Claude Code has a mature, officially specified plugin system built around a `.claude-plugin/plugin.json` manifest. Plugins are self-contained directories containing skills, agents, commands, hooks, MCP servers, and LSP servers. Distribution uses a marketplace model: a repo with `.claude-plugin/marketplace.json` lists plugins with their sources (relative paths, GitHub repos, git URLs, npm packages, or git subdirectories).

The ecosystem is clearly "clone whole repo" for the marketplace level, but decomposed at the plugin level -- a marketplace repo contains or references multiple individual plugins. This is the same pattern as Homebrew taps (repo = collection of formulae) and Oh My Zsh (monorepo of plugins).

**Key finding for ccp**: ccp's current source system (clone repo, pick items) aligns well with how Claude Code's official plugin system works. The official system clones a marketplace repo, then installs individual plugins from it. ccp clones a source repo, then installs individual items from it. The structural parallel is strong.

---

## 1. Claude Code Official Plugin Format

### Directory Structure

```
my-plugin/
+-- .claude-plugin/
|   +-- plugin.json          # REQUIRED: name, description, version
+-- commands/                # Slash commands (.md files)
+-- agents/                  # Agent definitions (.md files with YAML frontmatter)
+-- skills/                  # Agent Skills (folders with SKILL.md)
+-- hooks/
|   +-- hooks.json           # Same format as settings.json hooks
+-- .mcp.json                # MCP server config
+-- .lsp.json                # LSP server config
+-- bin/                     # Executables added to PATH
+-- settings.json            # Default settings (currently only "agent" key)
+-- README.md
```

**Critical rule**: commands/, agents/, skills/, hooks/ go at plugin root. Only plugin.json goes inside `.claude-plugin/`.

### plugin.json Schema

```json
{
  "name": "my-plugin",           // kebab-case, becomes skill namespace prefix
  "description": "...",
  "version": "1.0.0",           // semver
  "author": { "name": "...", "email": "..." },
  "homepage": "...",
  "repository": "...",
  "license": "MIT",
  "keywords": ["..."],
  // Optional custom component paths:
  "commands": "./custom-commands",
  "agents": ["./agents", "./specialized-agents"],
  "hooks": "./config/hooks.json",
  "mcpServers": { ... },
  "lspServers": { ... }
}
```

### SKILL.md Format

```yaml
---
name: code-review
description: Reviews code for best practices. Use when reviewing code or PRs.
disable-model-invocation: true   # optional
---

When reviewing code, check for:
1. Code organization
2. Error handling
3. Security concerns
```

Skills are namespaced: `/plugin-name:skill-name`. `$ARGUMENTS` captures user input.

### hooks.json Format

Identical to settings.json hooks format:

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Write|Edit",
      "hooks": [{ "type": "command", "command": "..." }]
    }]
  }
}
```

`${CLAUDE_PLUGIN_ROOT}` references install directory. `${CLAUDE_PLUGIN_DATA}` for persistent state.

### Plugin Installation

Plugins are **copied** to `~/.claude/plugins/cache` -- not symlinked, not referenced in-place. Cannot reference `../` paths outside plugin dir. Symlinks inside plugin dir are followed during copy.

---

## 2. Marketplace Format

### marketplace.json Schema

Lives at `.claude-plugin/marketplace.json` in repo root.

```json
{
  "name": "company-tools",        // kebab-case, used in install commands
  "owner": { "name": "...", "email": "..." },
  "metadata": {
    "description": "...",
    "version": "...",
    "pluginRoot": "./plugins"     // base dir for relative paths
  },
  "plugins": [
    {
      "name": "code-formatter",
      "source": "./plugins/formatter",           // relative path
      "description": "...",
      "version": "2.1.0",
      "strict": true                             // default
    },
    {
      "name": "deploy-tools",
      "source": { "source": "github", "repo": "company/deploy-plugin" }
    }
  ]
}
```

### Plugin Source Types

| Source | Format | Use case |
|--------|--------|----------|
| Relative path | `"./plugins/foo"` | Plugin lives in same repo |
| GitHub | `{ "source": "github", "repo": "owner/repo", "ref": "v2.0", "sha": "abc..." }` | External GitHub repo |
| Git URL | `{ "source": "url", "url": "https://..." }` | Any git host |
| Git subdirectory | `{ "source": "git-subdir", "url": "...", "path": "tools/plugin" }` | Plugin inside monorepo |
| npm | `{ "source": "npm", "package": "@org/plugin", "version": "^2.0" }` | npm distribution |

### Strict Mode

- `strict: true` (default): plugin.json is authority, marketplace supplements
- `strict: false`: marketplace entry is entire definition, plugin needs no plugin.json

### Installation Commands

```bash
/plugin marketplace add owner/repo          # Add marketplace
/plugin marketplace add owner/repo@v2.0     # Pin to ref
/plugin install plugin-name@marketplace     # Install specific plugin
/plugin marketplace update                  # Refresh all
```

### Official Marketplace

- Repo: `anthropics/claude-plugins-official`
- Auto-available on Claude Code start
- Browsable at claude.com/plugins
- Submit via claude.ai/settings/plugins/submit

---

## 3. Real-World Repository Patterns

### Pattern A: Marketplace Monorepo (plugins co-located)

**anthropics/claude-code/plugins/** -- 13 official demo plugins:

```
plugins/
+-- .claude-plugin/marketplace.json
+-- agent-sdk-dev/
+-- code-review/
+-- commit-commands/
+-- feature-dev/
+-- frontend-design/
+-- hookify/
+-- plugin-dev/
+-- pr-review-toolkit/
+-- security-guidance/
+-- ...
```

Each plugin is self-contained subdirectory with `.claude-plugin/plugin.json`, commands/, agents/, skills/, hooks/.

**anthropics/claude-plugins-official** -- same pattern:
```
+-- .claude-plugin/marketplace.json
+-- plugins/           # Anthropic-maintained
+-- external_plugins/  # Community contributions
```

### Pattern B: Marketplace Aggregator (plugins in external repos)

**2389-research/claude-plugins** -- 35+ plugins:

```
+-- .claude-plugin/marketplace.json    # References external repos
+-- docs/
+-- scripts/
+-- tests/
```

marketplace.json entries reference plugins in separate repos under the 2389-research org. Install: `/plugin install css-development@2389-research`.

### Pattern C: Skills-Only Repo (no plugin wrapper)

**anthropics/skills** -- Reference skill collection:

```
+-- .claude-plugin/           # Makes it installable as plugin
+-- skills/
|   +-- creative-skills/
|   +-- dev-skills/
|   +-- document-skills/
|   +-- ...
+-- spec/                     # Agent Skills specification
+-- template/                 # Skill template
```

Each skill = folder with SKILL.md. YAML frontmatter (name, description) + markdown instructions.

### Pattern D: Community Monorepos

- `jeffallan/claude-skills` -- 65 specialized skills in one repo
- `obra/superpowers` -- SDLC competency bundle
- `trailofbits/skills` -- Security audit skills
- `K-Dense-AI/claude-scientific-skills` -- Research skills

Common: single repo, many skills organized by category. Some wrap as plugins, some are raw skill folders.

### Pattern E: Single-Purpose Repos

- `zarazhangrui/codebase-to-course` -- one focused skill
- `alonw0/web-asset-generator` -- one focused skill
- `jawwadfirdousi/agent-skills` -- PostgreSQL queries

---

## 4. skills.sh

**What it is**: "The Open Agent Skills Ecosystem" -- a cross-platform skill marketplace/directory.

**Key facts**:
- Supports 20+ agents: Claude Code, Cursor, Cline, Gemini, Codex CLI, GitHub Copilot, VSCode
- Install: `npx skillsadd <owner/repo>`
- 91K+ total installs across skills
- Leaderboard: All Time, Trending (24h), Hot
- Indexes by: source repo (owner/repo), skill ID, install count, name

**Metadata**: Minimal -- repo reference, skill name, install count. Not a rich manifest format. Essentially a directory/index pointing at GitHub repos.

**Relation to Claude Code**: Listed as one of many supported agents. Skills.sh is agent-neutral -- it indexes SKILL.md-based skills that work across multiple AI coding tools.

**Not a backend**: It's a discovery layer / leaderboard, not a package registry. Skills live in GitHub repos; skills.sh indexes them.

---

## 5. Comparable Ecosystems Analysis

### Oh My Zsh -- Monorepo Model

- Bundled: 300+ plugins live in `plugins/` dir of main repo
- External: third-party plugins are separate repos, loaded via plugin managers (zsh-users/zsh-autosuggestions, etc.)
- Each plugin = single directory with `*.plugin.zsh` file
- **Pattern**: Clone main repo = get everything. External plugins = clone individual repos.

### Homebrew Taps -- Repo-per-Collection

- Each "tap" = one git repo containing multiple formulae
- `brew tap owner/repo` clones the repo
- `brew install owner/repo/formula` installs from that tap
- Formulae are Ruby files in the repo
- **Pattern**: Clone repo (tap) as collection, install individual items (formulae) from it.

### Neovim (lazy.nvim) -- Clone Individual Repos

- Each plugin = one GitHub repo
- lazy.nvim manages: clone, lazy-load, update, pin
- Uses partial clones (not shallow) for efficiency
- Plugin spec: Lua table with repo URL + config
- **Pattern**: Every plugin is its own repo. Manager clones each independently.

### VSCode Extensions -- Package Registry

- Each extension = vsix package (zip with manifest)
- Published to marketplace.visualstudio.com
- manifest: package.json with contributes section
- **Pattern**: Central registry, individual packages. No repo-cloning -- packaged artifacts.

### asdf/mise -- Repo-per-Plugin

- Each plugin = one GitHub repo with scripts (bin/install, bin/list-all, etc.)
- Plugin registers tool management logic, not the tool itself
- `asdf plugin add nodejs` clones the nodejs plugin repo
- **Pattern**: One repo per plugin. Each cloned independently.

---

## 6. The Key Question: Clone Whole Repo vs Decompose

### Taxonomy

| Ecosystem | Unit of Distribution | Cloning Model | Decomposition |
|-----------|---------------------|---------------|---------------|
| Claude Code Marketplace | Marketplace repo | Clone repo, install individual plugins | Yes -- repo is collection, items are individual |
| Oh My Zsh (bundled) | Main repo | Clone once, enable individual plugins | Yes -- monorepo with selectable items |
| Oh My Zsh (external) | Plugin repo | Clone per plugin | No -- 1 repo = 1 plugin |
| Homebrew Taps | Tap repo | Clone per tap | Yes -- tap has many formulae |
| Neovim lazy.nvim | Plugin repo | Clone per plugin | No -- 1 repo = 1 plugin |
| VSCode | Extension package | Download package | No -- 1 package = 1 extension |
| asdf/mise | Plugin repo | Clone per plugin | No -- 1 repo = 1 plugin |
| skills.sh | Skill repo | Reference to repo | Varies -- repo may have 1 or many skills |

### Answer

**Both patterns exist and are standard.** The choice depends on the ecosystem's unit of trust and update:

**Clone-whole-repo-as-one-unit** (asdf, lazy.nvim, VSCode):
- When each repo IS a single logical unit
- Update granularity = entire repo
- Simple mental model: 1 repo = 1 thing

**Clone-repo-as-collection, decompose into items** (Homebrew taps, Claude Code marketplaces, Oh My Zsh):
- When a repo is a curated collection
- Users pick which items to activate
- Common for curated/organizational collections

**Claude Code specifically uses the collection model**: marketplace repo -> individual plugin selection. This maps directly to ccp's source model: source repo -> individual item selection.

### Implication for ccp

ccp's `source add owner/repo` + `install owner/repo skills/foo` pattern mirrors exactly how Claude Code marketplaces work:
1. Register collection (marketplace add / source add)
2. Browse items (plugin discover / find)
3. Install individual items (plugin install / install)

The key difference: Claude Code's official system expects `.claude-plugin/plugin.json` per installable unit. ccp's hub items (skills, agents, hooks) are lighter-weight -- individual folders, not full plugins. This is analogous to skills.sh's model (individual SKILL.md folders) vs the official plugin model (full plugin directories).

---

## Unresolved Questions

1. **skills.sh API**: No public API documentation found. Does it have a REST API that ccp currently uses, or is it scraped? How does `ccp find` actually query skills.sh?

2. **Plugin vs Skill granularity**: Claude Code's plugin system bundles skills+agents+hooks+commands into a single installable unit. ccp decomposes these into individual hub items. Is there user demand to install full plugins as atomic units rather than individual skills/agents?

3. **marketplace.json adoption**: How many community repos actually ship marketplace.json? The pattern seems early -- most community repos are raw skill collections without plugin.json wrappers.

4. **SKILL.md as universal format**: The Agent Skills spec (from anthropics/skills) appears to be the cross-agent standard. Is ccp's hub skill format compatible with SKILL.md, or is there a translation step?

5. **Plugin caching behavior**: Claude Code copies plugins to `~/.claude/plugins/cache`. How does this interact with ccp's symlink-based profile system? Can ccp-managed profiles reference cached plugins?

---

## Sources

- [Create plugins - Claude Code Docs](https://code.claude.com/docs/en/plugins)
- [Create and distribute a plugin marketplace - Claude Code Docs](https://code.claude.com/docs/en/plugin-marketplaces)
- [Discover and install plugins - Claude Code Docs](https://code.claude.com/docs/en/discover-plugins)
- [anthropics/claude-plugins-official](https://github.com/anthropics/claude-plugins-official)
- [anthropics/claude-code/plugins](https://github.com/anthropics/claude-code/tree/main/plugins)
- [anthropics/skills](https://github.com/anthropics/skills)
- [2389-research/claude-plugins](https://github.com/2389-research/claude-plugins)
- [hesreallyhim/awesome-claude-code](https://github.com/hesreallyhim/awesome-claude-code)
- [skills.sh](https://skills.sh)
- [Plugin Architecture (DeepWiki)](https://deepwiki.com/anthropics/claude-code/4.1-plugin-marketplace-and-discovery)
