package store

import "time"

// Session is a fully-parsed Claude Code session, ready to persist.
type Session struct {
	ID                    string
	RepoRootPath          string // absolute path; repo identity
	SourcePath            string // path to the source .jsonl file; ingestion identity
	FileSize              int64
	FileModTime           time.Time
	GitBranch             string
	StartedAt             time.Time
	EndedAt               time.Time
	InputTokens           int64
	OutputTokens          int64
	CacheCreation5mTokens int64
	CacheCreation1hTokens int64
	CacheReadTokens       int64
	WebSearches           int64
	CostUSD               float64
	Prompts               []Prompt
}

// Prompt is one user turn within a session, plus everything the model did
// in response.
type Prompt struct {
	UUID                  string
	Seq                   int
	Text                  string
	Model                 string
	Speed                 string
	CreatedAt             time.Time
	InputTokens           int64
	OutputTokens          int64
	CacheCreation5mTokens int64
	CacheCreation1hTokens int64
	CacheReadTokens       int64
	WebSearches           int64
	CostUSD               float64
	ToolCalls             []ToolCall
}

// ToolCall is one tool invocation made while answering a Prompt.
type ToolCall struct {
	ToolName  string
	FilePath  string // relative to repo root; empty if not a file-touching tool
	CreatedAt time.Time
}

// Summary is an at-a-glance rollup across everything ingested.
type Summary struct {
	Projects              int64
	Repos                 int64
	Sessions              int64
	Prompts               int64
	InputTokens           int64
	OutputTokens          int64
	CacheCreation5mTokens int64
	CacheCreation1hTokens int64
	CacheReadTokens       int64
	WebSearches           int64
	CostUSD               float64
}

// ProjectCost is one row of the cost-by-project report.
type ProjectCost struct {
	Project      string
	Sessions     int64
	Prompts      int64
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// RepoCost is one row of the cost-by-repo report.
type RepoCost struct {
	Project      string
	RepoRootPath string
	Sessions     int64
	Prompts      int64
	CostUSD      float64
}

// SessionCost is one row of the cost-by-session report.
type SessionCost struct {
	SessionID    string
	Project      string
	RepoRootPath string
	StartedAt    time.Time
	Prompts      int64
	CostUSD      float64
}

// FileCost is one row of the cost-by-file report. CostUSD is the sum of the
// cost of every distinct prompt that touched the file — a prompt touching
// several files attributes its full cost to each, so this is "how much did
// work touching this file cost," not a partition of total spend.
type FileCost struct {
	Project      string
	RepoRootPath string
	FilePath     string
	ToolCalls    int64
	CostUSD      float64
}

// FolderCost is one row of the cost-by-folder report, aggregated the same
// way as FileCost.
type FolderCost struct {
	Project      string
	RepoRootPath string
	FolderPath   string
	ToolCalls    int64
	CostUSD      float64
}
