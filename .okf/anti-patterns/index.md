# Anti-patterns

Approaches that were tried and removed. Do not re-introduce them.

* [Composition layers](composition-layers.md) - Indirection between profiles and hub items (engines, contexts, resolvers) — profiles reference hub items directly.
* [Per-key setting fragments](per-key-setting-fragments.md) - Splitting settings.json into one YAML file per key — use complete settings templates.
* [CLAUDE.md @import parsing](claude-md-import-parsing.md) - Parsing CLAUDE.md for @path imports and creating dual symlinks — users manage their own imports.
* [Processor interface chains](processor-interface-chains.md) - Interfaces like TemplateProcessor or SettingsBuilder for what is load JSON, add hooks, write file — use one function.
* [Configurable data sharing](configurable-data-sharing.md) - Letting users choose shared vs isolated per data directory type — all data is shared.
* [Force-updating git tags](force-updating-git-tags.md) - Moving an existing tag with git tag -f — always increment the version instead.
* [Over-modularized migrations](over-modularized-migrations.md) - A separate file for every migration type — keep migration code minimal and delete completed migrations.
* [Predicting another program's loader on the path that deletes](predicting-another-programs-loader.md) - Re-implementing another harness's loader and acting destructively on the guess — predictions may only report.
