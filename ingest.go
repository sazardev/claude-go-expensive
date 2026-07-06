package expensive

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/sazardev/claude-go-expensive/pricing"
	"github.com/sazardev/claude-go-expensive/store"
	"github.com/sazardev/claude-go-expensive/transcript"
)

// fileTools maps tool names that operate on a single file to the input
// field that holds its path.
var fileTools = map[string]string{
	"Read":         "file_path",
	"Write":        "file_path",
	"Edit":         "file_path",
	"NotebookEdit": "notebook_path",
}

// buildSession groups a session's transcript entries into prompts: each
// top-level user turn (not a tool_result relay, not a subagent sidechain)
// starts a new prompt; every assistant reply and tool call up to the next
// top-level user turn is attributed to it.
//
// toolUseSeq maps a tool_use block's own ID to the Seq of the prompt that
// issued it, so a subagent transcript spawned by that tool call (see
// subagents.go) can be attributed back to the right prompt.
//
// ok is false if no session ID could be recovered from the transcript.
func buildSession(entries []transcript.Entry, sourcePath string, fileSize int64, fileModTime time.Time) (sess store.Session, toolUseSeq map[string]int, ok bool) {
	sess = store.Session{SourcePath: sourcePath, FileSize: fileSize, FileModTime: fileModTime}
	toolUseSeq = make(map[string]int)

	var current *store.Prompt
	seq := 0
	for _, e := range entries {
		if sess.RepoRootPath == "" && e.Cwd != "" {
			sess.RepoRootPath = e.Cwd
		}
		if sess.ID == "" && e.SessionID != "" {
			sess.ID = e.SessionID
		}
		if sess.GitBranch == "" && e.GitBranch != "" {
			sess.GitBranch = e.GitBranch
		}
		if sess.StartedAt.IsZero() && !e.Timestamp.IsZero() {
			sess.StartedAt = e.Timestamp
		}
		if !e.Timestamp.IsZero() {
			sess.EndedAt = e.Timestamp
		}

		if e.Message == nil {
			continue
		}

		switch e.Message.Role {
		case "user":
			if e.IsSidechain || e.Message.IsToolResultOnly() {
				continue
			}
			if current != nil {
				sess.Prompts = append(sess.Prompts, *current)
			}
			seq++
			current = &store.Prompt{UUID: e.UUID, Seq: seq, Text: e.Message.Text(), CreatedAt: e.Timestamp}

		case "assistant":
			if current == nil {
				// Assistant activity with no preceding top-level user turn
				// (e.g. the transcript opens mid-sidechain) — open an
				// unlabeled prompt rather than dropping the usage.
				seq++
				current = &store.Prompt{Seq: seq, CreatedAt: e.Timestamp}
			}
			if current.Model == "" {
				current.Model = e.Message.Model
			}
			addUsage(current, e.Message.Usage)

			blocks := e.Message.Blocks()
			for _, b := range blocks {
				if b.Type == "tool_use" && b.ID != "" {
					toolUseSeq[b.ID] = current.Seq
				}
			}
			current.ToolCalls = append(current.ToolCalls, toolCallsFromBlocks(blocks, sess.RepoRootPath, e.Timestamp)...)
		}
	}
	if current != nil {
		sess.Prompts = append(sess.Prompts, *current)
	}

	for i := range sess.Prompts {
		priceUsage(&sess.Prompts[i])
		rollUp(&sess, &sess.Prompts[i])
	}

	return sess, toolUseSeq, sess.ID != ""
}

// addUsage folds one assistant message's raw usage into p's running totals.
func addUsage(p *store.Prompt, u *transcript.Usage) {
	if u == nil {
		return
	}
	p.InputTokens += u.InputTokens
	p.OutputTokens += u.OutputTokens
	if u.CacheCreation != nil {
		p.CacheCreation5mTokens += u.CacheCreation.Ephemeral5mInputTokens
		p.CacheCreation1hTokens += u.CacheCreation.Ephemeral1hInputTokens
	} else {
		// Older transcripts without the 5m/1h breakdown: Claude Code's
		// default TTL is 5 minutes, so assume that.
		p.CacheCreation5mTokens += u.CacheCreationInputTokens
	}
	p.CacheReadTokens += u.CacheReadInputTokens
	if u.ServerToolUse != nil {
		p.WebSearches += u.ServerToolUse.WebSearchRequests
	}
	if p.Speed == "" && u.Speed != "" {
		p.Speed = u.Speed
	}
}

// priceUsage sets p.CostUSD from p's accumulated token/request totals.
func priceUsage(p *store.Prompt) {
	cost, ok := pricing.Cost(p.Model, pricing.Usage{
		InputTokens:           p.InputTokens,
		OutputTokens:          p.OutputTokens,
		CacheCreation5mTokens: p.CacheCreation5mTokens,
		CacheCreation1hTokens: p.CacheCreation1hTokens,
		CacheReadTokens:       p.CacheReadTokens,
		WebSearchRequests:     p.WebSearches,
		Speed:                 p.Speed,
	})
	if ok {
		p.CostUSD = cost
	}
}

// rollUp adds p's totals into sess's session-level totals.
func rollUp(sess *store.Session, p *store.Prompt) {
	sess.InputTokens += p.InputTokens
	sess.OutputTokens += p.OutputTokens
	sess.CacheCreation5mTokens += p.CacheCreation5mTokens
	sess.CacheCreation1hTokens += p.CacheCreation1hTokens
	sess.CacheReadTokens += p.CacheReadTokens
	sess.WebSearches += p.WebSearches
	sess.CostUSD += p.CostUSD
}

// toolCallsFromBlocks extracts ToolCall records, with file attribution,
// from one assistant message's content blocks.
func toolCallsFromBlocks(blocks []transcript.Block, repoRoot string, createdAt time.Time) []store.ToolCall {
	var calls []store.ToolCall
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		tc := store.ToolCall{ToolName: b.Name, CreatedAt: createdAt}
		if field, ok := fileTools[b.Name]; ok {
			if p := extractStringField(b.Input, field); p != "" {
				if rel, ok := relativeToRepo(repoRoot, p); ok {
					tc.FilePath = rel
				}
			}
		}
		calls = append(calls, tc)
	}
	return calls
}

func extractStringField(input json.RawMessage, field string) string {
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

// relativeToRepo returns filePath relative to repoRoot. ok is false if
// repoRoot is unknown or filePath falls outside it — those tool calls are
// still recorded, just without file attribution.
func relativeToRepo(repoRoot, filePath string) (string, bool) {
	if repoRoot == "" || filePath == "" {
		return "", false
	}
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}
