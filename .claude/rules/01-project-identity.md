# Project Identity

ccp is a Go CLI that manages Claude Code profiles through a central hub. Project knowledge lives in
the OKF bundle at `.okf/` — start at `.okf/index.md`.

- Five user-facing concepts only: **Hub**, **Profile**, **Settings Template**, **Source**,
  **Activation**. Bundles are a kind of hub item, not a sixth (`.okf/product/core-concepts.md`).
- Removed in v0.28 and never to return: engines, contexts, setting fragments, linked dirs
  (`@import` parsing), DataConfig, processor interfaces (`.okf/decisions/v0-28-simplification.md`).
