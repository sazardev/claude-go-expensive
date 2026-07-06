package expensive

import (
	"testing"
	"time"

	"github.com/sazardev/claude-go-expensive/transcript"
)

func approxEqual(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	return d < eps && d > -eps
}

func TestBuildSession(t *testing.T) {
	entries, err := transcript.ReadFile("testdata/sample_session.jsonl")
	if err != nil {
		t.Fatalf("transcript.ReadFile() error = %v", err)
	}

	sess, toolUseSeq, ok := buildSession(entries, "testdata/sample_session.jsonl", 1234, time.Unix(0, 0))
	if !ok {
		t.Fatalf("buildSession() ok = false; want true")
	}
	if got := toolUseSeq["toolu_1"]; got != 1 {
		t.Errorf(`toolUseSeq["toolu_1"] = %d; want 1 (issued by Prompts[0])`, got)
	}
	if got := toolUseSeq["toolu_3"]; got != 2 {
		t.Errorf(`toolUseSeq["toolu_3"] = %d; want 2 (issued by Prompts[1])`, got)
	}

	if sess.ID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("sess.ID = %q; want the session uuid", sess.ID)
	}
	if sess.RepoRootPath != "/home/dev/project" {
		t.Errorf("sess.RepoRootPath = %q; want %q", sess.RepoRootPath, "/home/dev/project")
	}
	if sess.GitBranch != "main" {
		t.Errorf("sess.GitBranch = %q; want %q", sess.GitBranch, "main")
	}
	if len(sess.Prompts) != 2 {
		t.Fatalf("len(sess.Prompts) = %d; want 2", len(sess.Prompts))
	}

	p0 := sess.Prompts[0]
	if p0.Text != "fix the bug in main.go" {
		t.Errorf("Prompts[0].Text = %q", p0.Text)
	}
	if p0.InputTokens != 1400 || p0.OutputTokens != 100 ||
		p0.CacheCreation5mTokens != 150 || p0.CacheCreation1hTokens != 50 || p0.CacheReadTokens != 500 {
		t.Errorf("Prompts[0] usage = %+v; want input=1400 output=100 cache5m=150 cache1h=50 cacheRead=500", p0)
	}
	if len(p0.ToolCalls) != 2 {
		t.Fatalf("len(Prompts[0].ToolCalls) = %d; want 2 (Read + Edit)", len(p0.ToolCalls))
	}
	for _, tc := range p0.ToolCalls {
		if tc.FilePath != "main.go" {
			t.Errorf("Prompts[0].ToolCalls file path = %q; want %q", tc.FilePath, "main.go")
		}
	}
	// 5m writes cost 1.25x input; 1h writes cost 2x input — mixing them
	// verifies the split isn't silently collapsed into one rate.
	wantCost0 := 1400*5e-6 + 100*25e-6 + 150*6.25e-6 + 50*10e-6 + 500*0.5e-6
	if !approxEqual(p0.CostUSD, wantCost0) {
		t.Errorf("Prompts[0].CostUSD = %v; want %v", p0.CostUSD, wantCost0)
	}

	p1 := sess.Prompts[1]
	if p1.Text != "now add a test" {
		t.Errorf("Prompts[1].Text = %q", p1.Text)
	}
	if len(p1.ToolCalls) != 1 || p1.ToolCalls[0].FilePath != "main_test.go" {
		t.Errorf("Prompts[1].ToolCalls = %+v; want one Write of main_test.go", p1.ToolCalls)
	}

	wantSessionCost := wantCost0 + (400*5e-6 + 150*25e-6)
	if !approxEqual(sess.CostUSD, wantSessionCost) {
		t.Errorf("sess.CostUSD = %v; want %v", sess.CostUSD, wantSessionCost)
	}
	if sess.InputTokens != 1800 || sess.OutputTokens != 250 {
		t.Errorf("sess totals = input:%d output:%d; want input:1800 output:250", sess.InputTokens, sess.OutputTokens)
	}
}

func TestRelativeToRepo(t *testing.T) {
	tests := []struct {
		name     string
		repoRoot string
		filePath string
		wantRel  string
		wantOK   bool
	}{
		{"inside repo", "/repo", "/repo/pkg/main.go", "pkg/main.go", true},
		{"repo root itself", "/repo", "/repo", "", false},
		{"outside repo", "/repo", "/etc/hosts", "", false},
		{"unknown repo root", "", "/repo/main.go", "", false},
		{"empty file path", "/repo", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, ok := relativeToRepo(tt.repoRoot, tt.filePath)
			if ok != tt.wantOK || rel != tt.wantRel {
				t.Errorf("relativeToRepo(%q, %q) = (%q, %v); want (%q, %v)", tt.repoRoot, tt.filePath, rel, ok, tt.wantRel, tt.wantOK)
			}
		})
	}
}
