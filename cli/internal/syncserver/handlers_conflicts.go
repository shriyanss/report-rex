package syncserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	conflicts, err := s.store.ListOpenConflicts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to list conflicts")
		return
	}
	out := make([]syncproto.ConflictSummary, 0, len(conflicts))
	for _, c := range conflicts {
		out = append(out, syncproto.ConflictSummary{
			ID: c.ID, Path: c.Path, Kind: syncproto.ConflictKind(c.Kind),
			RemoteRevision: c.RemoteRevision, LocalUsername: c.LocalUsername, CreatedAt: c.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, syncproto.ConflictsResponse{Conflicts: out})
}

func parseConflictID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid conflict id")
		return 0, false
	}
	return id, true
}

func (s *Server) handleGetConflict(w http.ResponseWriter, r *http.Request) {
	id, ok := parseConflictID(w, r)
	if !ok {
		return
	}
	c, err := s.store.GetConflict(r.Context(), id)
	if errors.Is(err, ErrConflictNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conflict not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load conflict")
		return
	}

	detail := syncproto.ConflictDetail{
		ID: c.ID, Path: c.Path, Kind: syncproto.ConflictKind(c.Kind), BaseRevision: c.BaseRevision,
		LocalHash: c.LocalHash, LocalUsername: c.LocalUsername,
		RemoteRevision: c.RemoteRevision, RemoteHash: c.RemoteHash,
		Status: c.Status, CreatedAt: c.CreatedAt, HasBase: c.BaseRevision != nil,
	}

	include := r.URL.Query().Get("include")
	if strings.Contains(include, "local") {
		detail.LocalContent = c.LocalContent
	}
	if strings.Contains(include, "remote") {
		if remote, err := s.store.Get(r.Context(), c.Path); err == nil {
			detail.RemoteContent = remote.Content
			if !remote.Deleted {
				detail.RemoteContentRevision = remote.Revision
			}
		}
	}
	if strings.Contains(include, "base") && c.BaseRevision != nil {
		if base, err := s.store.RevisionContent(r.Context(), c.Path, *c.BaseRevision); err == nil {
			detail.BaseContent = base
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	id, ok := parseConflictID(w, r)
	if !ok {
		return
	}
	username := usernameFromContext(r.Context())

	var req syncproto.ResolveRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxUploadSize+1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	c, err := s.store.GetConflict(r.Context(), id)
	if errors.Is(err, ErrConflictNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conflict not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load conflict")
		return
	}
	if c.Status != "open" {
		writeError(w, http.StatusConflict, "already_resolved", "conflict already resolved")
		return
	}

	resp, err := s.applyResolution(r.Context(), c, req, username)
	if errors.Is(err, ErrStaleBase) {
		writeError(w, http.StatusConflict, "stale_remote", "the file changed on the server since this merge was built; fetch the conflict again and re-merge")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if err := s.store.MarkResolved(r.Context(), id, string(req.Resolution), username); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to mark conflict resolved")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// applyResolution performs the file-level side effect for a resolution
// decision. It does not mark the conflict resolved -- the caller does that
// after this returns successfully, so a failure here never leaves a
// conflict silently closed without the corresponding state change.
func (s *Server) applyResolution(ctx context.Context, c *Conflict, req syncproto.ResolveRequest, username string) (syncproto.ResolveResponse, error) {
	switch req.Resolution {
	case syncproto.ResolutionUseRemote:
		// Pure local overwrite; no server/git state change.
		return syncproto.ResolveResponse{Path: c.Path}, nil

	case syncproto.ResolutionUseLocal, syncproto.ResolutionMerge, syncproto.ResolutionRestoreLocal:
		content := c.LocalContent
		if req.Resolution == syncproto.ResolutionMerge {
			content = req.MergedContent
		}
		cur, err := s.store.Get(ctx, c.Path)
		var base *int64
		if err == nil {
			base = &cur.Revision
		}
		if req.Resolution == syncproto.ResolutionMerge && req.ExpectedRemoteRevision != nil {
			base = req.ExpectedRemoteRevision
		}
		rec, err := s.store.Put(ctx, c.Path, content, base, username)
		if err != nil {
			return syncproto.ResolveResponse{}, fmt.Errorf("apply resolution: %w", err)
		}
		s.materializer.Enqueue(c.Path, "write", rec.Revision, username)
		s.notifier.NotifyFileUpdated(c.Path, rec.Revision, rec.Hash, username, false)
		return syncproto.ResolveResponse{Path: c.Path, NewRevision: &rec.Revision}, nil

	case syncproto.ResolutionKeepBoth:
		suffix := req.KeepBothSuffix
		if suffix == "" {
			suffix = c.LocalUsername
		}
		newPath := s.deriveKeepBothPath(ctx, c.Path, suffix)
		rec, err := s.store.Put(ctx, newPath, c.LocalContent, nil, username)
		if err != nil {
			return syncproto.ResolveResponse{}, fmt.Errorf("apply keep-both resolution: %w", err)
		}
		s.materializer.Enqueue(newPath, "write", rec.Revision, username)
		s.notifier.NotifyFileUpdated(newPath, rec.Revision, rec.Hash, username, false)
		return syncproto.ResolveResponse{Path: c.Path, NewPath: newPath, NewRevision: &rec.Revision}, nil

	case syncproto.ResolutionKeepDeletion:
		// Tombstone already reflects the deletion; nothing further to do.
		return syncproto.ResolveResponse{Path: c.Path}, nil

	default:
		return syncproto.ResolveResponse{}, fmt.Errorf("unknown resolution %q", req.Resolution)
	}
}

// deriveKeepBothPath computes a collision-free sibling path for "keep
// both", e.g. proof.png -> proof.conflict-alice.png, appending a numeric
// suffix if that's also taken.
func (s *Server) deriveKeepBothPath(ctx context.Context, original, suffix string) string {
	ext := path.Ext(original)
	base := strings.TrimSuffix(original, ext)

	candidate := fmt.Sprintf("%s.conflict-%s%s", base, suffix, ext)
	for n := 2; ; n++ {
		if _, err := s.store.Get(ctx, candidate); errors.Is(err, ErrNotFound) {
			return candidate
		}
		candidate = fmt.Sprintf("%s.conflict-%s-%d%s", base, suffix, n, ext)
	}
}
