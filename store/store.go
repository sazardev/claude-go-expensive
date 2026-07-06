// Package store persists parsed Claude Code sessions to SQLite and reports
// cost/usage across project, repo, folder, file, session, and prompt.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"path"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store is a handle to the SQLite database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite database at dbPath and applies the schema.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	// A single connection keeps writer/reader ordering simple; this package
	// targets local, single-process usage, not concurrent server access.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveSession persists a fully-parsed session under projectName, replacing
// any prior data for the same sess.SourcePath. It returns skipped=true
// without writing if a session at that source path with the same file size
// and modification time was already ingested.
func (s *Store) SaveSession(ctx context.Context, projectName string, sess Session) (skipped bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var existingSize, existingMTime int64
	err = tx.QueryRowContext(ctx,
		`SELECT file_size, file_mtime FROM sessions WHERE source_path = ?`, sess.SourcePath,
	).Scan(&existingSize, &existingMTime)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// not previously ingested
	case err != nil:
		return false, fmt.Errorf("check existing session: %w", err)
	default:
		if existingSize == sess.FileSize && existingMTime == sess.FileModTime.Unix() {
			return true, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE source_path = ?`, sess.SourcePath); err != nil {
			return false, fmt.Errorf("clear stale session: %w", err)
		}
	}

	projectID, err := upsertNamed(ctx, tx, "projects", "name", projectName)
	if err != nil {
		return false, fmt.Errorf("upsert project %q: %w", projectName, err)
	}
	repoID, err := upsertRepo(ctx, tx, projectID, sess.RepoRootPath)
	if err != nil {
		return false, fmt.Errorf("upsert repo %q: %w", sess.RepoRootPath, err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (
			id, repo_id, source_path, file_size, file_mtime, git_branch,
			started_at, ended_at, input_tokens, output_tokens,
			cache_creation_5m_tokens, cache_creation_1h_tokens, cache_read_tokens,
			web_searches, cost_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, repoID, sess.SourcePath, sess.FileSize, sess.FileModTime.Unix(), sess.GitBranch,
		sess.StartedAt, sess.EndedAt, sess.InputTokens, sess.OutputTokens,
		sess.CacheCreation5mTokens, sess.CacheCreation1hTokens, sess.CacheReadTokens,
		sess.WebSearches, sess.CostUSD,
	)
	if err != nil {
		return false, fmt.Errorf("insert session: %w", err)
	}

	for _, p := range sess.Prompts {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO prompts (
				session_id, uuid, seq, text, model, speed, created_at,
				input_tokens, output_tokens, cache_creation_5m_tokens, cache_creation_1h_tokens,
				cache_read_tokens, web_searches, cost_usd
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sess.ID, nullIfEmpty(p.UUID), p.Seq, p.Text, p.Model, nullIfEmpty(p.Speed), p.CreatedAt,
			p.InputTokens, p.OutputTokens, p.CacheCreation5mTokens, p.CacheCreation1hTokens,
			p.CacheReadTokens, p.WebSearches, p.CostUSD,
		)
		if err != nil {
			return false, fmt.Errorf("insert prompt: %w", err)
		}
		promptID, err := res.LastInsertId()
		if err != nil {
			return false, fmt.Errorf("prompt id: %w", err)
		}

		for _, tc := range p.ToolCalls {
			var fileID sql.NullInt64
			if tc.FilePath != "" {
				folderPath := path.Dir(tc.FilePath)
				if folderPath == "." {
					folderPath = ""
				}
				fID, err := upsertFolder(ctx, tx, repoID, folderPath)
				if err != nil {
					return false, fmt.Errorf("upsert folder %q: %w", folderPath, err)
				}
				fileID, err = upsertFile(ctx, tx, fID, tc.FilePath)
				if err != nil {
					return false, fmt.Errorf("upsert file %q: %w", tc.FilePath, err)
				}
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tool_calls (prompt_id, file_id, tool_name, created_at) VALUES (?, ?, ?, ?)`,
				promptID, fileID, tc.ToolName, tc.CreatedAt,
			); err != nil {
				return false, fmt.Errorf("insert tool call: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return false, nil
}

func upsertNamed(ctx context.Context, tx *sql.Tx, table, column, value string) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s(%s) VALUES (?) ON CONFLICT(%s) DO NOTHING`, table, column, column),
		value,
	); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE %s = ?`, table, column), value).Scan(&id)
	return id, err
}

func upsertRepo(ctx context.Context, tx *sql.Tx, projectID int64, rootPath string) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO repos(project_id, root_path) VALUES (?, ?) ON CONFLICT(root_path) DO NOTHING`,
		projectID, rootPath,
	); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM repos WHERE root_path = ?`, rootPath).Scan(&id)
	return id, err
}

func upsertFolder(ctx context.Context, tx *sql.Tx, repoID int64, folderPath string) (sql.NullInt64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO folders(repo_id, path) VALUES (?, ?) ON CONFLICT(repo_id, path) DO NOTHING`,
		repoID, folderPath,
	); err != nil {
		return sql.NullInt64{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM folders WHERE repo_id = ? AND path = ?`, repoID, folderPath,
	).Scan(&id); err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

func upsertFile(ctx context.Context, tx *sql.Tx, folderID sql.NullInt64, filePath string) (sql.NullInt64, error) {
	if !folderID.Valid {
		return sql.NullInt64{}, fmt.Errorf("upsertFile: invalid folder id")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO files(folder_id, path) VALUES (?, ?) ON CONFLICT(folder_id, path) DO NOTHING`,
		folderID.Int64, filePath,
	); err != nil {
		return sql.NullInt64{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM files WHERE folder_id = ? AND path = ?`, folderID.Int64, filePath,
	).Scan(&id); err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
