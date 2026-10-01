---
type: Decision
title: "A machine-local snapshot tells settings edits from input changes"
description: "ccp records what it last wrote to settings.json under ~/.ccp/state, so regeneration keeps edits but applies template and fragment changes."
tags: [decision, settings]
decided: 2026-10-01
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-01T08:00:00Z }
sources:
  - id: session-review
    resource: "Review of agent sessions using ccp, 2026-09-13 to 2026-10-01"
    title: Session transcripts showing settings.json edits lost to profile sync --force
---

# Decision

ccp keeps `~/.ccp/state/settings/<profile>.json` for each profile: what it last generated for
`settings.json` (template, fragment and its own hooks). Regeneration compares `settings.json` against it, and against a fresh
generation, before overwriting anything. The file is machine-local state. It lives outside the
profile directory so that a dotfiles setup linking whole profiles (stow, a linked `~/.ccp`) cannot
carry it, and `ccp bootstrap --push` does not add it.

# Why

Without a record of what ccp wrote, an edit on disk and a changed input (a template switch, a
fragment edit) look the same. Treating both as edits froze template changes; treating both as
inputs silently discarded edits made by hand or by Claude Code, which agents ran into repeatedly
with `ccp profile sync --force`. Storing a hash would detect a change but not say which keys; the
full snapshot names them and also tells ccp's own hooks from hooks added with `/hooks`.

# Implication

* `settings.json` is still the file Claude Code reads; the snapshot is never read by Claude Code.
* A profile with no snapshot (older ccp, or a new machine) falls back to comparing with a fresh
  generation until ccp next writes its settings. Syncing the snapshot through dotfiles was declined:
  it describes this machine's last write, and a copy from another machine would misclassify edits.
  A first version kept it inside the profile directory; review showed whole-profile dotfiles links
  would commit it, so it moved to `~/.ccp/state/`.
* See [settings templates](/reference/settings-templates.md) for the rules ccp applies.
