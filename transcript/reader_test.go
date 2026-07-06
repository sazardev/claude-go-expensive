package transcript

import (
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"user","uuid":"u1","sessionId":"sess-1","cwd":"/repo","timestamp":"2026-07-06T10:00:00Z","message":{"role":"user","content":"fix the bug"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"sess-1","timestamp":"2026-07-06T10:00:05Z","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"On it."},{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/repo/main.go"}}],"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		``,
		`not json at all`,
		`{"type":"user","uuid":"tr1","parentUuid":"a1","sessionId":"sess-1","timestamp":"2026-07-06T10:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents"}]}}`,
	}, "\n")

	entries, err := Read(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Read() returned %d entries; want 3 (malformed/blank lines skipped)", len(entries))
	}

	if got := entries[0].Message.Text(); got != "fix the bug" {
		t.Errorf("entries[0].Message.Text() = %q; want %q", got, "fix the bug")
	}

	blocks := entries[1].Message.Blocks()
	if len(blocks) != 2 || blocks[1].Type != "tool_use" || blocks[1].Name != "Read" {
		t.Fatalf("entries[1].Message.Blocks() = %+v; want [text, tool_use Read]", blocks)
	}
	if got := entries[1].Message.Usage.InputTokens; got != 100 {
		t.Errorf("entries[1].Message.Usage.InputTokens = %d; want 100", got)
	}

	if !entries[2].Message.IsToolResultOnly() {
		t.Errorf("entries[2].Message.IsToolResultOnly() = false; want true")
	}
}

func TestMessageBlocksNilSafety(t *testing.T) {
	var m *Message
	if got := m.Blocks(); got != nil {
		t.Errorf("nil Message.Blocks() = %v; want nil", got)
	}
	if got := m.Text(); got != "" {
		t.Errorf("nil Message.Text() = %q; want empty", got)
	}
}
