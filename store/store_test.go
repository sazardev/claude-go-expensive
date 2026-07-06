package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleSession() Session {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	return Session{
		ID:           "sess-1",
		RepoRootPath: "/repo",
		SourcePath:   "/logs/sess-1.jsonl",
		FileSize:     100,
		FileModTime:  now,
		StartedAt:    now,
		EndedAt:      now.Add(time.Minute),
		InputTokens:  1000,
		OutputTokens: 200,
		CostUSD:      0.05,
		Prompts: []Prompt{
			{
				UUID:         "p1",
				Seq:          1,
				Text:         "fix the bug",
				Model:        "claude-opus-4-8",
				CreatedAt:    now,
				InputTokens:  1000,
				OutputTokens: 200,
				CostUSD:      0.05,
				ToolCalls: []ToolCall{
					{ToolName: "Read", FilePath: "main.go", CreatedAt: now},
					{ToolName: "Edit", FilePath: "main.go", CreatedAt: now},
					{ToolName: "Bash", CreatedAt: now}, // no file
				},
			},
		},
	}
}

func TestSaveSessionAndReports(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	skipped, err := s.SaveSession(ctx, "repo", sampleSession())
	if err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if skipped {
		t.Fatalf("SaveSession() skipped = true on first save; want false")
	}

	summary, err := s.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Sessions != 1 || summary.Prompts != 1 {
		t.Fatalf("Summary() = %+v; want 1 session, 1 prompt", summary)
	}
	if summary.CostUSD != 0.05 {
		t.Errorf("Summary().CostUSD = %v; want 0.05", summary.CostUSD)
	}

	byProject, err := s.CostByProject(ctx)
	if err != nil {
		t.Fatalf("CostByProject() error = %v", err)
	}
	if len(byProject) != 1 || byProject[0].Project != "repo" || byProject[0].Sessions != 1 {
		t.Fatalf("CostByProject() = %+v; want one row for project %q", byProject, "repo")
	}

	byFile, err := s.CostByFile(ctx, 10)
	if err != nil {
		t.Fatalf("CostByFile() error = %v", err)
	}
	if len(byFile) != 1 {
		t.Fatalf("CostByFile() returned %d rows; want 1 (only main.go was touched)", len(byFile))
	}
	if byFile[0].FilePath != "main.go" || byFile[0].ToolCalls != 2 {
		t.Errorf("CostByFile()[0] = %+v; want main.go with 2 tool calls", byFile[0])
	}
	if byFile[0].CostUSD != 0.05 {
		t.Errorf("CostByFile()[0].CostUSD = %v; want 0.05 (not double-counted across the 2 tool calls)", byFile[0].CostUSD)
	}

	byFolder, err := s.CostByFolder(ctx, 10)
	if err != nil {
		t.Fatalf("CostByFolder() error = %v", err)
	}
	if len(byFolder) != 1 || byFolder[0].FolderPath != "" {
		t.Fatalf("CostByFolder() = %+v; want one row for the repo root folder", byFolder)
	}
}

func TestSaveSessionIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := sampleSession()

	if _, err := s.SaveSession(ctx, "repo", sess); err != nil {
		t.Fatalf("first SaveSession() error = %v", err)
	}

	skipped, err := s.SaveSession(ctx, "repo", sess)
	if err != nil {
		t.Fatalf("second SaveSession() error = %v", err)
	}
	if !skipped {
		t.Errorf("SaveSession() skipped = false on unchanged re-ingest; want true")
	}

	sess.FileModTime = sess.FileModTime.Add(time.Hour)
	sess.Prompts[0].Text = "fix the bug, take two"
	skipped, err = s.SaveSession(ctx, "repo", sess)
	if err != nil {
		t.Fatalf("third SaveSession() error = %v", err)
	}
	if skipped {
		t.Errorf("SaveSession() skipped = true after mtime change; want false (should re-ingest)")
	}

	summary, err := s.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Sessions != 1 || summary.Prompts != 1 {
		t.Fatalf("Summary() after re-ingest = %+v; want the stale session replaced, not duplicated", summary)
	}
}
