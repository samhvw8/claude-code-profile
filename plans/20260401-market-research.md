# CCP Market Research Report

**Date**: 2026-03-27
**Subject**: Claude Code Profile/Configuration Management Landscape

---

## 1. Claude Code Ecosystem Size

### Core Repository
| Metric | Value | Source |
|--------|-------|--------|
| GitHub stars | **97,461** | [anthropics/claude-code](https://github.com/anthropics/claude-code) |
| Forks | **14,679** | GitHub API |
| Open issues | **8,571** | GitHub API |
| Created | 2025-02-22 | GitHub API |
| Watchers | 623 | GitHub API |

### Official Skills Repository
| Metric | Value |
|--------|-------|
| Stars | **108,209** |
| Forks | **12,038** |
| Repo | [anthropics/skills](https://github.com/anthropics/skills) |

### Ecosystem Breadth
- **14,691 repos** tagged with "claude-code" on GitHub (across Python 3,339 / TypeScript 2,821 / Shell 2,066 / JavaScript 1,285 / Go 605 / Rust 489)
- **awesome-claude-code** (hesreallyhim): 35,381 stars, 2,673 forks
- Described as "the fastest-growing repo in GitHub history" after source code leak (March 2026)

### Skills Marketplaces
| Platform | Claimed Skills | Notes |
|----------|---------------|-------|
| [SkillsMP](https://skillsmp.com/) | 700,000+ | Aggregates from GitHub; inflated count likely includes forks/variants |
| [SkillHub](https://www.skillhub.club) | 7,000+ | AI-evaluated, multi-agent compatible |
| [Claude Marketplaces](https://claudemarketplaces.com/) | 2,300+ | Single-command install |
| [Skills Directory](https://www.skillsdirectory.com/) | Unknown | Verified/secure focus |
| [claude-skill-registry](https://github.com/majiayu000/claude-skill-registry) | Unknown | Searchable index, daily updated |
| alirezarezvani/claude-skills | 220+ | Curated, multi-agent |

### Revenue & Adoption
- Claude Code run-rate revenue: ~**$2.5B** as of Feb 2026
- 73% of engineering teams use AI coding tools daily (survey of 15,000 devs)
- Claude 5 is top choice for complex tasks at 44%; Claude Code second for autocomplete at 31%
- Claude Code overtook Copilot and Cursor as most-used AI coding tool in 8 months
- 75% adoption in small companies; enterprises lag (56% Copilot, Claude Code behind)
- 300,000+ business customers for Anthropic overall

---

## 2. Pain Points & Configuration Issues

### Skill Limits (CRITICAL for ccp)

**Issue #13343**: [Skills truncated at 30 makes remaining skills undiscoverable](https://github.com/anthropics/claude-code/issues/13343)
- `<available_skills>` shows "Showing 30 of 39 skills due to token limits"
- Truncated skills are invisible to Claude -- they effectively don't exist
- Undocumented ~16,000 char budget for skill metadata; ~42 skills max at 263 chars avg
- Truncation algorithm punishes well-documented skills (verbose descriptions get cut first)
- State: **Closed** (5 reactions, 3 comments) -- but downstream issues persist

**Issue #10238**: [Add support for subdirectories in skills](https://github.com/anthropics/claude-code/issues/10238) -- **123 reactions**, open

**Issue #18192**: [Recursive skill discovery](https://github.com/anthropics/claude-code/issues/18192) -- **37 reactions**, open

**Issue #14920**: [Disable individual plugin skills](https://github.com/anthropics/claude-code/issues/14920) -- **36 reactions**, open

**Issue #9716**: [Claude not aware of available skills](https://github.com/anthropics/claude-code/issues/9716) -- **66 reactions**, open

### Multi-Account / Profile Switching

**Issue #18435**: [Multi-account switching in Desktop app](https://github.com/anthropics/claude-code/issues/18435) -- **280 reactions**, 40 comments, OPEN
- "Sign out, re-enter credentials, lose context every time I switch accounts"
- "Interrupts workflow multiple times per day"

**Issue #27359**: [Named account profiles for quick switching](https://github.com/anthropics/claude-code/issues/27359) -- **23 reactions**, open

**Issue #30388**: [Session profiles / presets](https://github.com/anthropics/claude-code/issues/30388) -- closed, 0 reactions
- Requested: named presets for model + permissions + MCP tools
- Use cases: research (Opus, read-only, web MCPs), coding (Sonnet, full perms), security review (Opus, restricted)

### MCP Configuration Problems

**Issue #24000**: [MCP Profiles for Rapid Context-Aware Tool Switching](https://github.com/anthropics/claude-code/issues/24000)
- MCP config is all-or-nothing; no scoping per project/task
- Unused MCPs waste 10-15% of context budget
- `disallowedTools` doesn't work for MCP servers
- Devs manually edit `.claude.json` between sessions

### Skills/Plugin Integration Issues

**Issue #31005**: [Support for AGENTS.md](https://github.com/anthropics/claude-code/issues/31005) -- **110 reactions**
**Issue #18949**: [Skills from plugins don't appear in autocomplete](https://github.com/anthropics/claude-code/issues/18949) -- **70 reactions**
**Issue #18950**: [Skills/subagents don't inherit permissions](https://github.com/anthropics/claude-code/issues/18950) -- **40 reactions**
**Issue #20697**: [Sync skills between Desktop and CLI](https://github.com/anthropics/claude-code/issues/20697) -- **28 reactions**

### Context & Configuration Complexity
- CLAUDE.md instruction limit: ~150-200 effective instructions; system prompt consumes ~50 slots before CLAUDE.md loads
- Each low-value rule reduces compliance with high-value ones
- Context window fills fast; performance degrades
- Prompt caching issues in March 2026 caused abnormal token drain

---

## 3. Competitor / Related Tools

### Direct Competitors (Profile/Config Management)

| Tool | Stars | Forks | Lang | Created | Focus |
|------|-------|-------|------|---------|-------|
| [CCS (kaitranntt)](https://github.com/kaitranntt/ccs) | **1,729** | 139 | TS | 2025-11 | Multi-account switching, multi-provider (300+ models), OAuth proxy, visual dashboard |
| [claude-code-switch (foreveryh)](https://github.com/foreveryh/claude-code-switch) | **520** | 69 | Shell | 2025-09 | One-command model/provider switcher (Anthropic API only) |
| [ClaudeCTX (foxj77)](https://github.com/foxj77/claudectx) | **74** | 4 | Go | 2025-12 | Full config profile switching, atomic swaps, auto-rollback |
| [claude-swap (realiti4)](https://github.com/realiti4/claude-swap) | **63** | 11 | Python | 2026-01 | Multi-account switching |
| [clausona (larcane97)](https://github.com/larcane97/clausona) | **20** | 2 | TS | 2026-03 | Profile management, symlinked shared env |
| [claude-code-profiles (pegasusheavy)](https://github.com/pegasusheavy/claude-code-profiles) | **9** | 2 | PS | 2026-02 | Work/personal account isolation |
| [claudectx (FGRibreau)](https://github.com/FGRibreau/claudectx) | 5 | 0 | Rust | 2026-01 | Config + auth switching |
| [claude-switch (hoangvu12)](https://github.com/hoangvu12/claude-switch) | 4 | 0 | TS | 2026-03 | OAuth + API key profile swapping |
| [claude-code-swap (tensakulabs)](https://github.com/tensakulabs/claude-code-swap) | 0 | 0 | Rust | 2026-03 | Multi-provider profiles |
| [claude-account-switcher (ukogan)](https://github.com/ukogan/claude-account-switcher) | 0 | 0 | Shell | 2026-02 | Isolated config dirs + symlinks |

Also: **Raycast extension** (Claude Code Switcher), **claude-provider** (plugin + CLI with slash commands), **cc-switch** (cross-platform desktop tool for Claude/Codex/Gemini/OpenCode).

### Other Config Tools
- **claude-rules-doctor** (nulone): Detects dead `.claude/rules/` files (glob mismatch validation)
- **ccexp** (nyatinte): Interactive TUI for discovering config files and slash commands
- **Claude Config** (VS Code extension): Visual settings management

### What Competitors Solve vs. What ccp Solves

| Capability | CCS | claude-code-switch | ClaudeCTX | clausona | **ccp** |
|-----------|-----|-------------------|-----------|----------|---------|
| Account switching | Yes | No | Yes | Yes | Yes |
| Provider switching | Yes (300+) | Yes | No | No | No (not a goal) |
| Settings profiles | Partial | No | Yes | Partial | Yes (templates) |
| Skills/agents management | No | No | No | No | **Yes (hub)** |
| Engine/context composition | No | No | No | No | **Yes** |
| Hub (shared skill library) | No | No | No | No | **Yes** |
| Source management (install/update) | No | No | No | No | **Yes** |
| Hooks management | No | No | No | No | **Yes** |
| Shared data dirs | No | No | No | Symlinks | **Yes** |
| CLAUDE.md linked dirs | No | No | No | No | **Yes** |
| Settings templates | No | No | No | No | **Yes** |
| Drift detection | No | No | No | No | **Yes** |
| Migration tooling | No | No | No | No | **Yes** |

**Key insight**: ALL competitors focus on account/provider switching. NONE address the skill/agent organization, hub management, composable profiles, or settings template problems. ccp occupies a unique niche.

### How Other AI Tools Handle Config

- **Cursor**: `.cursorrules` per project; no profile system; model change in one instance affects all instances (known pain point)
- **Windsurf**: `.windsurf/` dir committed to source; cleaner than Cursor but no multi-profile
- **Copilot**: `AGENTS.md` open standard emerging as cross-tool format; no profile management
- **Cross-tool**: Community building symlink-based sync CLIs, chezmoi-based config generators

---

## 4. User Profiles & Needs

### Who Uses Claude Code
- **Solo devs** (majority): personal projects, freelance; need quick switching between project contexts
- **Small teams** (75% adoption): startup engineers; need shared conventions via CLAUDE.md
- **Enterprise** (growing but lags Copilot): need security baselines, compliance rules, multi-repo config distribution
- **Product managers**: emerging user class (ccforpms.com exists)

### CLAUDE_CONFIG_DIR Usage Patterns
The built-in mechanism for multi-config is shell aliases:
```bash
alias claude-work="CLAUDE_CONFIG_DIR=~/.claude-work claude"
alias claude-personal="CLAUDE_CONFIG_DIR=~/.claude-personal claude"
```
- Works but manual, error-prone, no shared resources between profiles
- Multiple blog posts and guides teach this pattern -- indicates unmet demand
- Two terminals can run simultaneously with different accounts

### Enterprise Configuration Needs
- Security baselines authored once, inherited everywhere (CI/CD syncs `.claude/rules/`)
- CLAUDE.md declares MCP server availability
- Hooks fire automatically before/after operations
- Agent teammates inherit lead's permission settings
- Multi-repo distribution via pipeline

### Power User Setup Patterns
- Feature-specific sub-agents with skills for progressive disclosure
- Research -> Plan -> Execute -> Review -> Ship workflow
- Multiple MCP servers (web search, context7, Slack, etc.)
- Custom hooks, commands, rules
- The skill truncation at 30-39 skills confirms power users accumulate many skills

---

## 5. Value Proposition Validation

### Evidence: People Want Config Switching

| Signal | Strength | Evidence |
|--------|----------|---------|
| Multi-account switching demand | **Very Strong** | #18435 has 280 reactions + 40 comments (top feature request); 10+ competing tools built |
| Session profiles demand | **Moderate** | #30388 explicitly requests named presets; MCP profiles requested multiple times |
| Tool proliferation | **Strong** | 10+ account-switching tools created in 6 months = massive unmet need |
| Blog/guide proliferation | **Strong** | Multiple Medium articles, blog posts teaching CLAUDE_CONFIG_DIR workarounds |

### Evidence: Skill Limit Is a Real Constraint

| Signal | Strength | Evidence |
|--------|----------|---------|
| Skills truncated at 30 | **Confirmed** | #13343 documented; 16K char budget empirically measured |
| Skill organization requests | **Very Strong** | #10238 (123 reactions), #18192 (37), #14920 (36), #9716 (66) |
| Skills not discoverable | **Confirmed** | Truncated skills invisible to Claude; punishes good documentation |
| Marketplace growth | **Strong** | 700K+ skills on SkillsMP; 108K stars on anthropics/skills |

### Evidence: Per-Project Configuration Is a Need

| Signal | Strength | Evidence |
|--------|----------|---------|
| MCP all-or-nothing problem | **Confirmed** | #24000 documents 10-15% context waste from unwanted MCPs |
| Different task setups | **Confirmed** | #30388 lists research/coding/security as requiring fundamentally different configs |
| Project-level settings hierarchy | **Native** | Claude Code already has user/project/local settings layers |
| CLAUDE.md per-project | **Native** | Already walks directory tree merging instructions |

### Evidence: Team Config Sharing Is Valuable

| Signal | Strength | Evidence |
|--------|----------|---------|
| Enterprise adoption growing | **Moderate** | 300K+ business customers; Team/Enterprise plans exist |
| CI/CD config distribution | **Emerging** | Pattern documented: pipeline syncs shared rules across repos |
| CLAUDE.md in version control | **Standard** | Official best practice for team standardization |
| AGENTS.md cross-tool standard | **Emerging** | #31005 (110 reactions) requesting support |

---

## 6. Market Gaps & Opportunities for ccp

### Gap 1: Skill Organization Beyond File Limits
No tool addresses the 30-skill truncation problem. ccp's hub system + engine/context composition could enable selective skill loading per task/project, keeping active skills under the limit while maintaining a large library.

### Gap 2: Composable Configuration
Every competitor does flat profile switching. None offer layered composition (engine + context + profile overrides). ccp's two-layer architecture is architecturally unique.

### Gap 3: Settings Templates
No competitor manages settings.json templates. Manual editing or full-profile switching are the only options elsewhere.

### Gap 4: Hub as Shared Skill Library
No tool provides a central hub for organizing, sharing, and linking skills/agents/hooks across profiles. Every competing tool treats profiles as monolithic config blobs.

### Gap 5: Source Management
ccp's `find`/`install`/`source` workflow for discovering and managing skill sources has no equivalent in the competitor landscape.

### Gap 6: Cross-Tool Configuration
AGENTS.md is emerging as a cross-tool standard. No profile manager currently generates configurations for multiple AI coding tools from a single source.

---

## 7. Risks & Concerns

1. **Anthropic could build this natively.** Issue #30388 (session profiles) was closed -- unclear if "wontfix" or "we'll build it ourselves." The 280-reaction #18435 suggests Anthropic knows the demand exists.

2. **Ecosystem fragmentation.** 10+ switching tools in 6 months suggests the market may commoditize the simple switching case before ccp gains traction.

3. **Complexity vs. adoption.** ccp's two-layer composition (engines + contexts) is powerful but may be over-engineered for the 80% use case (just switch my account/settings). Onboarding friction matters.

4. **Skill limit may be fixed.** If Anthropic raises the 30-skill truncation limit significantly, a core value prop weakens (though hub organization still has value for management).

5. **Discovery problem.** ccp is not listed on awesome-claude-code's config managers page (only claude-rules-doctor and ClaudeCTX are). Getting listed would provide visibility.

---

## 8. Strategic Recommendations

1. **Get listed on awesome-claude-code immediately.** 35K stars = largest discovery channel. Submit a PR to hesreallyhim/awesome-claude-code.

2. **Lead with the unique value.** Don't compete on account switching (CCS owns that at 1.7K stars). Lead with: "Manage 50+ skills across projects without hitting the 30-skill wall."

3. **Simplify onboarding.** `ccp init` should work in <30 seconds with sensible defaults. The competition's zero-config approach is attractive.

4. **Consider skills.sh / SkillsMP integration.** The marketplace ecosystem is exploding. Being a first-class client for the dominant registry provides distribution.

5. **Target power users first.** The 30-skill limit only affects users with many skills. These are also the users most likely to adopt a profile manager. Small market but high engagement.

6. **Document the "why" clearly.** Competitors' READMEs are mostly "switch accounts with one command." ccp's README should answer: "What happens when you have 40 skills, 3 projects, and need different configs for each?"

---

## Unresolved Questions

1. What is skills.sh specifically? Searches returned SkillsMP, SkillHub, etc. but "skills.sh" as a standalone registry was not found as a distinct platform -- it may be an internal/planned name or alias.
2. How many active daily users does Claude Code CLI have specifically (vs. Claude overall)?
3. Has Anthropic commented publicly on plans for native profile/preset support?
4. What is the actual distribution of skills-per-user? The 30-skill truncation issue references 39, the char-budget research references 42 max -- but no survey data on typical counts.
5. Is the awesome-claude-code "Config Managers" category growing? Only 2 tools listed currently.
