# Decisions

Settled choices with their rationale. Do not re-open one without a user request; supersede it with a new decision instead.

# Architecture and product

* [Account pools: auto:<pool> picks across profiles](account-pools.md) - 2026-11-23: An account names at most one pool; --account auto:<pool> picks across profiles, ccp run relinks a free Claude account (process scan plus flock), env and session never relink.
* [Shared MCP servers: a profile mcp.json passed with --mcp-config](profile-mcp-file.md) - 2026-11-23: profiles/<name>/mcp.json reaches every account through ccp run's --mcp-config=<file>; ccp never writes .claude.json and only reports servers added on one account.
* [Account is the sixth concept; the budget is seven for now](account-concept.md) - 2026-11-18: The concept budget rises from five to seven and Account becomes a concept with a visible ccp account command (list, add, and since 2026-11-23 pool); the --account mechanism is unchanged.
* [Accounts are an option of Activation](accounts-as-activation-option.md) - 2026-11-17: One profile runs under several Claude logins through account folders chosen with --account, or --account auto for the one with the most plan limit left; supersedes the per-profile API key decision.
* [v0.28 simplification: flat profiles](v0-28-simplification.md) - Profiles are flat — no engine/context composition layers; the hub is the sharing mechanism.
* [API keys are per-profile](api-keys-per-profile.md) - Deprecated, superseded by accounts as an Activation option. API keys live in each profile; there is no shared account or credentials layer.
* [Complete settings templates, not fragments](complete-settings-templates.md) - A settings template is a whole settings.json; hooks are overlaid from hub hooks.
* [A machine-local snapshot tells settings edits from input changes](settings-snapshot.md) - ccp records what it last wrote to settings.json under ~/.ccp/state, so regeneration keeps edits but applies template and fragment changes.
* [All data directories are always shared](data-sharing-always-shared.md) - Runtime data directories are always symlinked to profiles/shared, with no per-type configuration.
* [Hub remove offers copy-to-profile](hub-remove-copy-to-profile.md) - Removing a hub item used by profiles offers copy / delete / cancel instead of a destructive yes/no.
* [Bundles are atomic composite hub items](bundles-atomic-composite.md) - A bundles hub item type groups coupled items that link and remove as one unit, without becoming a sixth concept.
* [Command surface: about 18 visible commands](command-surface.md) - Keep roughly 18 visible commands and hide power-user commands.
* [Source items are shadows, not copies](shadow-source-items.md) - A hub item installed from a source is a symlink into the source clone; ccp.lock pins each source's commit; --clone is the only way to get an editable copy.

# Other harnesses

* [Custom harness plugins use TOML manifests or JSON exec plugins](harness-plugins-toml-and-exec.md) - 2026-11-22: Hybrid non-JS harness extensibility via `~/.ccp/harnesses/<id>.toml` or `ccp-harness-<id>` executables producing a validated `HarnessPlan` applied by ccp's shared copy/link/style ledgers.
* [agy accounts use file token mode](agy-accounts-file-token-mode.md) - 2026-11-20: Account bindings gain harness = "agy"; each agy account runs with --app_data_dir and SSH_CLIENT, merged settings.json, and /usage quota reads.
* [agy: converted copies, skill links, GEMINI.md](agy-copies-links-and-gemini-md.md) - Why ccp agy sync converts agents and rules, links skills and CLAUDE.md, writes the output style to GEMINI.md, and mirrors no hooks or commands.

# omp integration (oldest first)

* [Brainstorm outcome: link only what omp loads](omp-link-only-what-omp-loads.md) - The first omp design — a single loadability predicate gating links, additive follow, and the refused alternatives.
* [Adversarial review round](omp-adversarial-review-round.md) - The first external review's High findings were fixed; state-file ownership and a linked-plugin migration were declined.
* [Round 3: rules travel in RULES.md](omp-rules-travel-in-rules-md.md) - Rules are never linked into omp; RULES.md carries all of them, and loadability predictions only produce notes.
