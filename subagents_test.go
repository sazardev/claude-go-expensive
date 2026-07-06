package expensive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sazardev/claude-go-expensive/transcript"
)

// writeSessionWithSubagent lays out a session the way Claude Code does when
// it spawns one subagent: <dir>/<session>.jsonl plus
// <dir>/<session>/subagents/agent-x.jsonl (+ .meta.json sidecar), both
// carrying the same sessionId.
func writeSessionWithSubagent(t *testing.T, dir, sessionID string) string {
	t.Helper()

	main := `{"type":"user","uuid":"u1","sessionId":"` + sessionID + `","cwd":"/repo","timestamp":"2026-07-06T10:00:00Z","message":{"role":"user","content":"investigate the bug"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"` + sessionID + `","timestamp":"2026-07-06T10:00:02Z","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","id":"toolu_agent1","name":"Agent","input":{"prompt":"explore the repo"}}],"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
`
	mainPath := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatalf("write main session: %v", err)
	}

	subagentDir := filepath.Join(dir, sessionID, "subagents")
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatalf("mkdir subagents: %v", err)
	}

	agent := `{"type":"user","uuid":"su1","sessionId":"` + sessionID + `","isSidechain":true,"cwd":"/repo","timestamp":"2026-07-06T10:00:03Z","message":{"role":"user","content":"explore the repo"}}
{"type":"assistant","uuid":"sa1","sessionId":"` + sessionID + `","isSidechain":true,"timestamp":"2026-07-06T10:00:05Z","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","id":"toolu_sub1","name":"Read","input":{"file_path":"/repo/main.go"}}],"usage":{"input_tokens":5000,"output_tokens":2000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
`
	if err := os.WriteFile(filepath.Join(subagentDir, "agent-x.jsonl"), []byte(agent), 0o644); err != nil {
		t.Fatalf("write subagent transcript: %v", err)
	}

	meta := `{"agentType":"Explore","description":"explore the repo","toolUseId":"toolu_agent1","spawnDepth":1}`
	if err := os.WriteFile(filepath.Join(subagentDir, "agent-x.meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write subagent meta: %v", err)
	}

	return mainPath
}

func TestIngestDirMergesSubagentIntoParentSession(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	const sessionID = "22222222-2222-2222-2222-222222222222"
	writeSessionWithSubagent(t, dir, sessionID)

	tr, err := Open(filepath.Join(t.TempDir(), "expensive.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer tr.Close()

	stats, err := tr.IngestDir(ctx, dir)
	if err != nil {
		t.Fatalf("IngestDir() error = %v", err)
	}
	if len(stats.Errors) != 0 {
		t.Fatalf("IngestDir() errors = %v; want none (subagent file must not be ingested as its own session)", stats.Errors)
	}
	if stats.Sessions != 1 {
		t.Fatalf("IngestDir() stats.Sessions = %d; want 1 (only the top-level session, not the subagent file)", stats.Sessions)
	}

	summary, err := tr.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Sessions != 1 {
		t.Fatalf("Summary().Sessions = %d; want 1", summary.Sessions)
	}
	// 100+5000 input, 20+2000 output: the subagent's usage must be folded in,
	// not dropped.
	if summary.InputTokens != 5100 || summary.OutputTokens != 2020 {
		t.Errorf("Summary() tokens = in:%d out:%d; want in:5100 out:2020 (main + subagent)", summary.InputTokens, summary.OutputTokens)
	}

	byFile, err := tr.CostByFile(ctx, 10)
	if err != nil {
		t.Fatalf("CostByFile() error = %v", err)
	}
	if len(byFile) != 1 || byFile[0].FilePath != "main.go" {
		t.Fatalf("CostByFile() = %+v; want the subagent's Read of main.go to be attributed", byFile)
	}
}

func TestMergeSubagentsFallsBackToUnattributed(t *testing.T) {
	dir := t.TempDir()
	const sessionID = "33333333-3333-3333-3333-333333333333"
	mainPath := writeSessionWithSubagent(t, dir, sessionID)

	// Break the link: meta.json points at a tool_use ID that doesn't exist
	// in the main transcript.
	metaPath := filepath.Join(dir, sessionID, "subagents", "agent-x.meta.json")
	if err := os.WriteFile(metaPath, []byte(`{"toolUseId":"toolu_does_not_exist"}`), 0o644); err != nil {
		t.Fatalf("rewrite meta: %v", err)
	}

	entries, err := transcript.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	info, err := os.Stat(mainPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	sess, toolUseSeq, ok := buildSession(entries, mainPath, info.Size(), info.ModTime())
	if !ok {
		t.Fatalf("buildSession() ok = false")
	}
	if len(sess.Prompts) != 1 {
		t.Fatalf("len(sess.Prompts) before merge = %d; want 1", len(sess.Prompts))
	}

	mergeSubagents(&sess, toolUseSeq, mainPath)

	if len(sess.Prompts) != 2 {
		t.Fatalf("len(sess.Prompts) after merge = %d; want 2 (original + unattributed subagent bucket)", len(sess.Prompts))
	}
	unattributed := sess.Prompts[1]
	if unattributed.InputTokens != 5000 || unattributed.OutputTokens != 2000 {
		t.Errorf("unattributed prompt usage = %+v; want the subagent's usage, not dropped", unattributed)
	}
}
