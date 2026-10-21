// Package syncclient implements the report-rex sync client: local
// workspace scanning, the .report-rex/sync.db local metadata store, HTTP/WS
// calls against the sync server, and the reconciliation engine that drives
// push/pull/conflict handling.
package syncclient

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// LocalDir is the hidden directory, relative to a workspace root, holding
// client-local sync metadata. It is never itself synchronized.
const LocalDir = ".report-rex"

const localDBFile = "sync.db"

var localMigrations = []string{
	`CREATE TABLE IF NOT EXISTS sync_state (
		path                        TEXT PRIMARY KEY,
		last_known_remote_revision  INTEGER,
		last_known_remote_hash      TEXT,
		last_synced_local_hash      TEXT,
		updated_at                  TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS local_conflicts (
		server_conflict_id  INTEGER PRIMARY KEY,
		path                TEXT NOT NULL,
		kind                TEXT NOT NULL,
		remote_revision     INTEGER NOT NULL,
		fetched_at          TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS session (
		id            INTEGER PRIMARY KEY CHECK (id = 1),
		host          TEXT NOT NULL,
		port          INTEGER NOT NULL,
		username      TEXT NOT NULL,
		token         TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);`,
}

// LocalStore wraps the client's .report-rex/sync.db.
type LocalStore struct {
	db *sql.DB
}

// OpenLocalStore opens (creating if necessary) the sync metadata DB under
// workspaceDir/.report-rex/sync.db.
func OpenLocalStore(workspaceDir string) (*LocalStore, error) {
	dir := filepath.Join(workspaceDir, LocalDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	dbPath := filepath.Join(dir, localDBFile)
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open local db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range localMigrations {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate local db: %w", err)
		}
	}
	return &LocalStore{db: db}, nil
}

func (s *LocalStore) Close() error { return s.db.Close() }

// SyncStateRow mirrors a row of the sync_state table.
type SyncStateRow struct {
	Path                    string
	LastKnownRemoteRevision *int64
	LastKnownRemoteHash     string
	LastSyncedLocalHash     string
}

func (s *LocalStore) Get(path string) (*SyncStateRow, bool, error) {
	row := s.db.QueryRow(`
		SELECT path, last_known_remote_revision, last_known_remote_hash, last_synced_local_hash
		FROM sync_state WHERE path = ?`, path)
	var r SyncStateRow
	var rev sql.NullInt64
	var remoteHash, localHash sql.NullString
	if err := row.Scan(&r.Path, &rev, &remoteHash, &localHash); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if rev.Valid {
		r.LastKnownRemoteRevision = &rev.Int64
	}
	r.LastKnownRemoteHash = remoteHash.String
	r.LastSyncedLocalHash = localHash.String
	return &r, true, nil
}

func (s *LocalStore) All() (map[string]SyncStateRow, error) {
	rows, err := s.db.Query(`SELECT path, last_known_remote_revision, last_known_remote_hash, last_synced_local_hash FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SyncStateRow{}
	for rows.Next() {
		var r SyncStateRow
		var rev sql.NullInt64
		var remoteHash, localHash sql.NullString
		if err := rows.Scan(&r.Path, &rev, &remoteHash, &localHash); err != nil {
			return nil, err
		}
		if rev.Valid {
			r.LastKnownRemoteRevision = &rev.Int64
		}
		r.LastKnownRemoteHash = remoteHash.String
		r.LastSyncedLocalHash = localHash.String
		out[r.Path] = r
	}
	return out, rows.Err()
}

// Upsert records the last-known-synced state for path after a successful
// push or pull.
func (s *LocalStore) Upsert(path string, remoteRevision int64, remoteHash, localHash, ts string) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_state (path, last_known_remote_revision, last_known_remote_hash, last_synced_local_hash, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			last_known_remote_revision=excluded.last_known_remote_revision,
			last_known_remote_hash=excluded.last_known_remote_hash,
			last_synced_local_hash=excluded.last_synced_local_hash,
			updated_at=excluded.updated_at`,
		path, remoteRevision, remoteHash, localHash, ts)
	return err
}

// Forget removes a path's sync_state row (used after a path is deleted on
// both sides so it stops being tracked).
func (s *LocalStore) Forget(path string) error {
	_, err := s.db.Exec(`DELETE FROM sync_state WHERE path = ?`, path)
	return err
}

// MarkPendingMerge records that `rex resolve --merge` wrote conflict
// markers for conflict id, built against remote revision remoteRev, so the
// next --merge run submits the user's hand-edited file instead of merging
// (and re-writing markers over their work) all over again.
func (s *LocalStore) MarkPendingMerge(id int64, path, kind string, remoteRev int64) error {
	_, err := s.db.Exec(`
		INSERT INTO local_conflicts (server_conflict_id, path, kind, remote_revision, fetched_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(server_conflict_id) DO UPDATE SET remote_revision=excluded.remote_revision, fetched_at=excluded.fetched_at`,
		id, path, kind, remoteRev, now())
	return err
}

// PendingMerge returns the remote revision a pending hand merge for
// conflict id was built against, if there is one.
func (s *LocalStore) PendingMerge(id int64) (remoteRev int64, ok bool) {
	if err := s.db.QueryRow(`SELECT remote_revision FROM local_conflicts WHERE server_conflict_id = ?`, id).Scan(&remoteRev); err != nil {
		return 0, false
	}
	return remoteRev, true
}

func (s *LocalStore) ClearPendingMerge(id int64) error {
	_, err := s.db.Exec(`DELETE FROM local_conflicts WHERE server_conflict_id = ?`, id)
	return err
}

// SaveSession caches the bearer token for reuse across `sync`/`conflicts`/
// `resolve` invocations against the same workspace.
func (s *LocalStore) SaveSession(host string, port int, username, token, ts string) error {
	_, err := s.db.Exec(`
		INSERT INTO session (id, host, port, username, token, updated_at) VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET host=excluded.host, port=excluded.port,
			username=excluded.username, token=excluded.token, updated_at=excluded.updated_at`,
		host, port, username, token, ts)
	return err
}
