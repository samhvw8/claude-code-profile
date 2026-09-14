# Architecture

* [Package layout](package-layout.md) - How ccp's Go code is split between a thin Cobra command layer and domain packages under internal/.
* [The ~/.ccp directory layout](directory-layout.md) - What lives under ~/.ccp — hub, store, sources, profiles, shared data — and how each class of data is shared.
* [Key types and functions](key-types.md) - The Go types and entry points most changes touch — Paths, Manifest, Source, CcpConfig, Template, and settings generation.
* [Profile activation](activation.md) - How a profile becomes active — per project through env files, globally through the ~/.claude symlink, or per command.
