package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB for now (keeps options open for later).
type DB struct {
	SQL *sql.DB
}

func Open(sqlitePath string) (*DB, error) {
	// modernc sqlite DSN: "file:<path>?_pragma=..."
	// Keep it simple and apply pragmas explicitly after open.
	dsn := fmt.Sprintf("file:%s", sqlitePath)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Basic health check
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// Pragmas (sane defaults for a small web backend)
	// WAL helps concurrency; foreign_keys for referential integrity; busy_timeout avoids "database is locked".
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}

	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("apply pragma (%s): %w", p, err)
		}
	}

	return &DB{SQL: sqlDB}, nil
}

func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

// Migrate creates tables if they do not exist.
func (d *DB) Migrate() error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("db not initialized")
	}

	stmts := []string{
		`
CREATE TABLE IF NOT EXISTS comments (
  id            TEXT PRIMARY KEY,
  site_id       TEXT NOT NULL,
  entry_id      TEXT,
  post_path     TEXT NOT NULL,
  parent_id     TEXT,
  status        TEXT NOT NULL,
  author        TEXT NOT NULL,
  email         TEXT NOT NULL,
	author_url    TEXT,
  body          TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  approved_at   INTEGER,
  rejected_at   INTEGER,

  FOREIGN KEY(parent_id) REFERENCES comments(id) ON DELETE CASCADE
);

`,
		`CREATE INDEX IF NOT EXISTS idx_comments_site_status_created ON comments(site_id, status, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_comments_site_post_created   ON comments(site_id, post_path, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_comments_site_parent_created ON comments(site_id, parent_id, created_at);`,
		`,
CREATE TABLE IF NOT EXISTS pipeline_runs (
  id                  INTEGER PRIMARY KEY,
  site_id             TEXT NOT NULL,
  trigger_comment_id  TEXT,

  state               TEXT NOT NULL,        -- queued|running|success|failed
  step                TEXT,                -- checkout|hugo|commit|push
  error_message       TEXT,

  created_at          INTEGER NOT NULL,
  started_at          INTEGER,
  finished_at         INTEGER
);
`,
		`CREATE INDEX IF NOT EXISTS idx_runs_site_created  ON pipeline_runs(site_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_state_created ON pipeline_runs(state, created_at);`,
	}

	for _, s := range stmts {
		if _, err := d.SQL.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	log.Println("sqlite migration done")
	return nil
}
