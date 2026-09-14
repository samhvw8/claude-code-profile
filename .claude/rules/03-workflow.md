# Workflow Rules

- After every change: `go build -o ccp .` and `go test ./...`.
- Never force-update tags (`git tag -f`, `git push --tags -f`); always increment the version and create a new tag.
- Never push without an explicit request — the user pushes after reviewing.
- Update `.okf/` in the same change as the code it describes: touched concepts, their `index.md`, and a
  dated `.okf/log.md` entry. Load the `okf` skill first; run the `validate` skill with `--strict` before
  calling doc work done.
- Release: update `.okf/` and the version line in `CLAUDE.md`, add the release to
  `.okf/product/release-history.md`, commit, tag — do not push.

Detail: `.okf/engineering/workflow.md`, `.okf/engineering/code-standards.md`.
