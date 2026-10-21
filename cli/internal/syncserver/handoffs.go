package syncserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// errHandoffNotFound is returned by findHandoff when no handoff carries the
// requested id.
var errHandoffNotFound = errors.New("handoff not found")

// handoffRecord pairs a parsed handoff with the store state it came from.
type handoffRecord struct {
	handoff  *handoff.Handoff
	content  []byte
	revision int64 // current revision of the *path*, i.e. the tombstone's for a resolved handoff
	deleted  bool
}

func (r handoffRecord) entry() syncproto.HandoffEntry {
	h := r.handoff
	return syncproto.HandoffEntry{
		ID: h.ID, Path: h.Path, Title: h.Title,
		From: h.From, To: h.To, Status: h.Status, Revision: r.revision,
		CreatedAt: h.CreatedAt, ResolvedBy: h.ResolvedBy, ResolvedAt: h.ResolvedAt,
	}
}

// collectHandoffs reads every handoff in the canonical store, splitting the
// ones whose path and `to` metadata agree from the ones that need operator
// attention. Resolved handoffs (whose active path has been tombstoned) are
// read back from file_revisions and only included when asked for.
//
// ponytail: reads and parses every handoff file on each call, which is fine
// for the tens-of-handoffs an engagement produces. Add a handoffs index
// table if that ever reaches the hundreds.
func (s *Server) collectHandoffs(ctx context.Context, includeResolved bool) ([]handoffRecord, []syncproto.HandoffInvalid, error) {
	manifest, err := s.store.Manifest(ctx)
	if err != nil {
		return nil, nil, err
	}
	var out []handoffRecord
	var invalid []syncproto.HandoffInvalid
	for _, rec := range manifest {
		if !handoff.IsPath(rec.Path) {
			continue
		}
		var content []byte
		if rec.Deleted {
			if !includeResolved {
				continue
			}
			content, _, err = s.store.LatestContentRevision(ctx, rec.Path)
			if errors.Is(err, ErrNotFound) {
				continue // tombstone with no retained content: nothing to report
			}
			if err != nil {
				return nil, nil, err
			}
		} else {
			full, err := s.store.Get(ctx, rec.Path)
			if errors.Is(err, ErrNotFound) {
				continue // raced with a delete
			}
			if err != nil {
				return nil, nil, err
			}
			content = full.Content
		}

		h, parseErr := handoff.Parse(rec.Path, content)
		if parseErr != nil {
			var ve *handoff.ValidationError
			if errors.As(parseErr, &ve) {
				invalid = append(invalid, syncproto.HandoffInvalid{
					Path: ve.Path, DirAssignee: ve.DirAssignee,
					MetaAssignee: ve.MetaAssignee, Error: ve.Error(),
				})
			}
			continue
		}
		// A tombstoned path is not active regardless of what its last
		// retained frontmatter says, so report it as resolved either way.
		if rec.Deleted && h.Status != handoff.StatusResolved {
			h.Status = handoff.StatusResolved
		}
		if !includeResolved && h.Resolved() {
			continue
		}
		out = append(out, handoffRecord{handoff: h, content: content, revision: rec.Revision, deleted: rec.Deleted})
	}
	return out, invalid, nil
}

// findHandoff locates a handoff by its stable id, searching resolved
// handoffs too so `view` keeps working after resolution.
func (s *Server) findHandoff(ctx context.Context, id string) (handoffRecord, error) {
	recs, _, err := s.collectHandoffs(ctx, true)
	if err != nil {
		return handoffRecord{}, err
	}
	for _, r := range recs {
		if r.handoff.ID == id {
			return r, nil
		}
	}
	return handoffRecord{}, errHandoffNotFound
}

func (s *Server) handleListHandoffs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	includeResolved := q.Get("resolved") == "1" || q.Get("resolved") == "true"
	all := q.Get("all") == "1" || q.Get("all") == "true"

	// Scope defaults to the authenticated user's own queue; ?user= selects
	// someone else's, ?all=1 drops the filter entirely.
	scope := usernameFromContext(r.Context())
	if u := q.Get("user"); u != "" {
		scope = u
	}
	if all {
		scope = ""
	}

	recs, invalid, err := s.collectHandoffs(r.Context(), includeResolved)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load handoffs")
		return
	}
	entries := make([]syncproto.HandoffEntry, 0, len(recs))
	for _, rec := range recs {
		if scope != "" && rec.handoff.To != scope {
			continue
		}
		entries = append(entries, rec.entry())
	}
	// Invalid handoffs can't be attributed to a queue (that's exactly what's
	// broken about them), so they're always reported in full.
	writeJSON(w, http.StatusOK, syncproto.HandoffListResponse{Handoffs: entries, Invalid: invalid})
}

func (s *Server) handleGetHandoff(w http.ResponseWriter, r *http.Request) {
	rec, err := s.findHandoff(r.Context(), r.PathValue("id"))
	if errors.Is(err, errHandoffNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no handoff with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load handoff")
		return
	}
	writeJSON(w, http.StatusOK, syncproto.HandoffDetail{HandoffEntry: rec.entry(), Content: rec.content})
}

// handleResolveHandoff moves a handoff to its terminal state. The final
// metadata (status/resolved_by/resolved_at) is written as one more canonical
// revision, then the active path is tombstoned so it stops being
// materialized into client workspaces. The content itself is never lost --
// it stays in file_revisions and in the server's git history, which is what
// `handoffs view <id>` and `handoffs list --resolved` read back.
func (s *Server) handleResolveHandoff(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	rec, err := s.findHandoff(r.Context(), r.PathValue("id"))
	if errors.Is(err, errHandoffNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no handoff with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to load handoff")
		return
	}
	if rec.deleted || rec.handoff.Resolved() {
		writeError(w, http.StatusConflict, "already_resolved", "handoff is already resolved")
		return
	}

	resolved, err := handoff.SetResolved(rec.content, username, handoff.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to update handoff metadata")
		return
	}
	path := rec.handoff.Path
	final, err := s.store.Put(r.Context(), path, resolved, &rec.revision, username)
	if errors.Is(err, ErrStaleBase) {
		writeError(w, http.StatusConflict, "stale", "handoff changed concurrently; retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to store resolved handoff")
		return
	}
	s.materializer.Enqueue(path, "write", final.Revision, username)

	tomb, err := s.store.Delete(r.Context(), path, final.Revision, username)
	if err != nil {
		// The resolved metadata is committed; the tombstone can be retried.
		s.notifier.NotifyFileUpdated(path, final.Revision, final.Hash, username, false)
		writeError(w, http.StatusInternalServerError, "internal", "failed to retire resolved handoff")
		return
	}
	s.materializer.Enqueue(path, "delete", tomb.Revision, username)
	s.notifier.NotifyFileUpdated(path, tomb.Revision, "", username, true)

	writeJSON(w, http.StatusOK, syncproto.HandoffResolveResponse{
		ID: rec.handoff.ID, Path: path, Revision: tomb.Revision,
	})
}

// validateHandoffUpload enforces the handoff namespace's invariants on every
// write, so a malformed or hand-edited file can never be delivered to the
// wrong user. It returns an HTTP status and message when the upload must be
// rejected.
//
// Identity is server-checked rather than server-rewritten: the client stamps
// `from`/`rex_handoff_id` on create (see `rex handoffs sync`) and the server
// refuses anything that disagrees with the authenticated session or with the
// stored original.
func (s *Server) validateHandoffUpload(ctx context.Context, path string, body []byte, username string) (int, string) {
	if !handoff.IsPath(path) {
		return http.StatusUnprocessableEntity,
			"paths under handoffs/ must be exactly handoffs/<assignee>/<title>.md"
	}
	h, err := handoff.Parse(path, body)
	if err != nil {
		return http.StatusUnprocessableEntity, err.Error()
	}
	if h.ID == "" {
		return http.StatusUnprocessableEntity, "handoff is missing rex_handoff_id metadata"
	}

	cur, err := s.store.Get(ctx, path)
	if errors.Is(err, ErrNotFound) || (err == nil && cur.Deleted) {
		// Create (or re-create after resolution): the creator is whoever is
		// authenticated, never whoever the file claims.
		if h.From != username {
			return http.StatusUnprocessableEntity,
				"handoff `from` must be the authenticated user (" + username + ")"
		}
		return 0, ""
	}
	if err != nil {
		return http.StatusInternalServerError, "failed to load current handoff state"
	}

	// Update: identity is immutable. Status may only move open -> resolved,
	// and only through the resolve endpoint, which supplies resolved_by.
	prev := handoff.ParseMeta(cur.Content)
	if prev.From != "" && h.From != prev.From {
		return http.StatusUnprocessableEntity, "handoff `from` cannot be changed after creation"
	}
	if prev.ID != "" && h.ID != prev.ID {
		return http.StatusUnprocessableEntity, "rex_handoff_id cannot be changed after creation"
	}
	if h.Resolved() && !prev.Resolved() {
		// Editing `status: resolved` into the file by hand would leave a
		// handoff that reads as finished but is still materialized in every
		// subscriber's workspace. Resolution has to go through the resolve
		// operation so the path is retired at the same time.
		return http.StatusUnprocessableEntity,
			"resolve a handoff with `rex handoffs remove <id>` (or by deleting the file), not by editing `status`"
	}
	return 0, ""
}
