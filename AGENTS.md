# AGENTS.md

## Project

- Go module `claude-go-expensive`, requires Go 1.26.4.
- Early-stage: no Go source files, no build/test/lint config, no CI, no remote.

## Skills (auto-installed)

Loaded from `skills-lock.json` based on `go.mod`:
- `golang-patterns` — idiomatic Go conventions
- `golang-testing` — table-driven tests, subtests, benchmarks
- `caveman`, `caveman-commit`, `caveman-review` — compressed output

## First-time setup

No `go.sum` yet. Run `go mod tidy` before building or testing.

## Commands

No standard commands defined yet. When adding them, register in `AGENTS.md`:
- test: `go test ./...`
- lint: (none configured)
- typecheck: `go build ./...`
