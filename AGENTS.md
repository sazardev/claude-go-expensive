# AGENTS.md

## Project

- Go module `claude-go-expensive`, requires Go 1.26.4.
- Library that parses Claude Code's own session transcripts
  (`~/.claude/projects/**/*.jsonl`) into a SQLite database of token usage
  and cost, by project/repo/folder/file/session/prompt. No CLI, no CI, no
  remote — see `README.md` for usage.

## Packages

- `pricing` — Claude model USD-per-token pricing table + cost calculation
  (`pricing.Usage`/`pricing.Cost`), including Fast Mode rates.
- `transcript` — parses raw JSONL transcript lines, including the nested
  `cache_creation` (5m/1h split) and `server_tool_use` usage fields.
- `store` — SQLite schema, persistence (`SaveSession`), and report queries.
- root package (`expensive`) — `Tracker` facade: `Open`, `IngestFile`,
  `IngestDir`, and the report methods.
  - `ingest.go` — groups transcript entries into prompts/tool calls.
  - `subagents.go` — folds `<session>/subagents/agent-*.jsonl` transcripts
    (same session ID as the parent) into the prompt that spawned them.
  - `gitremote.go` — derives project identity from a repo's git remote
    instead of its directory name.

## Skills (auto-installed)

Loaded from `skills-lock.json` based on `go.mod`:
- `golang-patterns` — idiomatic Go conventions
- `golang-testing` — table-driven tests, subtests, benchmarks
- `caveman`, `caveman-commit`, `caveman-review` — compressed output

## Commands

- test: `go test ./...`
- lint: `go vet ./...` (no additional linter configured)
- typecheck: `go build ./...`
- tidy: `go mod tidy`
