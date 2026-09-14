# Decisions

Settled choices with their rationale. Do not re-open one without a user request; supersede it with a new decision instead.

# Architecture and product

* [v0.28 simplification: flat profiles](v0-28-simplification.md) - Profiles are flat — no engine/context composition layers; the hub is the sharing mechanism.
* [API keys are per-profile](api-keys-per-profile.md) - API keys live in each profile; there is no shared account or credentials layer.
* [Complete settings templates, not fragments](complete-settings-templates.md) - A settings template is a whole settings.json; hooks are overlaid from hub hooks.
* [All data directories are always shared](data-sharing-always-shared.md) - Runtime data directories are always symlinked to profiles/shared, with no per-type configuration.
* [Hub remove offers copy-to-profile](hub-remove-copy-to-profile.md) - Removing a hub item used by profiles offers copy / delete / cancel instead of a destructive yes/no.
* [Bundles are atomic composite hub items](bundles-atomic-composite.md) - A bundles hub item type groups coupled items that link and remove as one unit, without becoming a sixth concept.
* [Command surface: about 18 visible commands](command-surface.md) - Keep roughly 18 visible commands and hide power-user commands.

# omp integration (oldest first)

* [Brainstorm outcome: link only what omp loads](omp-link-only-what-omp-loads.md) - The first omp design — a single loadability predicate gating links, additive follow, and the refused alternatives.
* [Adversarial review round](omp-adversarial-review-round.md) - The first external review's High findings were fixed; state-file ownership and a linked-plugin migration were declined.
* [Round 3: rules travel in RULES.md](omp-rules-travel-in-rules-md.md) - Rules are never linked into omp; RULES.md carries all of them, and loadability predictions only produce notes.
