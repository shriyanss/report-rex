package syncserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ConflictKind mirrors syncproto.ConflictKind as plain strings to avoid a
// server->proto import for internal storage.
type ConflictKind string

const (
	ConflictKindDivergentEdit  ConflictKind = "divergent_edit"
	ConflictKindInitialUnknown ConflictKind = "initial_unknown_base"
	ConflictKindDeleteEdit     ConflictKind = "delete_edit"
)

// Conflict mirrors a row of the conflicts table.
type Conflict struct {
	ID             int64
	Path           string
	Kind           ConflictKind
	BaseRevision   *int64
	LocalContent   []byte
	LocalHash      string
	LocalUsername  string
	RemoteRevision int64
	RemoteHash     string
	Status         string
	Resolution     string
	CreatedAt      string
}

var ErrConflictNotFound = errors.New("conflict not found")
var ErrConflictAlreadyResolved = errors.New("conflict already resolved")

// CreateConflict records a rejected/divergent change as an open conflict.
// A (path, user) pair has at most one open conflict: if one already exists
// it's updated in place with the latest local content and remote state,
// and created is false. Without this, a user who keeps editing a
// conflicted file (or restarts `rex sync`) files a fresh conflict on every
// reconcile pass -- thousands under load, and `rex resolve` then can't pick
// one without --id.
func (s *Store) CreateConflict(ctx context.Context, c Conflict) (id int64, created bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		SELECT id FROM conflicts WHERE path = ? AND local_username = ? AND status = 'open'
		ORDER BY id LIMIT 1`, c.Path, c.LocalUsername).Scan(&id)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
			UPDATE conflicts SET kind=?, base_revision=?, local_content=?, local_hash=?,
				remote_revision=?, remote_hash=? WHERE id = ?`,
			string(c.Kind), c.BaseRevision, c.LocalContent, c.LocalHash,
			c.RemoteRevision, c.RemoteHash, id); err != nil {
			return 0, false, fmt.Errorf("update conflict: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.ExecContext(ctx, `
			INSERT INTO conflicts (path, kind, base_revision, local_content, local_hash, local_username,
				remote_revision, remote_hash, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'open', ?)`,
			c.Path, string(c.Kind), c.BaseRevision, c.LocalContent, c.LocalHash, c.LocalUsername,
			c.RemoteRevision, c.RemoteHash, now())
		if err != nil {
			return 0, false, fmt.Errorf("insert conflict: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, false, err
		}
		created = true
	default:
		return 0, false, err
	}
	return id, created, tx.Commit()
}

// ListOpenConflicts returns all conflicts with status='open'.
func (s *Store) ListOpenConflicts(ctx context.Context) ([]Conflict, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, kind, remote_revision, local_username, created_at FROM conflicts
		WHERE status = 'open' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Conflict
	for rows.Next() {
		var c Conflict
		if err := rows.Scan(&c.ID, &c.Path, &c.Kind, &c.RemoteRevision, &c.LocalUsername, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConflict returns full detail for one conflict.
func (s *Store) GetConflict(ctx context.Context, id int64) (*Conflict, error) {
	var c Conflict
	var base sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, path, kind, base_revision, local_content, local_hash, local_username,
			remote_revision, remote_hash, status, COALESCE(resolution, ''), created_at
		FROM conflicts WHERE id = ?`, id).Scan(
		&c.ID, &c.Path, &c.Kind, &base, &c.LocalContent, &c.LocalHash, &c.LocalUsername,
		&c.RemoteRevision, &c.RemoteHash, &c.Status, &c.Resolution, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflictNotFound
	}
	if err != nil {
		return nil, err
	}
	if base.Valid {
		c.BaseRevision = &base.Int64
	}
	return &c, nil
}

// MarkResolved records the resolution decision on a conflict row. It does
// not itself perform the file mutation (callers do that via Put/Delete and
// then call MarkResolved).
func (s *Store) MarkResolved(ctx context.Context, id int64, resolution, resolvedBy string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE conflicts SET status='resolved', resolution=?, resolved_by=?, resolved_at=?
		WHERE id = ? AND status = 'open'`, resolution, resolvedBy, now(), id)
	if err != nil {
		return fmt.Errorf("mark conflict resolved: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either it doesn't exist or was already resolved; distinguish.
		if _, getErr := s.GetConflict(ctx, id); getErr != nil {
			return getErr
		}
		return ErrConflictAlreadyResolved
	}
	return nil
}
