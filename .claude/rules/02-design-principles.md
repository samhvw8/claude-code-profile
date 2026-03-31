# Design Principles

## Simplicity Over Abstraction

Always prefer reusing existing types/patterns over creating new ones. When facing "new type vs reuse existing", reuse wins by default. Only propose new abstractions if reuse is genuinely impossible.

> "Combine and reduce complex for easy to extend and maintain"

## Complexity Budget

- Max 5 user-facing concepts. Adding a 6th requires removing one.
- No processor/builder interfaces for single-path logic. Use plain functions.
- If a feature can be a flag on an existing command, don't create a new command.
- Hidden commands (`Hidden: true`) are acceptable for power users.

## Go Patterns

- Standard Go formatting (gofmt)
- Errors returned, not panicked
- Table-driven tests with `t.TempDir()` for filesystem tests
- Platform-specific code via build tags (`//go:build !windows`)
- Relative symlinks for cross-machine portability

## Settings Generation

One function, not a pipeline:

```go
func GenerateSettings(manifest *Manifest, paths *config.Paths, profileDir string) (map[string]interface{}, error)
```

Loads template → overlays hooks → returns settings map. No interfaces needed.

## Data Sharing

All data directories (tasks, todos, history, etc.) are always symlinked to `~/.ccp/profiles/shared/`. No per-type configuration.
