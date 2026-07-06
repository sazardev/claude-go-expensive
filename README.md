# claude-go-expensive

[![Go Reference](https://pkg.go.dev/badge/github.com/sazardev/claude-go-expensive.svg)](https://pkg.go.dev/github.com/sazardev/claude-go-expensive)
[![CI](https://github.com/sazardev/claude-go-expensive/actions/workflows/ci.yml/badge.svg)](https://github.com/sazardev/claude-go-expensive/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sazardev/claude-go-expensive)](https://goreportcard.com/report/github.com/sazardev/claude-go-expensive)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A Go library that turns Claude Code's own session transcripts
(`~/.claude/projects/**/*.jsonl`) into a queryable SQLite database of token
usage and cost, broken down by **project → repo → folder → file → session →
prompt**.

It doesn't call the Claude API or instrument anything — it reads the JSONL
logs Claude Code already writes locally, computes cost from the published
per-model pricing, and stores the result so you can slice it with SQL (or
the built-in report methods) instead of scrolling through logs.

Ships both as a **CLI** (`claude-cost`) and as an importable **Go library**.

## CLI

Requires Go 1.26+.

```sh
go install github.com/sazardev/claude-go-expensive/cmd/claude-cost@latest
```

That drops a `claude-cost` binary in `$(go env GOPATH)/bin` — make sure it's
on your `PATH`, then run it from anywhere, like any other CLI tool:

```sh
claude-cost                    # summary + cost by project + top 5 sessions
claude-cost projects           # cost by project
claude-cost repos              # cost by repo/clone
claude-cost sessions -limit 20 # top N sessions
claude-cost files -limit 20    # top N files
claude-cost folders -limit 20  # top N folders
claude-cost version
claude-cost --help
```

No setup needed — it ingests `~/.claude/projects` on every run (incremental:
unchanged sessions are skipped) into `~/.claude-cost/expensive.db`, a fixed
location so the same history is there no matter where you invoke it from.
Override with `-db path` / `-logs path` if you need to.

## Go library

```sh
go get github.com/sazardev/claude-go-expensive
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	expensive "github.com/sazardev/claude-go-expensive"
)

func main() {
	ctx := context.Background()

	tr, err := expensive.Open("expensive.db")
	if err != nil {
		log.Fatal(err)
	}
	defer tr.Close()

	logsDir, err := expensive.DefaultLogsDir() // ~/.claude/projects
	if err != nil {
		log.Fatal(err)
	}

	stats, err := tr.IngestDir(ctx, logsDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ingested %d sessions (%d already up to date)\n", stats.Sessions, stats.Skipped)

	byProject, err := tr.CostByProject(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range byProject {
		fmt.Printf("%s: $%.2f across %d sessions\n", p.Project, p.CostUSD, p.Sessions)
	}
}
```

Re-running `IngestDir` is cheap and safe: a session file that hasn't changed
(same size and modification time) is skipped, and a session file that grew
(the common case — Claude Code appends to the active session live) is
re-parsed and replaces the prior data for that session.

## Reports (library)

`Tracker` exposes:

- `Summary` — totals across everything ingested
- `CostByProject`, `CostByRepo` — aggregated, most expensive first
- `CostBySession(ctx, limit)`, `CostByFile(ctx, limit)`, `CostByFolder(ctx, limit)` — top-N, most expensive first

For anything else, open the SQLite file directly — the schema is in
[`store/schema.sql`](store/schema.sql).

## How it works

- **`transcript`** parses the raw JSONL lines Claude Code writes per session,
  including the nested usage breakdown (5m vs 1h cache writes, server tool
  use) that newer Claude Code versions emit.
- **`pricing`** holds the published USD-per-token rates for every Claude
  model (plus Fast Mode's premium rate), keyed by the exact model ID string
  that shows up in a transcript.
- The root package groups a session's entries into **prompts** (one per
  top-level user turn) and **tool calls** (assistant tool use within that
  turn), attributing token usage and cost to each, and extracts the file
  path touched by `Read`/`Write`/`Edit`/`NotebookEdit` calls for the
  file/folder rollups.
- **Subagents** (Task/Agent tool calls) get their own JSONL file under
  `<session>/subagents/agent-*.jsonl`, sharing the parent session's ID.
  These are folded into the prompt that spawned them — matched via the
  `toolUseId` in the subagent's `.meta.json` sidecar — rather than ingested
  as (colliding, since the ID isn't unique) standalone sessions. A subagent
  whose spawning call can't be identified still counts, under a synthetic
  "unattributed" prompt, so its cost is never silently dropped.
- **Project identity** prefers the git remote (`origin`) URL over the
  directory name, resolved from `.git/config` (following a worktree's
  `.git` file pointer to the main repo's config when needed). This rolls
  multiple clones or worktrees of the same repository up under one project
  instead of fragmenting cost by directory name.
- **`store`** persists the result to SQLite and answers the report queries.
- **`cmd/claude-cost`** is a thin CLI over the same `Tracker` API, with a
  fixed default database location (`~/.claude-cost/expensive.db`) so it
  behaves like a normal installed tool rather than a per-directory script.

### Known limitations

- Cost attribution per file/folder is "how much did work touching this file
  cost," not a partition of total spend — a prompt that touches three files
  attributes its full cost to each, so summing across files overcounts
  relative to the session total.
- A subagent transcript is priced as one aggregate (one model, one usage
  total), not per-turn like a top-level session's prompts.
- `service_tier` values other than `"standard"` (priority/batch) aren't
  priced — a request billed under one of those tiers is costed at the
  standard rate.
- `claude-sonnet-5` pricing reflects the introductory rate in effect through
  2026-08-31; update `pricing/pricing.go` after that date.

### Validated against real usage

Tested against a real `~/.claude/projects` tree (32 session files across 9
directories, several of them separate clones of the same repo). Two
findings from that run drove real fixes, not just theoretical ones:

- **Cache writes aren't all 5-minute.** ~70% of this account's
  `cache_creation_input_tokens` turned out to be 1-hour writes (billed at 2x
  input, not 1.25x) once the transcript's `cache_creation` breakdown was
  read instead of assumed.
- **Subagent work was either dropped or crashed the ingest.** Sessions using
  the `Agent`/`Task` tool write a separate JSONL per subagent under
  `<session>/subagents/`, carrying the *same* session ID as the parent —
  before the fix this caused a `UNIQUE constraint failed` on every subagent
  file and left their tokens (in one session, ~104K output tokens) out of
  the total entirely.

Fixing both raised the measured total cost on that account by roughly 4.4x
— the subagent gap was the larger of the two.

## Development

```sh
go build ./...       # typecheck
go vet ./...          # lint
go test ./...         # test
gofmt -l .             # formatting (should print nothing)
```

Pull requests welcome. See [LICENSE](LICENSE) (MIT).
