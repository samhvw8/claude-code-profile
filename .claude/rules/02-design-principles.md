# Design Principles

- Reuse existing types and patterns before adding new ones; a new abstraction only when reuse is impossible.
- Max 5 user-facing concepts — a 6th requires removing one.
- No processor/builder interfaces for single-path logic; settings generation is one function.
- A feature that can be a flag on an existing command is a flag; power-user commands may be hidden.
- Check `.okf/decisions/` and `.okf/anti-patterns/` before proposing a design: settled decisions are
  not re-opened without a user request, and removed approaches do not return.

Detail: `.okf/engineering/design-principles.md`.
