# Anti-Patterns

Things that have been tried and removed. Do not re-introduce.

## Composition Layers

Adding indirection between profiles and hub items (engines, contexts, resolvers). Profiles reference hub items directly. Period.

## Per-Key Setting Fragments

Splitting settings.json into individual YAML files per key. Use complete settings templates instead.

## CLAUDE.md @Import Parsing

Parsing CLAUDE.md for `@path` references and creating dual symlinks. Too much complexity for a niche feature. Users manage their own @imports.

## Processor Interface Chains

Creating interfaces (TemplateProcessor, FragmentProcessor, HookProcessor, SettingsBuilder) for what is fundamentally "load JSON, add hooks, write file." Use a single function.

## Configurable Data Sharing

Letting users choose shared vs isolated per data directory type. All data is shared. No configuration needed.

## Force-Updating Git Tags

Never `git tag -f`. Always increment version. Force-updating prevents GitHub CI from re-running.

## Over-Modularized Migrations

Creating a separate file for every migration type. Keep migration code minimal — delete completed migrations after a few versions.
