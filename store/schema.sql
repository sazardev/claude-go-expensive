CREATE TABLE IF NOT EXISTS projects (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS repos (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    root_path  TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS folders (
    id      INTEGER PRIMARY KEY,
    repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    path    TEXT NOT NULL, -- relative to repo root; "" is the repo root itself
    UNIQUE (repo_id, path)
);

CREATE TABLE IF NOT EXISTS files (
    id        INTEGER PRIMARY KEY,
    folder_id INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    path      TEXT NOT NULL, -- relative to repo root
    UNIQUE (folder_id, path)
);

CREATE TABLE IF NOT EXISTS sessions (
    id                       TEXT PRIMARY KEY,
    repo_id                  INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    source_path              TEXT NOT NULL UNIQUE,
    file_size                INTEGER NOT NULL DEFAULT 0,
    file_mtime               INTEGER NOT NULL DEFAULT 0, -- unix seconds
    git_branch               TEXT,
    started_at               DATETIME,
    ended_at                 DATETIME,
    input_tokens             INTEGER NOT NULL DEFAULT 0,
    output_tokens            INTEGER NOT NULL DEFAULT 0,
    cache_creation_5m_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_1h_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens        INTEGER NOT NULL DEFAULT 0,
    web_searches             INTEGER NOT NULL DEFAULT 0,
    cost_usd                 REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS prompts (
    id                       INTEGER PRIMARY KEY,
    session_id               TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    uuid                     TEXT,
    seq                      INTEGER NOT NULL,
    text                     TEXT,
    model                    TEXT,
    speed                    TEXT,
    created_at               DATETIME,
    input_tokens             INTEGER NOT NULL DEFAULT 0,
    output_tokens            INTEGER NOT NULL DEFAULT 0,
    cache_creation_5m_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_1h_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens        INTEGER NOT NULL DEFAULT 0,
    web_searches             INTEGER NOT NULL DEFAULT 0,
    cost_usd                 REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tool_calls (
    id         INTEGER PRIMARY KEY,
    prompt_id  INTEGER NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    file_id    INTEGER REFERENCES files(id) ON DELETE SET NULL,
    tool_name  TEXT NOT NULL,
    created_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_repos_project        ON repos(project_id);
CREATE INDEX IF NOT EXISTS idx_folders_repo         ON folders(repo_id);
CREATE INDEX IF NOT EXISTS idx_files_folder         ON files(folder_id);
CREATE INDEX IF NOT EXISTS idx_sessions_repo        ON sessions(repo_id);
CREATE INDEX IF NOT EXISTS idx_prompts_session      ON prompts(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_prompt    ON tool_calls(prompt_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_file      ON tool_calls(file_id);
