package syncserver

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const schemaVersion = 1

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (
		version INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS files (
		path            TEXT PRIMARY KEY,
		content         BLOB,
		content_hash    TEXT,
		size            INTEGER NOT NULL DEFAULT 0,
		revision        INTEGER NOT NULL,
		deleted         INTEGER NOT NULL DEFAULT 0,
		modified_by     TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS file_revisions (
		path            TEXT NOT NULL,
		revision        INTEGER NOT NULL,
		content         BLOB,
		content_hash    TEXT,
		size            INTEGER NOT NULL DEFAULT 0,
		deleted         INTEGER NOT NULL DEFAULT 0,
		modified_by     TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		PRIMARY KEY (path, revision)
	);`,
	`CREATE TABLE IF NOT EXISTS users (
		username        TEXT PRIMARY KEY,
		password_hash   TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token           TEXT PRIMARY KEY,
		username        TEXT NOT NULL REFERENCES users(username),
		created_at      TEXT NOT NULL,
		last_seen_at    TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS conflicts (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		path            TEXT NOT NULL,
		kind            TEXT NOT NULL,
		base_revision   INTEGER,
		local_content   BLOB NOT NULL,
		local_hash      TEXT NOT NULL,
		local_username  TEXT NOT NULL,
		remote_revision INTEGER NOT NULL,
		remote_hash     TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'open',
		resolution      TEXT,
		resolved_by     TEXT,
		resolved_at     TEXT,
		created_at      TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS pending_materializations (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		path            TEXT NOT NULL,
		revision        INTEGER NOT NULL,
		op              TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS commit_batches (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at      TEXT NOT NULL,
		committed_at    TEXT,
		git_commit_sha  TEXT,
		paths_json      TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'pending'
	);`,
	`CREATE INDEX IF NOT EXISTS idx_file_revisions_path ON file_revisions(path);`,
	`CREATE INDEX IF NOT EXISTS idx_conflicts_status ON conflicts(status);`,
	`CREATE INDEX IF NOT EXISTS idx_files_deleted ON files(deleted);`,
}

// OpenDB opens (creating if necessary) the server's SQLite database at path
// and applies any pending schema migrations.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SQLite only supports one writer at a time; serialize at the
	// database/sql level so concurrent handlers don't race on
	// SQLITE_BUSY beyond what busy_timeout absorbs.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w\nstatement: %s", err, stmt)
		}
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		return fmt.Errorf("read schema_meta: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`INSERT INTO schema_meta (version) VALUES (?)`, schemaVersion); err != nil {
			return fmt.Errorf("seed schema_meta: %w", err)
		}
	}
	return nil
}
