// Package expensive tracks Claude Code token usage and cost by parsing its
// session transcripts (~/.claude/projects/**/*.jsonl) into a SQLite database,
// organized by project, repo, folder, file, session, and prompt.
package expensive

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sazardev/claude-go-expensive/store"
	"github.com/sazardev/claude-go-expensive/transcript"
)

// Tracker ingests Claude Code session transcripts into a SQLite database and
// reports token usage and cost.
type Tracker struct {
	store *store.Store
}

// Open creates or opens the SQLite database at dbPath.
func Open(dbPath string) (*Tracker, error) {
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Tracker{store: s}, nil
}

// Close releases the underlying database connection.
func (t *Tracker) Close() error {
	return t.store.Close()
}

// Stats summarizes the result of an IngestDir run.
type Stats struct {
	Sessions int // newly ingested or re-ingested after a change
	Skipped  int // already up to date
	Errors   []error
}

// IngestFile parses and stores a single session JSONL transcript. It
// returns ingested=false without error if the file was already ingested
// unchanged (same size and modification time).
func (t *Tracker) IngestFile(ctx context.Context, path string) (ingested bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	entries, err := transcript.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	sess, toolUseSeq, ok := buildSession(entries, path, info.Size(), info.ModTime())
	if !ok {
		return false, fmt.Errorf("no session id found in %s", path)
	}
	// Best-effort, like transcript.Read's tolerance of malformed lines: a
	// subagent transcript that fails to read just means that subagent's
	// usage is undercounted, not that the whole session fails to save.
	mergeSubagents(&sess, toolUseSeq, path)

	projectName := detectProjectName(sess.RepoRootPath)
	if projectName == "" || projectName == "." || projectName == string(filepath.Separator) {
		projectName = filepath.Base(filepath.Dir(path))
	}

	skipped, err := t.store.SaveSession(ctx, projectName, sess)
	if err != nil {
		return false, fmt.Errorf("save session %s: %w", sess.ID, err)
	}
	return !skipped, nil
}

// IngestDir walks root recursively and ingests every top-level *.jsonl
// session transcript found — the layout Claude Code uses under
// ~/.claude/projects. Per-subagent transcripts (<session>/subagents/*.jsonl)
// are skipped here; IngestFile folds them into their parent session instead
// of ingesting them as standalone sessions (they share the parent's session
// ID). A file that fails to ingest is recorded in Stats.Errors rather than
// aborting the walk.
func (t *Tracker) IngestDir(ctx context.Context, root string) (Stats, error) {
	var stats Stats
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			stats.Errors = append(stats.Errors, err)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") || isSubagentTranscript(path) {
			return nil
		}
		ingested, err := t.IngestFile(ctx, path)
		if err != nil {
			stats.Errors = append(stats.Errors, err)
			return nil
		}
		if ingested {
			stats.Sessions++
		} else {
			stats.Skipped++
		}
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("walk %s: %w", root, err)
	}
	return stats, nil
}

// DefaultLogsDir returns the directory where Claude Code stores session
// transcripts (~/.claude/projects).
func DefaultLogsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Summary is an at-a-glance rollup across everything ingested.
func (t *Tracker) Summary(ctx context.Context) (store.Summary, error) {
	return t.store.Summary(ctx)
}

// CostByProject aggregates token usage and cost per project, most expensive
// first.
func (t *Tracker) CostByProject(ctx context.Context) ([]store.ProjectCost, error) {
	return t.store.CostByProject(ctx)
}

// CostByRepo aggregates token usage and cost per repo, most expensive first.
func (t *Tracker) CostByRepo(ctx context.Context) ([]store.RepoCost, error) {
	return t.store.CostByRepo(ctx)
}

// CostBySession lists the most expensive sessions, up to limit rows.
func (t *Tracker) CostBySession(ctx context.Context, limit int) ([]store.SessionCost, error) {
	return t.store.CostBySession(ctx, limit)
}

// CostByFile lists the files most expensive to work on, up to limit rows.
func (t *Tracker) CostByFile(ctx context.Context, limit int) ([]store.FileCost, error) {
	return t.store.CostByFile(ctx, limit)
}

// CostByFolder lists the folders most expensive to work on, up to limit
// rows.
func (t *Tracker) CostByFolder(ctx context.Context, limit int) ([]store.FolderCost, error) {
	return t.store.CostByFolder(ctx, limit)
}
