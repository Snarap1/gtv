# gtv — instructions for coding agents

## Project

`gtv` is a Go 1.22 CLI that runs Gradle tests and renders their results compactly.
It has no external Go dependencies. Gradle test events are collected as NDJSON by
`internal/gradlescript/listener.gradle`; do not parse Gradle console output.

## Working conventions

- Keep changes focused and preserve the existing package layout.
- Format Go code with `gofmt`.
- Add or update focused tests for behavioral changes.
- Run `go test ./...` before handing off. If the default Go cache is unavailable,
  use `GOCACHE=/tmp/gtv-gocache go test ./...`.
- Read `ROADMAP.md` before implementing a milestone; it contains verified Gradle
  behavior and open constraints. `CODE_REVIEW.md` records already-resolved issues.

## Key constraints

- Preserve streaming behavior and bounded memory in the runner.
- Treat malformed or unreadable NDJSON as an error, not an empty test run.
- Gradle test identity must include both task path and descriptor ID.
- Do not reintroduce `--rerun` for aggregate Gradle tasks; test tasks are forced
  to run in the init script so compilation remains cacheable.
