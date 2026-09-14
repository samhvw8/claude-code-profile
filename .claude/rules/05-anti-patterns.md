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

## Predicting Another Program's Loader On The Path That Deletes

Do not answer "does the other harness load this?" by re-implementing that harness's loader, and then *act* on the guess — refuse a link, remove a link, or omit content from the only file ccp controls. A wrong prediction then removes configuration silently: the link looks healthy, the item is never read, no error is raised, and nothing reports it. Rules were the sharp case, because `RULES.md` was the only other carrier and the guess decided whether a rule appeared in it.

What replaced it: an item omp is predicted to ignore is still linked and reported (a link nobody reads costs nothing, and a later omp may read it); only a link the profile does not want is removed; and for the one item type ccp is solely responsible for, ccp generates the carrier itself instead of predicting. A prediction may stay only as a *reporter*, where a wrong answer costs a line of output.
