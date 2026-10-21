package syncserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// ErrStalePlan is returned by ApplyRevert when the workspace changed after
// the admin previewed the revert, so the plan they approved is out of date.
var ErrStalePlan = errors.New("workspace changed since the revert was previewed")

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// RevertPlan computes what rolling the workspace back to "right after path
// reached revision" (or, with before, right before it) takes. The target
// point is that revision's file_revisions rowid, which grows in commit order across every path (created_at strings
// don't sort reliably). Each path's target state is its last revision at or
// before that point; none at all means it didn't exist yet. Handoffs are left
// alone: they have their own lifecycle and are not report content.
//
// The returned token hashes the target and every op's current revision, so
// ApplyRevert can refuse a plan the workspace has since moved past.
func (s *Store) RevertPlan(ctx context.Context, q querier, path string, revision int64, before bool) ([]syncproto.RevertOp, string, error) {
	var cutoff int64
	err := q.QueryRowContext(ctx, `SELECT rowid FROM file_revisions WHERE path = ? AND revision = ?`, path, revision).Scan(&cutoff)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if before {
		cutoff--
	}

	rows, err := q.QueryContext(ctx, `
		SELECT f.path, f.revision, f.deleted, COALESCE(f.content_hash, ''),
		       r.revision, r.deleted, COALESCE(r.content_hash, ''), r.content
		FROM files f
		LEFT JOIN file_revisions r ON r.rowid = (
			SELECT MAX(rowid) FROM file_revisions WHERE path = f.path AND rowid <= ?)
		ORDER BY f.path`, cutoff)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	ops := []syncproto.RevertOp{}
	h := sha256.New()
	fmt.Fprintf(h, "%s\t%d\t%v\n", path, revision, before)
	for rows.Next() {
		var p, curHash string
		var curRev int64
		var curDeleted bool
		var tRev sql.NullInt64
		var tDeleted sql.NullBool
		var tHash sql.NullString
		var tContent []byte
		if err := rows.Scan(&p, &curRev, &curDeleted, &curHash, &tRev, &tDeleted, &tHash, &tContent); err != nil {
			return nil, "", err
		}
		if handoff.InNamespace(p) {
			continue
		}
		op := syncproto.RevertOp{Path: p, FromRevision: curRev}
		switch {
		case !tRev.Valid || tDeleted.Bool:
			if curDeleted {
				continue
			}
			op.Action = "delete"
		case curDeleted || curHash != tHash.String:
			op.Action, op.TargetRevision, op.Content = "write", tRev.Int64, tContent
			if op.Content == nil {
				op.Content = []byte{}
			}
		default:
			continue
		}
		ops = append(ops, op)
		fmt.Fprintf(h, "%s\t%d\t%s\n", op.Path, op.FromRevision, op.Action)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return ops, hex.EncodeToString(h.Sum(nil)), nil
}

// ApplyRevert re-plans inside one transaction and, if the plan still matches
// token, applies every op as a new revision attributed to username. History
// is only ever appended to: nothing newer than the target is discarded.
func (s *Store) ApplyRevert(ctx context.Context, path string, revision int64, before bool, token, username string) ([]*FileRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ops, got, err := s.RevertPlan(ctx, tx, path, revision, before)
	if err != nil {
		return nil, err
	}
	if got != token {
		return nil, ErrStalePlan
	}
	var out []*FileRecord
	for _, op := range ops {
		cur, err := txGet(tx, op.Path)
		if err != nil {
			return nil, err
		}
		var rec *FileRecord
		if op.Action == "delete" {
			rec, err = txTombstone(ctx, tx, cur, username)
		} else {
			rec, err = txWrite(ctx, tx, op.Path, op.Content, cur, username)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// readRevertRequest decodes a revert request and checks the admin password.
// Reverting is off entirely unless rex-svr was started with
// --admin-password.
func (s *Server) readRevertRequest(w http.ResponseWriter, r *http.Request) (syncproto.RevertRequest, bool) {
	var req syncproto.RevertRequest
	if !s.readAdminRequest(w, r, "revert", 1<<16, &req, func() string { return req.AdminPassword }) {
		return req, false
	}
	p, ok := normalizeOrReject(w, req.Path)
	req.Path = p
	return req, ok
}

// readAdminRequest decodes an admin-gated JSON body into req and checks the
// admin password password() returns from it. action names what's disabled
// ("revert", "delete") when rex-svr has no --admin-password, which switches
// every admin action off.
func (s *Server) readAdminRequest(w http.ResponseWriter, r *http.Request, action string, limit int64, req any, password func() string) bool {
	if s.cfg.AdminPasswordHash == "" {
		writeError(w, http.StatusForbidden, action+"_disabled", action+" is disabled: start rex-svr with --admin-password to enable it")
		return false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(s.cfg.AdminPasswordHash), []byte(password())) != nil {
		writeError(w, http.StatusForbidden, "bad_admin_password", "wrong admin password")
		return false
	}
	return true
}

func (s *Server) handleRevertPreview(w http.ResponseWriter, r *http.Request) {
	req, ok := s.readRevertRequest(w, r)
	if !ok {
		return
	}
	ops, token, err := s.store.RevertPlan(r.Context(), s.db, req.Path, req.Revision, req.Before)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such revision")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to plan revert")
		return
	}
	resp := syncproto.RevertPreview{Path: req.Path, Revision: req.Revision, Before: req.Before, Ops: ops, Plan: token, GitLog: []string{}}
	if gm, ok := s.materializer.(*GitMaterializer); ok {
		if content, err := s.store.RevisionContent(r.Context(), req.Path, req.Revision); err == nil && content != nil {
			resp.Commit = gm.CommitFor(req.Path, content)
		}
		resp.GitLog = gm.Log(30)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	req, ok := s.readRevertRequest(w, r)
	if !ok {
		return
	}
	username := usernameFromContext(r.Context())
	recs, err := s.store.ApplyRevert(r.Context(), req.Path, req.Revision, req.Before, req.Plan, username)
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such revision")
		return
	case errors.Is(err, ErrStalePlan):
		writeError(w, http.StatusConflict, "stale_plan", err.Error()+"; preview it again")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "failed to apply revert")
		return
	}

	resp := syncproto.RevertResponse{Ops: []syncproto.RevertOp{}}
	for _, rec := range recs {
		op := "write"
		if rec.Deleted {
			op = "delete"
		}
		s.materializer.Enqueue(rec.Path, op, rec.Revision, username)
		s.notifier.NotifyFileUpdated(rec.Path, rec.Revision, rec.Hash, username, rec.Deleted)
		resp.Ops = append(resp.Ops, syncproto.RevertOp{Path: rec.Path, Action: op, FromRevision: rec.Revision})
	}
	if gm, ok := s.materializer.(*GitMaterializer); ok && len(recs) > 0 {
		to := "to"
		if req.Before {
			to = "to before"
		}
		resp.Commit = gm.Commit(r.Context(), fmt.Sprintf("Revert %s %s rev %d by %s", to, req.Path, req.Revision, username))
	}
	writeJSON(w, http.StatusOK, resp)
}
