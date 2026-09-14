---
type: File Format
title: Hub hooks format
description: How hub hooks are stored (Claude Code hooks.json plus scripts), the Go types, event names, and how they reach settings.json.
resource: internal/config/hooks.go
tags: [reference, hooks, settings]
generated: { by: claude-code/claude-opus-5, at: 2026-09-14T00:50:10Z }
sources:
  - id: dev-reference
    resource: "docs/dev-reference.md (removed 2026-09-14 by the OKF migration)"
    title: Developer reference
  - id: settings-generator
    resource: internal/profile/settings_generator.go
    title: resolvePluginRootPath and hook overlay
    last_modified: 2026-09-14
---

# Layout

Hooks use the official Claude Code plugin `hooks.json` format, so a hub hook is also a valid
plugin hook directory.

```
~/.ccp/hub/hooks/<name>/
├── hooks.json         # official format
└── scripts/
    └── script.sh
```

# Schema

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          { "type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/session-start.sh", "timeout": 60 }
        ]
      }
    ]
  }
}
```

```go
// internal/config/hooks.go
type HooksJSON struct {
    Hooks map[HookType][]HookEntry `json:"hooks"`
}
type HookEntry struct {
    Matcher string        `json:"matcher,omitempty"`
    Hooks   []HookCommand `json:"hooks"`
}
type HookCommand struct {
    Type    string `json:"type"`              // "command"
    Command string `json:"command"`           // path or ${CLAUDE_PLUGIN_ROOT}/...
    Timeout int    `json:"timeout,omitempty"`
}
```

# Events

| Event | When |
|-------|------|
| `SessionStart` | Session startup, resume, clear, compact |
| `UserPromptSubmit` | Before the user's prompt is processed |
| `PreToolUse` / `PostToolUse` | Before/after a tool runs (filter with `matcher`) |
| `Stop` / `SubagentStop` | Session or subagent stops |

# How hooks reach settings.json

Hooks are never stored in [settings templates](/reference/settings-templates.md). When
settings are generated, every linked hook's `hooks.json` is overlaid onto the template, and
`${CLAUDE_PLUGIN_ROOT}` is replaced with the hook's portable `$HOME`-based path inside the
profile.[^settings-generator] Bundle hooks are merged the same way.

Legacy `hook.yaml` is still readable: `hub.GetHookManifest()` tries `hooks.json` first, then
falls back to `hook.yaml`.

Hooks are Claude Code only. omp does not run Claude hooks, so ccp never mirrors them
([omp linking](/features/omp/linking.md)).

[^settings-generator]: resolvePluginRootPath and hook overlay
