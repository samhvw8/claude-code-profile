# omp (oh-my-pi) integration

* [omp integration overview](overview.md) - Why ccp mirrors a profile into omp's own agent directory, what the mirror covers, and the ccp omp commands.
* [What ccp links into omp](linking.md) - The item types ccp links, why rules and hooks are not linked, the reporter-only loadability check, link identity, and what is never overwritten.
* [omp global context (AGENTS.md and RULES.md)](global-context.md) - The two user-level files ccp sets up so omp sessions get the profile's CLAUDE.md and every rule.
* [Following profile switches in omp](profile-switching.md) - How ccp use -g and ccp bootstrap keep omp in step with the active profile, and what counts as opting in.
* [omp health checks and live verification](health-and-verification.md) - What ccp omp list and ccp doctor report about omp, and how doctor --verify-omp asks a live omp what it loaded.
* [omp teardown and path resolution](teardown-and-paths.md) - How ccp finds every omp agent directory it may have written into, removes exactly its own paths, and resolves omp named profiles.
