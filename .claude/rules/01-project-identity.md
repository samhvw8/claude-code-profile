# Project Identity

ccp is a Go CLI that manages Claude Code profiles through a central hub. Project knowledge lives in
the OKF bundle at `.okf/` — start at `.okf/index.md`.

- At most seven user-facing concepts; six are in use: **Hub**, **Profile**, **Settings Template**,
  **Source**, **Activation**, **Account**. Bundles are a kind of hub item, not a concept
  (`.okf/product/core-concepts.md`, `.okf/decisions/account-concept.md`).
- Removed in v0.28 and never to return: engines, contexts, setting fragments, linked dirs
  (`@import` parsing), DataConfig, processor interfaces (`.okf/decisions/v0-28-simplification.md`).
