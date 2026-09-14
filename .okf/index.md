---
okf_version: '0.2'
---

# ccp — Claude Code Profile Manager

Go CLI that manages Claude Code profiles through a central hub of skills, agents, hooks, rules and commands. Start with the product overview, then follow the section that matches the task.

# Start here

* [Product overview](product/overview.md) - The problem ccp solves, its solution, explicit non-goals, assumptions, and deferred questions.
* [Core concepts and glossary](product/core-concepts.md) - The five user-facing concepts ccp is allowed to have, bundles as a composite hub item, and the glossary.
* [Design principles](engineering/design-principles.md) - How design choices are made in ccp — reuse over abstraction, a five-concept budget, flags over commands, one-function settings generation.
* [Development and release workflow](engineering/workflow.md) - How to change ccp safely — build and test loop, common tasks, release and tagging, git rules, version tracking.

# Sections

* [Product](product/) - Spec: problem, concepts, user stories, journeys, acceptance criteria, release history.
* [Architecture](architecture/) - Package layout, the ~/.ccp tree, key Go types, profile activation.
* [Reference](reference/) - File formats: profile.toml, ccp.toml, .ccp.yaml, hooks, settings templates.
* [Features](features/) - Bundles, project setup, source system, bootstrap, and the omp integration.
* [CLI](cli/) - Command reference and flags.
* [Engineering](engineering/) - Code standards, design principles, workflow, knowledge maintenance.
* [Decisions](decisions/) - Settled choices with rationale; not re-opened without a user request.
* [Anti-patterns](anti-patterns/) - Approaches that were tried, removed, and must not return.
