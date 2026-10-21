package syncserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/synccommon"
)

// ErrNotFound is returned when a path has no row in the files table at all
// (never existed).
var ErrNotFound = errors.New("path not found")

// ErrStaleBase is returned when a caller's base_revision no longer matches
// the current server revision for a path.
var ErrStaleBase = errors.New("stale base revision")

// FileRecord mirrors a row of the files table.
type FileRecord struct {
	Path       string
	Content    []byte
	Hash       string
	Size       int64
	Revision   int64
	Deleted    bool
	ModifiedBy string
	CreatedAt  string
	UpdatedAt  string
}

// Store wraps the canonical files/revisions persistence layer. All mutating
// methods run inside a single SQLite transaction (BEGIN IMMEDIATE) so
// content+hash+revision+audit-row changes are atomic: on any error the
// transaction rolls back and no partial write is visible.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Get returns the current record for path, or ErrNotFound if no row exists
// (including for paths that were never created).
func (s *Store) Get(ctx context.Context, path string) (*FileRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT path, content, content_hash, size, revision, deleted, modified_by, created_at, updated_at
		FROM files WHERE path = ?`, path)
	rec, err := scanFileRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return rec, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFileRecord(row rowScanner) (*FileRecord, error) {
	var r FileRecord
	var content []byte
	var hash sql.NullString
	var deleted int
	if err := row.Scan(&r.Path, &content, &hash, &r.Size, &r.Revision, &deleted, &r.ModifiedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Content = content
	r.Hash = hash.String
	r.Deleted = deleted != 0
	return &r, nil
}

// DeleteStats counts live files and the tombstones username has made since
// the given timestamp, for handleDeleteFile's mass-delete guard.
// ponytail: updated_at is RFC3339Nano with trimmed fractions, so string
// comparison is off by up to a second at the cutoff; irrelevant against
// massDeleteWindow.
func (s *Store) DeleteStats(ctx context.Context, username, since string) (live, recent int, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(deleted=0), 0), COALESCE(SUM(deleted=1 AND modified_by=? AND updated_at>=?), 0)
		FROM files`, username, since).Scan(&live, &recent)
	return live, recent, err
}

// Manifest returns every known path (including tombstones), for GET
// /manifest.
func (s *Store) Manifest(ctx context.Context) ([]FileRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, NULL as content, content_hash, size, revision, deleted, modified_by, created_at, updated_at
		FROM files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FileRecord
	for rows.Next() {
		rec, err := scanFileRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// Put creates or updates path. baseRevision is nil for a brand-new path;
// if the path already exists, baseRevision must match the current
// revision or ErrStaleBase is returned (with the current record attached
// via the returned error being wrapped — callers should re-fetch via Get
// for full detail). A nil baseRevision over a tombstone is a re-create,
// not a conflict: there's no live content to clobber, and a client that
// pulled the delete (or never saw the path) has no base to offer.
// Returns the new record on success.
func (s *Store) Put(ctx context.Context, path string, content []byte, baseRevision *int64, username string) (*FileRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	cur, err := txGet(tx, path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	exists := err == nil

	if exists {
		recreate := baseRevision == nil && cur.Deleted
		if !recreate && (baseRevision == nil || *baseRevision != cur.Revision) {
			return nil, ErrStaleBase
		}
	} else if baseRevision != nil {
		// Client thought a revision existed but it doesn't (e.g. deleted
		// then never seen). Treat as stale rather than silently creating.
		return nil, ErrStaleBase
	}

	var prev *FileRecord
	if exists {
		prev = cur
	}
	rec, err := txWrite(ctx, tx, path, content, prev, username)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return rec, nil
}

// txWrite stores content as path's next revision (revision 1 when cur is
// nil, i.e. the path never existed), including its audit row.
func txWrite(ctx context.Context, tx *sql.Tx, path string, content []byte, cur *FileRecord, username string) (*FileRecord, error) {
	var newRevision int64 = 1
	hash := synccommon.HashBytes(content)
	ts := now()
	createdAt := ts
	if cur != nil {
		newRevision = cur.Revision + 1
		createdAt = cur.CreatedAt
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (path, content, content_hash, size, revision, deleted, modified_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			content=excluded.content, content_hash=excluded.content_hash, size=excluded.size,
			revision=excluded.revision, deleted=0, modified_by=excluded.modified_by, updated_at=excluded.updated_at`,
		path, content, hash, len(content), newRevision, username, createdAt, ts); err != nil {
		return nil, fmt.Errorf("upsert file: %w", err)
	}

	if err := txInsertRevision(ctx, tx, path, newRevision, content, hash, int64(len(content)), false, username, ts); err != nil {
		return nil, err
	}

	return &FileRecord{
		Path: path, Content: content, Hash: hash, Size: int64(len(content)),
		Revision: newRevision, Deleted: false, ModifiedBy: username, CreatedAt: createdAt, UpdatedAt: ts,
	}, nil
}

// Delete tombstones path. baseRevision must match the current revision.
// Deleting an already-deleted path at the matching revision is an
// idempotent no-op that returns the existing tombstone record.
func (s *Store) Delete(ctx context.Context, path string, baseRevision int64, username string) (*FileRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	cur, err := txGet(tx, path)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if cur.Deleted && cur.Revision == baseRevision {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return cur, nil
	}

	if cur.Revision != baseRevision {
		return nil, ErrStaleBase
	}

	rec, err := txTombstone(ctx, tx, cur, username)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return rec, nil
}

// txTombstone deletes cur's path as its next revision, including its audit
// row.
func txTombstone(ctx context.Context, tx *sql.Tx, cur *FileRecord, username string) (*FileRecord, error) {
	newRevision := cur.Revision + 1
	ts := now()

	if _, err := tx.ExecContext(ctx, `
		UPDATE files SET content=NULL, content_hash=NULL, size=0, revision=?, deleted=1, modified_by=?, updated_at=?
		WHERE path=?`, newRevision, username, ts, cur.Path); err != nil {
		return nil, fmt.Errorf("tombstone file: %w", err)
	}

	if err := txInsertRevision(ctx, tx, cur.Path, newRevision, nil, "", 0, true, username, ts); err != nil {
		return nil, err
	}

	return &FileRecord{
		Path: cur.Path, Revision: newRevision, Deleted: true, ModifiedBy: username, CreatedAt: cur.CreatedAt, UpdatedAt: ts,
	}, nil
}

func txGet(tx *sql.Tx, path string) (*FileRecord, error) {
	row := tx.QueryRow(`
		SELECT path, content, content_hash, size, revision, deleted, modified_by, created_at, updated_at
		FROM files WHERE path = ?`, path)
	return scanFileRecord(row)
}

func txInsertRevision(ctx context.Context, tx *sql.Tx, path string, revision int64, content []byte, hash string, size int64, deleted bool, username, ts string) error {
	deletedInt := 0
	if deleted {
		deletedInt = 1
	}
	var hashVal any = hash
	if hash == "" {
		hashVal = nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO file_revisions (path, revision, content, content_hash, size, deleted, modified_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		path, revision, content, hashVal, size, deletedInt, username, ts)
	if err != nil {
		return fmt.Errorf("insert revision audit row: %w", err)
	}
	return nil
}

// RevisionContent returns the content bytes stored for path at a specific
// revision (used for 3-way merge BASE lookup).
func (s *Store) RevisionContent(ctx context.Context, path string, revision int64) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT content FROM file_revisions WHERE path = ? AND revision = ?`, path, revision).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return content, err
}

// LatestContentRevision returns the most recent revision of path that
// carried content, i.e. skipping tombstones. This is how a resolved
// handoff's final canonical content stays readable after its active path has
// been tombstoned: the content is retained in file_revisions (and in the
// server's git history) even though the path is no longer materialized into
// client workspaces.
func (s *Store) LatestContentRevision(ctx context.Context, path string) ([]byte, int64, error) {
	var content []byte
	var revision int64
	err := s.db.QueryRowContext(ctx, `
		SELECT content, revision FROM file_revisions
		WHERE path = ? AND deleted = 0
		ORDER BY revision DESC LIMIT 1`, path).Scan(&content, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	return content, revision, err
}
