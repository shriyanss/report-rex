package syncserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
	"github.com/shriyanss/report-rex/cli/internal/version"
)

const maxUploadSize = 200 << 20 // 200MB per file, generous for screenshots

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/v1/manifest", s.withAuth(s.handleManifest))
	s.mux.HandleFunc("GET /api/v1/files/{path...}", s.withAuth(s.handleGetFile))
	s.mux.HandleFunc("POST /api/v1/files/{path...}", s.withAuth(s.handlePutFile))
	s.mux.HandleFunc("DELETE /api/v1/files/{path...}", s.withAuth(s.handleDeleteFile))
	s.mux.HandleFunc("GET /api/v1/presence", s.withAuth(s.handleListPresence))
	s.mux.HandleFunc("GET /api/v1/conflicts", s.withAuth(s.handleListConflicts))
	s.mux.HandleFunc("GET /api/v1/conflicts/{id}", s.withAuth(s.handleGetConflict))
	s.mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", s.withAuth(s.handleResolveConflict))
	s.mux.HandleFunc("GET /api/v1/handoffs", s.withAuth(s.handleListHandoffs))
	s.mux.HandleFunc("GET /api/v1/handoffs/{id}", s.withAuth(s.handleGetHandoff))
	s.mux.HandleFunc("POST /api/v1/handoffs/{id}/resolve", s.withAuth(s.handleResolveHandoff))
	s.mux.HandleFunc("POST /api/v1/revert/preview", s.withAuth(s.handleRevertPreview))
	s.mux.HandleFunc("POST /api/v1/revert", s.withAuth(s.handleRevert))
	s.mux.HandleFunc("POST /api/v1/admin/delete", s.withAuth(s.handleBulkDelete))
	s.mux.HandleFunc("POST /api/v1/admin/verify", s.withAuth(s.handleAdminVerify))
	s.mux.HandleFunc("POST /api/v1/sync/restore", s.withAuth(s.handleSyncRestore))
	s.mux.HandleFunc("GET /api/v1/blame/{path...}", s.withAuth(s.handleBlame))
	s.mux.HandleFunc("GET /api/v1/healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /api/v1/ws", s.withAuth(s.handleWS))
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, syncproto.ErrorResponse{Error: msg, Code: code})
}

type ctxKeyUsername struct{}

func withUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, ctxKeyUsername{}, username)
}

func usernameFromContext(ctx context.Context) string {
	u, _ := ctx.Value(ctxKeyUsername{}).(string)
	return u
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		username, err := s.auth.Authorize(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
			return
		}
		ctx := withUsername(r.Context(), username)
		next(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func normalizeOrReject(w http.ResponseWriter, raw string) (string, bool) {
	p, err := synccommon.NormalizePath(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_path", err.Error())
		return "", false
	}
	if synccommon.IsExcluded(p) {
		writeError(w, http.StatusBadRequest, "excluded_path", "path is excluded from sync scope")
		return "", false
	}
	return p, true
}

// --- handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req syncproto.LoginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	token, err := s.auth.Login(r.Context(), req.Username, req.Password, s.cfg.PasswordHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, syncproto.LoginResponse{Token: token, Username: req.Username})
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	recs, err := s.store.Manifest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load manifest")
		return
	}
	entries := make([]syncproto.FileEntry, 0, len(recs))
	for _, rec := range recs {
		entries = append(entries, syncproto.FileEntry{
			Path: rec.Path, Revision: rec.Revision, SHA256: rec.Hash, Size: rec.Size,
			Deleted: rec.Deleted, ModifiedBy: rec.ModifiedBy, UpdatedAt: rec.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, syncproto.ManifestResponse{Files: entries})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	p, ok := normalizeOrReject(w, r.PathValue("path"))
	if !ok {
		return
	}
	rec, err := s.store.Get(r.Context(), p)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "path does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load file")
		return
	}
	if rec.Deleted {
		w.Header().Set(syncproto.HeaderDeleted, "true")
		w.Header().Set(syncproto.HeaderRevision, strconv.FormatInt(rec.Revision, 10))
		writeError(w, http.StatusGone, "deleted", "path has been deleted")
		return
	}
	w.Header().Set(syncproto.HeaderRevision, strconv.FormatInt(rec.Revision, 10))
	w.Header().Set(syncproto.HeaderSHA256, rec.Hash)
	w.Header().Set(syncproto.HeaderModifiedBy, rec.ModifiedBy)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(rec.Content)
}

func (s *Server) handleBlame(w http.ResponseWriter, r *http.Request) {
	p, ok := normalizeOrReject(w, r.PathValue("path"))
	if !ok {
		return
	}
	gm, ok := s.materializer.(*GitMaterializer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "no_git", "git history is not available on this server")
		return
	}
	if rec, err := s.store.Get(r.Context(), p); err != nil || rec.Deleted {
		writeError(w, http.StatusNotFound, "not_found", "path does not exist")
		return
	}
	lines, err := gm.Blame(r.Context(), p)
	if err != nil {
		// Written but not materialized yet: nothing in git to blame.
		writeError(w, http.StatusNotFound, "not_found", "path has no git history yet; retry in a few seconds")
		return
	}
	writeJSON(w, http.StatusOK, syncproto.BlameResponse{Path: p, Lines: lines})
}

func (s *Server) handlePutFile(w http.ResponseWriter, r *http.Request) {
	p, ok := normalizeOrReject(w, r.PathValue("path"))
	if !ok {
		return
	}
	username := usernameFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, maxUploadSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "failed to read body")
		return
	}
	if len(body) > maxUploadSize {
		writeError(w, http.StatusBadRequest, "too_large", "file exceeds maximum upload size")
		return
	}

	if claimed := r.Header.Get(syncproto.HeaderSHA256); claimed != "" {
		if actual := synccommon.HashBytes(body); actual != claimed {
			writeError(w, http.StatusBadRequest, "hash_mismatch", "X-Sha256 does not match body")
			return
		}
	}

	var baseRevision *int64
	if v := r.Header.Get(syncproto.HeaderBaseRevision); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid X-Base-Revision")
			return
		}
		baseRevision = &n
	}

	// The handoff namespace is not a plain file tree: its directory encodes
	// the assignee, so an upload whose metadata disagrees must be refused
	// rather than delivered to the wrong user's queue.
	if handoff.InNamespace(p) {
		if status, msg := s.validateHandoffUpload(r.Context(), p, body, username); status != 0 {
			writeError(w, status, "invalid_handoff", msg)
			return
		}
	}

	rec, err := s.store.Put(r.Context(), p, body, baseRevision, username)
	if errors.Is(err, ErrStaleBase) {
		s.handleUploadConflict(w, r, p, ConflictKindDivergentEdit, baseRevision, body, username)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to store file")
		return
	}

	s.materializer.Enqueue(p, "write", rec.Revision, username)
	s.notifier.NotifyFileUpdated(p, rec.Revision, rec.Hash, username, false)

	writeJSON(w, http.StatusOK, syncproto.PutFileResponse{Path: p, Revision: rec.Revision, SHA256: rec.Hash})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	p, ok := normalizeOrReject(w, r.PathValue("path"))
	if !ok {
		return
	}
	username := usernameFromContext(r.Context())

	// A handoff is retired by resolving it, never by deleting the canonical
	// file: the server keeps it for history. Refuse the raw delete outright
	// rather than trusting every client to route around it.
	if handoff.IsPath(p) {
		if rec, err := s.store.Get(r.Context(), p); err == nil && !rec.Deleted {
			if !handoff.ParseMeta(rec.Content).Resolved() {
				writeError(w, http.StatusUnprocessableEntity, "invalid_handoff",
					"resolve this handoff via POST /api/v1/handoffs/{id}/resolve (`rex handoffs remove`); the canonical file is retained on the server")
				return
			}
		}
	}

	v := r.Header.Get(syncproto.HeaderBaseRevision)
	if v == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "X-Base-Revision is required for delete")
		return
	}
	baseRevision, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid X-Base-Revision")
		return
	}

	// Mass-delete guard (issue #46), server side so old clients can't bypass it.
	live, recent, err := s.store.DeleteStats(r.Context(), username, time.Now().Add(-massDeleteWindow).UTC().Format(time.RFC3339Nano))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete file")
		return
	}
	if n := recent + 1; n >= massDeleteMinCount && float64(n)/float64(live+recent) > massDeleteMaxFraction {
		writeError(w, http.StatusUnprocessableEntity, "mass_delete_refused", fmt.Sprintf(
			"refusing to delete %d of %d files in %s -- if the workspace was moved, move it back; "+
				"to really delete them, use \"Report Rex: Delete for Everyone\" or `rex delete` with the admin password",
			n, live+recent, massDeleteWindow))
		return
	}

	rec, err := s.store.Delete(r.Context(), p, baseRevision, username)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "path does not exist")
		return
	}
	if errors.Is(err, ErrStaleBase) {
		// Base is stale: if remote is *also* a delete this is a delete/delete
		// race already handled as a no-op in Store.Delete; here it means the
		// remote was concurrently edited -- a delete/edit conflict.
		s.handleDeleteConflict(w, r, p, baseRevision, username)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete file")
		return
	}

	s.materializer.Enqueue(p, "delete", rec.Revision, username)
	s.notifier.NotifyFileUpdated(p, rec.Revision, "", username, true)

	writeJSON(w, http.StatusOK, syncproto.DeleteFileResponse{Path: p, Revision: rec.Revision, Deleted: true})
}

func (s *Server) handleUploadConflict(w http.ResponseWriter, r *http.Request, path string, kind ConflictKind, baseRevision *int64, localContent []byte, username string) {
	cur, err := s.store.Get(r.Context(), path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load current state")
		return
	}
	var remoteRevision int64
	var remoteHash string
	if err == nil {
		remoteRevision = cur.Revision
		remoteHash = cur.Hash
	}
	if baseRevision == nil {
		kind = ConflictKindInitialUnknown
	}
	id, created, err := s.store.CreateConflict(r.Context(), Conflict{
		Path: path, Kind: kind, BaseRevision: baseRevision,
		LocalContent: localContent, LocalHash: synccommon.HashBytes(localContent), LocalUsername: username,
		RemoteRevision: remoteRevision, RemoteHash: remoteHash,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to record conflict")
		return
	}
	// Only announce a genuinely new conflict: every broadcast makes every
	// client run a full reconcile, so re-announcing an updated one on each
	// rejected push turns one busy file into a reconcile storm.
	if created {
		s.notifier.NotifyConflictCreated(id, path, kind)
	}
	writeJSON(w, http.StatusConflict, syncproto.ConflictError{
		ConflictID: id, Kind: syncproto.ConflictKind(kind),
		RemoteRevision: remoteRevision, RemoteSHA256: remoteHash,
		RemoteContentURL: "/api/v1/files/" + path,
	})
}

func (s *Server) handleDeleteConflict(w http.ResponseWriter, r *http.Request, path string, baseRevision int64, username string) {
	cur, err := s.store.Get(r.Context(), path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load current state")
		return
	}
	id, created, err := s.store.CreateConflict(r.Context(), Conflict{
		Path: path, Kind: ConflictKindDeleteEdit, BaseRevision: &baseRevision,
		LocalContent: []byte{}, LocalHash: "", LocalUsername: username,
		RemoteRevision: cur.Revision, RemoteHash: cur.Hash,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to record conflict")
		return
	}
	if created {
		s.notifier.NotifyConflictCreated(id, path, ConflictKindDeleteEdit)
	}
	writeJSON(w, http.StatusConflict, syncproto.ConflictError{
		ConflictID: id, Kind: syncproto.ConflictKind(ConflictKindDeleteEdit),
		RemoteRevision: cur.Revision, RemoteSHA256: cur.Hash,
		RemoteContentURL: "/api/v1/files/" + path,
	})
}

// handleListPresence returns a snapshot of every currently-active
// edit-presence lease, for a client that just connected and hasn't seen a
// WS presence broadcast yet. Empty (rather than an error) if no WSHub is
// configured, since presence is purely advisory.
func (s *Server) handleListPresence(w http.ResponseWriter, r *http.Request) {
	hub, ok := s.notifier.(*WSHub)
	if !ok {
		writeJSON(w, http.StatusOK, syncproto.PresenceListResponse{Presence: []syncproto.FilePresenceEntry{}})
		return
	}
	active := hub.presence.Active()
	entries := make([]syncproto.FilePresenceEntry, 0, len(active))
	for _, e := range active {
		entries = append(entries, syncproto.FilePresenceEntry{
			Path: e.Path, Username: e.Username, SessionID: e.SessionID,
			State: e.State, Dirty: e.Dirty, ExpiresAt: e.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, syncproto.PresenceListResponse{Presence: entries})
}

func (s *Server) handleSyncRestore(w http.ResponseWriter, r *http.Request) {
	// The actual workspace overwrite is client-side; this endpoint just
	// returns the manifest, same as GET /manifest, for symmetry.
	s.handleManifest(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	dbOK, gitOK, pending := s.HealthStatus(r.Context())
	status := "ok"
	if !dbOK || !gitOK {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, syncproto.HealthResponse{
		Status: status, GitOK: gitOK, DBOK: dbOK, PendingMaterializations: pending,
		Version: version.Version,
	})
}
