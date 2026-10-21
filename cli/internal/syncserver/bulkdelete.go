package syncserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// BulkDelete tombstones every live path in paths in one transaction and
// returns the new tombstones plus the paths it skipped: missing, already
// deleted, or in the handoff namespace (handoffs are retired by resolving
// them, and a revert leaves them alone too).
func (s *Store) BulkDelete(ctx context.Context, paths []string, username string) ([]*FileRecord, []string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var out []*FileRecord
	skipped := []string{}
	for _, p := range paths {
		cur, err := txGet(tx, p)
		if handoff.InNamespace(p) || errors.Is(err, sql.ErrNoRows) || (err == nil && cur.Deleted) {
			skipped = append(skipped, p)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		rec, err := txTombstone(ctx, tx, cur, username)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, rec)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}
	return out, skipped, nil
}

// handleAdminVerify is POST /api/v1/admin/verify: 200 if the admin
// password is right, the same 403s as every admin action otherwise.
func (s *Server) handleAdminVerify(w http.ResponseWriter, r *http.Request) {
	var req syncproto.AdminVerifyRequest
	if s.readAdminRequest(w, r, "admin", 1<<12, &req, func() string { return req.AdminPassword }) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// handleBulkDelete is POST /api/v1/admin/delete: the deliberate,
// admin-password-gated way to delete what the sync client's mass-delete
// guard refuses to push (issue #46).
func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	var req syncproto.BulkDeleteRequest
	if !s.readAdminRequest(w, r, "delete", 1<<22, &req, func() string { return req.AdminPassword }) {
		return
	}
	// Excluded paths are skipped rather than refused: "select all" in an
	// editor happily includes .vscode/ and friends.
	paths := make([]string, 0, len(req.Paths))
	var excluded []string
	for _, raw := range req.Paths {
		p, err := synccommon.NormalizePath(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_path", err.Error())
			return
		}
		if synccommon.IsExcluded(p) {
			excluded = append(excluded, p)
			continue
		}
		paths = append(paths, p)
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "no paths to delete")
		return
	}

	username := usernameFromContext(r.Context())
	recs, skipped, err := s.store.BulkDelete(r.Context(), paths, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete files")
		return
	}

	resp := syncproto.BulkDeleteResponse{Deleted: []syncproto.DeleteFileResponse{}, Skipped: append(excluded, skipped...)}
	for _, rec := range recs {
		s.materializer.Enqueue(rec.Path, "delete", rec.Revision, username)
		s.notifier.NotifyFileUpdated(rec.Path, rec.Revision, "", username, true)
		resp.Deleted = append(resp.Deleted, syncproto.DeleteFileResponse{Path: rec.Path, Revision: rec.Revision, Deleted: true})
	}
	if gm, ok := s.materializer.(*GitMaterializer); ok && len(recs) > 0 {
		resp.Commit = gm.Commit(r.Context(), fmt.Sprintf("Delete %d file(s) by %s (admin)", len(recs), username))
	}
	writeJSON(w, http.StatusOK, resp)
}
