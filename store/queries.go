package store

import "context"

// Summary is an at-a-glance rollup across everything ingested.
func (s *Store) Summary(ctx context.Context) (Summary, error) {
	var sum Summary
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM projects),
			(SELECT COUNT(*) FROM repos),
			(SELECT COUNT(*) FROM sessions),
			(SELECT COUNT(*) FROM prompts),
			COALESCE((SELECT SUM(input_tokens) FROM sessions), 0),
			COALESCE((SELECT SUM(output_tokens) FROM sessions), 0),
			COALESCE((SELECT SUM(cache_creation_5m_tokens) FROM sessions), 0),
			COALESCE((SELECT SUM(cache_creation_1h_tokens) FROM sessions), 0),
			COALESCE((SELECT SUM(cache_read_tokens) FROM sessions), 0),
			COALESCE((SELECT SUM(web_searches) FROM sessions), 0),
			COALESCE((SELECT SUM(cost_usd) FROM sessions), 0)
	`).Scan(
		&sum.Projects, &sum.Repos, &sum.Sessions, &sum.Prompts,
		&sum.InputTokens, &sum.OutputTokens,
		&sum.CacheCreation5mTokens, &sum.CacheCreation1hTokens, &sum.CacheReadTokens,
		&sum.WebSearches, &sum.CostUSD,
	)
	return sum, err
}

// CostByProject aggregates token usage and cost per project, most expensive
// first.
func (s *Store) CostByProject(ctx context.Context) ([]ProjectCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH session_prompts AS (
			SELECT session_id, COUNT(*) AS n FROM prompts GROUP BY session_id
		)
		SELECT pr.name,
		       COUNT(DISTINCT s.id),
		       COALESCE(SUM(sp.n), 0),
		       COALESCE(SUM(s.input_tokens), 0),
		       COALESCE(SUM(s.output_tokens), 0),
		       COALESCE(SUM(s.cost_usd), 0)
		FROM projects pr
		JOIN repos r ON r.project_id = pr.id
		JOIN sessions s ON s.repo_id = r.id
		LEFT JOIN session_prompts sp ON sp.session_id = s.id
		GROUP BY pr.id
		ORDER BY 6 DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectCost
	for rows.Next() {
		var row ProjectCost
		if err := rows.Scan(&row.Project, &row.Sessions, &row.Prompts, &row.InputTokens, &row.OutputTokens, &row.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CostByRepo aggregates token usage and cost per repo, most expensive first.
func (s *Store) CostByRepo(ctx context.Context) ([]RepoCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH session_prompts AS (
			SELECT session_id, COUNT(*) AS n FROM prompts GROUP BY session_id
		)
		SELECT pr.name, r.root_path,
		       COUNT(DISTINCT s.id),
		       COALESCE(SUM(sp.n), 0),
		       COALESCE(SUM(s.cost_usd), 0)
		FROM repos r
		JOIN projects pr ON pr.id = r.project_id
		JOIN sessions s ON s.repo_id = r.id
		LEFT JOIN session_prompts sp ON sp.session_id = s.id
		GROUP BY r.id
		ORDER BY 5 DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RepoCost
	for rows.Next() {
		var row RepoCost
		if err := rows.Scan(&row.Project, &row.RepoRootPath, &row.Sessions, &row.Prompts, &row.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CostBySession lists the most expensive sessions, up to limit rows.
func (s *Store) CostBySession(ctx context.Context, limit int) ([]SessionCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH session_prompts AS (
			SELECT session_id, COUNT(*) AS n FROM prompts GROUP BY session_id
		)
		SELECT s.id, pr.name, r.root_path, s.started_at, COALESCE(sp.n, 0), s.cost_usd
		FROM sessions s
		JOIN repos r ON r.id = s.repo_id
		JOIN projects pr ON pr.id = r.project_id
		LEFT JOIN session_prompts sp ON sp.session_id = s.id
		ORDER BY s.cost_usd DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionCost
	for rows.Next() {
		var row SessionCost
		if err := rows.Scan(&row.SessionID, &row.Project, &row.RepoRootPath, &row.StartedAt, &row.Prompts, &row.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CostByFile lists the files most expensive to work on, up to limit rows.
// See FileCost for how cost is attributed.
func (s *Store) CostByFile(ctx context.Context, limit int) ([]FileCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pr.name, r.root_path, f.path,
		       COUNT(*) AS tool_calls,
		       COALESCE((
		           SELECT SUM(p.cost_usd) FROM prompts p
		           WHERE p.id IN (SELECT DISTINCT tc2.prompt_id FROM tool_calls tc2 WHERE tc2.file_id = f.id)
		       ), 0) AS cost_usd
		FROM tool_calls tc
		JOIN files f ON f.id = tc.file_id
		JOIN folders fo ON fo.id = f.folder_id
		JOIN repos r ON r.id = fo.repo_id
		JOIN projects pr ON pr.id = r.project_id
		GROUP BY f.id
		ORDER BY cost_usd DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FileCost
	for rows.Next() {
		var row FileCost
		if err := rows.Scan(&row.Project, &row.RepoRootPath, &row.FilePath, &row.ToolCalls, &row.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CostByFolder lists the folders most expensive to work on, up to limit
// rows. See FileCost for how cost is attributed.
func (s *Store) CostByFolder(ctx context.Context, limit int) ([]FolderCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pr.name, r.root_path, fo.path,
		       COUNT(*) AS tool_calls,
		       COALESCE((
		           SELECT SUM(p.cost_usd) FROM prompts p
		           WHERE p.id IN (
		               SELECT DISTINCT tc2.prompt_id FROM tool_calls tc2
		               JOIN files f2 ON f2.id = tc2.file_id
		               WHERE f2.folder_id = fo.id
		           )
		       ), 0) AS cost_usd
		FROM tool_calls tc
		JOIN files f ON f.id = tc.file_id
		JOIN folders fo ON fo.id = f.folder_id
		JOIN repos r ON r.id = fo.repo_id
		JOIN projects pr ON pr.id = r.project_id
		GROUP BY fo.id
		ORDER BY cost_usd DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FolderCost
	for rows.Next() {
		var row FolderCost
		if err := rows.Scan(&row.Project, &row.RepoRootPath, &row.FolderPath, &row.ToolCalls, &row.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
