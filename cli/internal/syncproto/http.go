// Package syncproto defines the wire-protocol types shared by the sync
// server and client: HTTP request/response JSON shapes and WebSocket
// message types. It has no server or client behavior of its own.
package syncproto

// LoginRequest is the body of POST /api/v1/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is returned on successful authentication.
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// FileEntry describes one path's canonical state, as returned in a
// manifest listing.
type FileEntry struct {
	Path       string `json:"path"`
	Revision   int64  `json:"revision"`
	SHA256     string `json:"sha256,omitempty"`
	Size       int64  `json:"size"`
	Deleted    bool   `json:"deleted"`
	ModifiedBy string `json:"modified_by"`
	UpdatedAt  string `json:"updated_at"`
}

// ManifestResponse is returned by GET /api/v1/manifest and POST
// /api/v1/sync/restore.
type ManifestResponse struct {
	Files []FileEntry `json:"files"`
}

// FilePresenceEntry describes one currently-active editor session's
// presence, as returned by GET /api/v1/presence -- a REST snapshot for
// clients that just connected and haven't seen a WS broadcast yet.
type FilePresenceEntry struct {
	Path      string        `json:"path"`
	Username  string        `json:"username"`
	SessionID string        `json:"session_id"`
	State     PresenceState `json:"state"`
	Dirty     bool          `json:"dirty"`
	ExpiresAt string        `json:"expires_at"`
}

// PresenceListResponse is returned by GET /api/v1/presence.
type PresenceListResponse struct {
	Presence []FilePresenceEntry `json:"presence"`
}

// PutFileResponse is returned on a successful POST /api/v1/files/{path}.
type PutFileResponse struct {
	Path     string `json:"path"`
	Revision int64  `json:"revision"`
	SHA256   string `json:"sha256"`
}

// DeleteFileResponse is returned on a successful DELETE /api/v1/files/{path}.
type DeleteFileResponse struct {
	Path     string `json:"path"`
	Revision int64  `json:"revision"`
	Deleted  bool   `json:"deleted"`
}

// ConflictKind enumerates why a conflict record was created.
type ConflictKind string

const (
	ConflictKindDivergentEdit  ConflictKind = "divergent_edit"
	ConflictKindInitialUnknown ConflictKind = "initial_unknown_base"
	ConflictKindDeleteEdit     ConflictKind = "delete_edit"
)

// ConflictError is the body of a 409 response to POST/DELETE
// /api/v1/files/{path}.
type ConflictError struct {
	ConflictID       int64        `json:"conflict_id"`
	Kind             ConflictKind `json:"kind"`
	RemoteRevision   int64        `json:"remote_revision"`
	RemoteSHA256     string       `json:"remote_sha256,omitempty"`
	RemoteContentURL string       `json:"remote_content_url,omitempty"`
}

// ConflictSummary is one entry in GET /api/v1/conflicts.
type ConflictSummary struct {
	ID             int64        `json:"id"`
	Path           string       `json:"path"`
	Kind           ConflictKind `json:"kind"`
	RemoteRevision int64        `json:"remote_revision"`
	LocalUsername  string       `json:"local_username"`
	CreatedAt      string       `json:"created_at"`
}

// ConflictsResponse is the body of GET /api/v1/conflicts.
type ConflictsResponse struct {
	Conflicts []ConflictSummary `json:"conflicts"`
}

// ConflictDetail is the body of GET /api/v1/conflicts/{id}.
type ConflictDetail struct {
	ID             int64        `json:"id"`
	Path           string       `json:"path"`
	Kind           ConflictKind `json:"kind"`
	BaseRevision   *int64       `json:"base_revision,omitempty"`
	LocalHash      string       `json:"local_hash"`
	LocalUsername  string       `json:"local_username"`
	LocalContent   []byte       `json:"local_content,omitempty"`
	RemoteRevision int64        `json:"remote_revision"`
	RemoteHash     string       `json:"remote_hash"`
	RemoteContent  []byte       `json:"remote_content,omitempty"`
	// RemoteContentRevision is the server revision RemoteContent was read
	// at (the path's *current* revision, which may be newer than
	// RemoteRevision, the revision the conflict was recorded against).
	// Zero when remote content wasn't requested or the path is tombstoned.
	RemoteContentRevision int64  `json:"remote_content_revision,omitempty"`
	BaseContent           []byte `json:"base_content,omitempty"`
	HasBase               bool   `json:"has_base"`
	Status                string `json:"status"`
	CreatedAt             string `json:"created_at"`
}

// ResolutionKind enumerates the accepted values of ResolveRequest.Resolution.
type ResolutionKind string

const (
	ResolutionUseRemote    ResolutionKind = "use_remote"
	ResolutionUseLocal     ResolutionKind = "use_local"
	ResolutionMerge        ResolutionKind = "merge"
	ResolutionKeepBoth     ResolutionKind = "keep_both"
	ResolutionKeepDeletion ResolutionKind = "keep_deletion"
	ResolutionRestoreLocal ResolutionKind = "restore_local"
)

// ResolveRequest is the body of POST /api/v1/conflicts/{id}/resolve.
type ResolveRequest struct {
	Resolution     ResolutionKind `json:"resolution"`
	MergedContent  []byte         `json:"merged_content,omitempty"`
	KeepBothSuffix string         `json:"keep_both_suffix,omitempty"`
	// ExpectedRemoteRevision, for merge, is the revision the merged
	// content was built on (ConflictDetail.RemoteContentRevision). If the
	// path has moved on since, the server refuses with 409 stale_remote
	// instead of overwriting a newer edit with a merge that never saw it.
	ExpectedRemoteRevision *int64 `json:"expected_remote_revision,omitempty"`
}

// ResolveResponse is returned on successful conflict resolution.
type ResolveResponse struct {
	Path        string `json:"path"`
	NewPath     string `json:"new_path,omitempty"` // set for keep_both
	NewRevision *int64 `json:"new_revision,omitempty"`
}

// HealthResponse is the body of GET /api/v1/healthz.
type HealthResponse struct {
	Status                  string `json:"status"`
	GitOK                   bool   `json:"git_ok"`
	DBOK                    bool   `json:"db_ok"`
	PendingMaterializations int    `json:"pending_materializations"`
	Version                 string `json:"version,omitempty"`
}

// ErrorResponse is the body of any 4xx/5xx response.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Header names used by the file content endpoints.
const (
	HeaderBaseRevision = "X-Base-Revision"
	HeaderSHA256       = "X-Sha256"
	HeaderRevision     = "X-Revision"
	HeaderModifiedBy   = "X-Modified-By"
	HeaderDeleted      = "X-Deleted"
)

// HandoffEntry describes one handoff in a listing or detail response. It is
// derived from the canonical file's frontmatter, never from client-supplied
// query state.
type HandoffEntry struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Title      string `json:"title"`
	From       string `json:"from"`
	To         string `json:"to"`
	Status     string `json:"status"`
	Revision   int64  `json:"revision"`
	CreatedAt  string `json:"created_at,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// HandoffInvalid describes a handoff file whose directory and `to` metadata
// disagree. These are reported alongside valid handoffs rather than dropped,
// so the CLI can surface the problem instead of hiding a misassigned file.
type HandoffInvalid struct {
	Path         string `json:"path"`
	DirAssignee  string `json:"dir_assignee"`
	MetaAssignee string `json:"meta_assignee"`
	Error        string `json:"error"`
}

// HandoffListResponse is the body of GET /api/v1/handoffs.
type HandoffListResponse struct {
	Handoffs []HandoffEntry   `json:"handoffs"`
	Invalid  []HandoffInvalid `json:"invalid,omitempty"`
}

// HandoffDetail is the body of GET /api/v1/handoffs/{id}: metadata plus the
// handoff's content, so a client can read a handoff without synchronizing
// it into its workspace.
type HandoffDetail struct {
	HandoffEntry
	Content []byte `json:"content"`
}

// HandoffResolveResponse is returned by POST /api/v1/handoffs/{id}/resolve.
type HandoffResolveResponse struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Revision int64  `json:"revision"`
}

// RevertRequest is the body of POST /api/v1/revert/preview and POST
// /api/v1/revert: roll the whole workspace back to how it stood right after
// Path reached Revision -- or, with Before, right before it, undoing that
// change and everything after it. Plan is only read by the apply endpoint; it must
// be the token the preview returned, so a revert never applies a plan the
// admin didn't see.
type RevertRequest struct {
	Path          string `json:"path"`
	Revision      int64  `json:"revision"`
	Before        bool   `json:"before,omitempty"`
	AdminPassword string `json:"admin_password"`
	Plan          string `json:"plan,omitempty"`
}

// RevertOp is one path a revert changes. Action is "write" (restore
// Content, as it stood at TargetRevision) or "delete" (the path didn't
// exist, or was deleted, at the target point).
type RevertOp struct {
	Path           string `json:"path"`
	Action         string `json:"action"`
	FromRevision   int64  `json:"from_revision"`
	TargetRevision int64  `json:"target_revision,omitempty"`
	Content        []byte `json:"content,omitempty"`
}

// RevertPreview is the body of POST /api/v1/revert/preview.
type RevertPreview struct {
	Path     string `json:"path"`
	Revision int64  `json:"revision"`
	Before   bool   `json:"before,omitempty"`
	// Commit is the server git commit that first recorded the selected
	// change ("<sha>\t<subject>"), or empty if that revision was coalesced
	// into a later commit or was a deletion.
	Commit string     `json:"commit,omitempty"`
	Ops    []RevertOp `json:"ops"`
	Plan   string     `json:"plan"`
	GitLog []string   `json:"git_log"`
}

// RevertResponse is the body of POST /api/v1/revert.
type RevertResponse struct {
	Ops []RevertOp `json:"ops"` // Content omitted; FromRevision is the new revision
	// Commit is the "Revert to ..." commit's sha, empty if git
	// materialization is unavailable.
	Commit string `json:"commit,omitempty"`
}

// AdminVerifyRequest is the body of POST /api/v1/admin/verify: check
// AdminPassword against rex-svr's admin password without doing anything,
// so a client can unlock its admin features up front (issue #57).
type AdminVerifyRequest struct {
	AdminPassword string `json:"admin_password"`
}

// BulkDeleteRequest is the body of POST /api/v1/admin/delete: tombstone
// every path in Paths at once, gated by rex-svr's admin password. It's the
// deliberate way to delete what the sync client's mass-delete guard refuses
// to push (issue #46).
type BulkDeleteRequest struct {
	Paths         []string `json:"paths"`
	AdminPassword string   `json:"admin_password"`
}

// BulkDeleteResponse is the body of POST /api/v1/admin/delete. Deleted
// lists each tombstoned path with its new revision; Skipped lists paths
// left alone (missing, already deleted, excluded from sync, or handoffs, which are retired by
// resolving them).
type BulkDeleteResponse struct {
	Deleted []DeleteFileResponse `json:"deleted"`
	Skipped []string             `json:"skipped"`
	// Commit is the delete commit's sha, empty if git materialization is
	// unavailable.
	Commit string `json:"commit,omitempty"`
}

// BlameLine is one line of GET /api/v1/blame/{path}: who last changed it,
// as recorded by the server's git history. Commit is empty for a line
// still waiting in the materializer's debounce window.
type BlameLine struct {
	Line    int    `json:"line"` // 1-based
	Commit  string `json:"commit,omitempty"`
	User    string `json:"user"`
	Time    string `json:"time"` // RFC 3339
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

// BlameResponse is the body of GET /api/v1/blame/{path}.
type BlameResponse struct {
	Path  string      `json:"path"`
	Lines []BlameLine `json:"lines"`
}
