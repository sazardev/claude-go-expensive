package expensive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// copyFixture copies testdata/sample_session.jsonl into
// <dir>/<project>/<session>.jsonl, mirroring the layout Claude Code writes
// under ~/.claude/projects.
func copyFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/sample_session.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	projectDir := filepath.Join(dir, "-home-dev-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sessionPath := filepath.Join(projectDir, "11111111-1111-1111-1111-111111111111.jsonl")
	if err := os.WriteFile(sessionPath, data, 0o644); err != nil {
		t.Fatalf("write fixture copy: %v", err)
	}
	return sessionPath
}

func TestTrackerIngestFileAndReports(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sessionPath := copyFixture(t, dir)

	tr, err := Open(filepath.Join(dir, "expensive.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer tr.Close()

	ingested, err := tr.IngestFile(ctx, sessionPath)
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	if !ingested {
		t.Fatalf("IngestFile() ingested = false; want true")
	}

	// Re-ingesting the same, unchanged file is a no-op.
	ingested, err = tr.IngestFile(ctx, sessionPath)
	if err != nil {
		t.Fatalf("second IngestFile() error = %v", err)
	}
	if ingested {
		t.Errorf("IngestFile() ingested = true on unchanged re-ingest; want false")
	}

	summary, err := tr.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Sessions != 1 || summary.Prompts != 2 {
		t.Fatalf("Summary() = %+v; want 1 session, 2 prompts", summary)
	}

	byProject, err := tr.CostByProject(ctx)
	if err != nil {
		t.Fatalf("CostByProject() error = %v", err)
	}
	if len(byProject) != 1 || byProject[0].Project != "project" {
		t.Fatalf("CostByProject() = %+v; want project named %q", byProject, "project")
	}

	byFile, err := tr.CostByFile(ctx, 10)
	if err != nil {
		t.Fatalf("CostByFile() error = %v", err)
	}
	if len(byFile) != 2 {
		t.Fatalf("CostByFile() returned %d rows; want 2 (main.go, main_test.go)", len(byFile))
	}
}

func TestTrackerIngestDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	copyFixture(t, dir)

	tr, err := Open(filepath.Join(t.TempDir(), "expensive.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer tr.Close()

	stats, err := tr.IngestDir(ctx, dir)
	if err != nil {
		t.Fatalf("IngestDir() error = %v", err)
	}
	if stats.Sessions != 1 || stats.Skipped != 0 || len(stats.Errors) != 0 {
		t.Fatalf("IngestDir() stats = %+v; want 1 session, 0 skipped, 0 errors", stats)
	}

	stats, err = tr.IngestDir(ctx, dir)
	if err != nil {
		t.Fatalf("second IngestDir() error = %v", err)
	}
	if stats.Sessions != 0 || stats.Skipped != 1 {
		t.Fatalf("second IngestDir() stats = %+v; want 0 new, 1 skipped", stats)
	}
}
