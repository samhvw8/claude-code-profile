---
type: Feature
title: "Output styles"
description: "The output-styles hub item type: linked into Claude Code like commands, and carried into omp, pi and opencode as a generated file because they have no output styles."
resource: internal/profile/style_carriers.go
tags: [feature, output-styles, integration, omp, pi, opencode]
generated: { by: claude-code/claude-opus-5-5, at: 2026-10-02T21:00:00Z }
sources:
  - id: pi-config-docs
    resource: "pi 0.99.2 docs/configuration.md (installed package)"
    title: pi agent directory and APPEND_SYSTEM.md
  - id: omp-binary
    resource: "omp 18.4.9 binary — APPEND_SYSTEM.md lookup and RPC get_state, checked 2026-10-02"
    title: omp append-system-prompt resolution
---

# The item type

`output-styles` is a leaf hub item type, like `commands`: `hub/output-styles/<name>.md` is linked
into `<profile>/output-styles/<name>.md` and listed under `[hub] output-styles` in
`profile.toml`. It is not a sixth concept — the five-concept budget is intact.

```bash
ccp link default output-styles/ste-lite.md
```

Linking makes a style selectable. Claude Code's `outputStyle` setting picks the active one — set it
in the profile's settings template or `settings-fragment.json`. Project setup copies styles into
`<project>/.claude/output-styles/` as well.

A profile type directory that is a symlink is refused — by `ccp link`, bundle linking, every
reconcile (`profile sync`, `profile edit`, `use`) and drift detection (`profile fix`, `doctor`) —
before anything changes. Through it, links would land wherever it points, and `profile fix` would
remove the files it points at: the hub itself, for the hand-made
`~/.claude/output-styles → hub/output-styles` link this type replaces. Remove that link (not its
target), then run `ccp profile sync <profile>`.

ccp did not manage `output-styles/` before it became a hub type, so there it removes only an
unlisted *link into the hub*. A real file, or a link to anywhere else, is the user's own style:
reconcile keeps it and logs it, and drift does not report it.

# The active style

`ActiveOutputStyle()` reads `outputStyle` from the profile's `settings.json` — the file Claude Code
reads at user level — and matches it against the linked styles: frontmatter `name`, else the
filename without `.md`; an exact match wins over a case-insensitive one.

| `outputStyle` | Carried to other harnesses |
|---------------|----------------------------|
| One linked style answers to it | Its body, frontmatter removed |
| Unset, or a built-in (`default`, `Explanatory`, `Learning`) | Nothing; ccp's carriers are removed |
| Two or more linked styles answer to it | Nothing; sync names every file. Carrying either would let a style that merely reuses a name replace the one chosen |
| No linked style answers to it | Nothing; sync reports the gap |
| Unknown — `settings.json` or a style file is unreadable, or a file's frontmatter is not valid YAML and no file matched | Carriers are left exactly as they are, and sync reports why |

A lookup problem is never fatal: an omp or opencode sync still mirrors agents, commands,
`AGENTS.md` and `RULES.md` when `settings.json` does not parse. An unreadable style file is
skipped, so a broken link to a deleted hub style cannot hide the selected one. A `name` in quotes is
still read when other frontmatter lines break YAML.

`/output-style` inside Claude Code saves the choice to that project's `.claude/settings.local.json`.
ccp does not follow it: the choice is per project, while the other harnesses' carriers are global.
Set the user-level default in the profile's settings template or `settings-fragment.json`.

`keep-coding-instructions` has no equivalent elsewhere: the other harnesses append the style to
their own prompt, they never replace it.

# Carriers

| Harness | File | How it loads | Written by |
|---------|------|--------------|------------|
| Claude Code | `<profile>/output-styles/<name>.md` (link) | `outputStyle` | `ccp link` |
| omp | `~/.omp/agent/APPEND_SYSTEM.md`, or `~/.omp/profiles/<name>/agent/` | Appended to every system prompt[^omp-binary] | `ccp omp sync` ([global context](/features/omp/global-context.md)) |
| pi | `<pi agent dir>/APPEND_SYSTEM.md` | Appended to every system prompt[^pi-config-docs] | `ccp pi sync` |
| opencode | `~/.config/opencode/output-style.md` | Listed once in `instructions` | `ccp opencode sync` ([opencode](/features/opencode.md)) |

omp's path comes from omp's own lookup, which ignores `PI_CODING_AGENT_DIR` (checked against omp
18.4.9); a project `.omp/APPEND_SYSTEM.md` takes precedence over it.

## Ownership: the carrier ledger

`~/.ccp/state/output-style-carriers.json` records every carrier ccp wrote — absolute path and the
hash of what was written. It is the only evidence of ownership:

| On disk | Sync does |
|---------|-----------|
| Nothing, style wanted | Writes it (temp file and rename: a session may read it at any moment) and records it |
| Exactly what ccp recorded | Rewrites it when the style changed, removes it when none is selected |
| Exactly what ccp would write, not recorded | Adopts it |
| Recorded, but changed since | Leaves it, reports "edited by hand", and forgets it |
| Anything else | Leaves it and reports it as occupied — a first line with ccp's `<!-- generated by ccp` marker grants nothing |

Carriers are grouped, and a sync updates every recorded carrier of its group, not only the one at
today's path. pi and opencode follow the active profile, so each is one group: a carrier written
while `PI_CODING_AGENT_DIR` or `XDG_CONFIG_HOME` pointed elsewhere is kept current or removed,
instead of loading a stale style forever. omp's named profiles mirror different ccp profiles on
purpose, so each omp directory is its own group.

Every ledger change goes through one locked load-change-save (`updateCarrierLedger`):

- An exclusive lock file serializes ccp processes, so a hook and a manual command cannot save over
  each other; a lock older than a minute is from a killed process and is taken over.
- The ledger is saved even when a carrier fails, so a carrier written before the failure stays
  provably ccp's.
- A ledger that is not valid JSON is moved to `.bak` and replaced by an empty one, with one line
  saying ownership proof was lost: carriers still holding ccp's text are adopted again, any other is
  left alone. `followHarnesses` repairs it before anything follows; status reports it as stale.
- An entry is dropped when its file is gone and nothing is wanted. An entry whose directory is
  missing — an unmounted drive — is kept and skipped.

Each written carrier is reported with its size, since the whole style is paid for in tokens in
every session of every harness that carries it.

The ledger also records which groups follow automatically (pi). `ccp reset` lists the carriers ccp
still owns before `~/.ccp` goes and removes them after, re-checking each hash first; one changed in
between is kept. `ccp omp unlink --all` removes omp's.

The header names the source style and does not name the harness, so omp and pi sharing one
directory cannot make their syncs undo each other.

# Following the profile

`followHarnesses` (`cmd/follow.go`) is the one place a profile change reaches the other harnesses,
so all of them follow the same events: `use -g`, link/unlink, interactive link, `profile edit`,
`profile sync` and `bootstrap` — and, as a Cobra `PostRun`, every command that changes hub items in
place: `hub rename`, `hub remove`, `hub update`, `install`, `source install` and `source update`.
A renamed or rewritten style would otherwise keep its old text in the other harnesses until an
unrelated command removed it. It re-syncs opencode and pi once the user has synced them by hand,
and refreshes omp's style carrier once ccp owns anything in omp — a style carrier it still owns
there counts, so deselecting a style always reaches omp. omp's links and context still
follow profile switches only ([profile switching](/features/omp/profile-switching.md)).

Changes are not live: editing a hub style's file directly updates Claude Code at once through the
link, and the other harnesses on the next of those events or an explicit sync. `ccp link --help`
says so.

# pi

`ccp pi sync [profile]` (hidden) writes the carrier into pi's agent directory —
`PI_CODING_AGENT_DIR`, else `~/.pi/agent` — and opts in to following. Carriers are global and follow
the active profile: syncing another profile by hand (`ccp pi sync dev`, `ccp opencode sync dev`)
prints a note that the active profile's next change replaces it. A pi carrier is no omp
opt-in: omp's opt-in looks only at its links, `AGENTS.md` and `RULES.md`.

# init

`ccp init` moves an existing `~/.claude/output-styles` into the default profile whole — directory
or symlink — rather than scanning its styles into the hub. Hidden files survive, and a directory
symlinked into dotfiles is moved as a link, never emptied. A symlinked one is then refused by the
type-directory guard above until it is replaced by links to hub styles.

[^pi-config-docs]: pi configuration docs — `<agent-dir>/APPEND_SYSTEM.md` "Adds instructions to Pi's system prompt".
[^omp-binary]: Verified with `omp --mode rpc --no-session` `get_state`: the file's text appeared at the end of `systemPrompt`.
