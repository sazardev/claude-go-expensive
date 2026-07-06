package expensive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sazardev/claude-go-expensive/pricing"
	"github.com/sazardev/claude-go-expensive/store"
	"github.com/sazardev/claude-go-expensive/transcript"
)

// subagentMeta is the sidecar Claude Code writes next to each subagent
// transcript (agent-<id>.meta.json), linking it back to the tool_use call
// that spawned it.
type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	ToolUseID   string `json:"toolUseId"`
	SpawnDepth  int    `json:"spawnDepth"`
}

// isSubagentTranscript reports whether path is one of the per-subagent
// transcripts Claude Code writes under <session-dir>/subagents/ — these are
// not standalone sessions and must be skipped by directory walks that
// ingest top-level session files (see mergeSubagents, which folds them into
// their parent session instead).
func isSubagentTranscript(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "subagents"
}

// mergeSubagents folds every subagent transcript spawned during sess
// (Claude Code writes one JSONL file per Task/Agent tool invocation, under
// <same dir as mainPath>/<sess.ID>/subagents/) into the prompt that spawned
// it, matched via the tool_use ID recorded in the subagent's .meta.json
// sidecar against toolUseSeq (built by buildSession). A subagent transcript
// whose spawning tool call can't be identified is still counted, under a
// synthetic "unattributed" prompt, so its cost is never silently dropped.
// A subagent file that fails to read is skipped — same tolerance
// transcript.Read applies to malformed lines within one file.
//
// This must run after buildSession has already priced sess.Prompts, since
// it adds directly to their (and sess's) token and cost totals.
func mergeSubagents(sess *store.Session, toolUseSeq map[string]int, mainPath string) {
	dir := filepath.Join(filepath.Dir(mainPath), sess.ID, "subagents")
	agentFiles, err := filepath.Glob(filepath.Join(dir, "agent-*.jsonl"))
	if err != nil || len(agentFiles) == 0 {
		return
	}

	unattributedIdx := -1

	for _, agentPath := range agentFiles {
		entries, err := transcript.ReadFile(agentPath)
		if err != nil {
			continue
		}
		usage, model, calls := aggregateSubagent(entries, sess.RepoRootPath)
		if usage == (pricing.Usage{}) && len(calls) == 0 {
			continue
		}
		cost, _ := pricing.Cost(model, usage)

		idx := -1
		if meta, err := readSubagentMeta(metaPathFor(agentPath)); err == nil {
			if seq, ok := toolUseSeq[meta.ToolUseID]; ok {
				for i := range sess.Prompts {
					if sess.Prompts[i].Seq == seq {
						idx = i
						break
					}
				}
			}
		}
		if idx == -1 {
			if unattributedIdx == -1 {
				sess.Prompts = append(sess.Prompts, store.Prompt{
					Seq:  len(sess.Prompts) + 1,
					Text: "(subagent work not attributable to a specific prompt)",
				})
				unattributedIdx = len(sess.Prompts) - 1
			}
			idx = unattributedIdx
		}

		p := &sess.Prompts[idx]
		p.InputTokens += usage.InputTokens
		p.OutputTokens += usage.OutputTokens
		p.CacheCreation5mTokens += usage.CacheCreation5mTokens
		p.CacheCreation1hTokens += usage.CacheCreation1hTokens
		p.CacheReadTokens += usage.CacheReadTokens
		p.WebSearches += usage.WebSearchRequests
		p.CostUSD += cost
		p.ToolCalls = append(p.ToolCalls, calls...)

		sess.InputTokens += usage.InputTokens
		sess.OutputTokens += usage.OutputTokens
		sess.CacheCreation5mTokens += usage.CacheCreation5mTokens
		sess.CacheCreation1hTokens += usage.CacheCreation1hTokens
		sess.CacheReadTokens += usage.CacheReadTokens
		sess.WebSearches += usage.WebSearchRequests
		sess.CostUSD += cost
	}
}

// aggregateSubagent sums a subagent transcript's own usage and file-touching
// tool calls. A subagent conversation is priced as one aggregate rather
// than per-turn like a top-level session's prompts — proportionate to how
// coarse "one prompt, one model" attribution already is at that level.
func aggregateSubagent(entries []transcript.Entry, repoRoot string) (pricing.Usage, string, []store.ToolCall) {
	var total pricing.Usage
	var model string
	var calls []store.ToolCall

	agg := &store.Prompt{}
	for _, e := range entries {
		if e.Message == nil || e.Message.Role != "assistant" {
			continue
		}
		if model == "" {
			model = e.Message.Model
		}
		addUsage(agg, e.Message.Usage)
		calls = append(calls, toolCallsFromBlocks(e.Message.Blocks(), repoRoot, e.Timestamp)...)
	}
	total = pricing.Usage{
		InputTokens:           agg.InputTokens,
		OutputTokens:          agg.OutputTokens,
		CacheCreation5mTokens: agg.CacheCreation5mTokens,
		CacheCreation1hTokens: agg.CacheCreation1hTokens,
		CacheReadTokens:       agg.CacheReadTokens,
		WebSearchRequests:     agg.WebSearches,
		Speed:                 agg.Speed,
	}
	return total, model, calls
}

func metaPathFor(agentPath string) string {
	return strings.TrimSuffix(agentPath, ".jsonl") + ".meta.json"
}

func readSubagentMeta(path string) (subagentMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return subagentMeta{}, err
	}
	var m subagentMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return subagentMeta{}, err
	}
	return m, nil
}
