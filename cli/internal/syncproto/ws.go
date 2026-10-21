package syncproto

// WSMessageType enumerates the "type" field of every WebSocket message
// exchanged on /api/v1/ws.
type WSMessageType string

const (
	WSTypeFileUpdated     WSMessageType = "file_updated"
	WSTypeConflictCreated WSMessageType = "conflict_created"
	WSTypePresence        WSMessageType = "presence"
	WSTypePresenceUpdate  WSMessageType = "presence_update"
	WSTypePresenceRelease WSMessageType = "presence_release"
)

// PresenceState is the editor-reported activity level for a file: someone
// merely has it open (VIEWING) vs. is actively changing it (EDITING).
type PresenceState string

const (
	PresenceViewing PresenceState = "viewing"
	PresenceEditing PresenceState = "editing"
)

// WSFileUpdated is sent server->client after every committed
// materialization (including deletes). It never carries file bytes; the
// client fetches content separately if/when it needs it.
type WSFileUpdated struct {
	Type       WSMessageType `json:"type"`
	Path       string        `json:"path"`
	Revision   int64         `json:"revision"`
	SHA256     string        `json:"sha256,omitempty"`
	ModifiedBy string        `json:"modified_by"`
	Deleted    bool          `json:"deleted"`
}

// WSConflictCreated is sent server->client when a new conflict record is
// created, so other connected clients can proactively surface it.
type WSConflictCreated struct {
	Type       WSMessageType `json:"type"`
	ConflictID int64         `json:"conflict_id"`
	Path       string        `json:"path"`
	Kind       ConflictKind  `json:"kind"`
}

// WSPresence is an advisory, best-effort edit-presence broadcast
// (server->client). It is never required for correctness. Action is
// "active" while the session's lease is live, or "idle" once it expires
// or is explicitly released.
type WSPresence struct {
	Type      WSMessageType `json:"type"`
	Username  string        `json:"username"`
	SessionID string        `json:"session_id"`
	Path      string        `json:"path"`
	State     PresenceState `json:"state,omitempty"`
	Dirty     bool          `json:"dirty,omitempty"`
	Action    string        `json:"action"` // "active" | "idle"
	ExpiresAt string        `json:"expires_at,omitempty"`
}

// WSPresenceUpdate is sent client->server to report (or renew) presence for
// a file under a specific editor session. Every update also renews the
// session's lease, so it doubles as a heartbeat -- there is no separate
// heartbeat message.
type WSPresenceUpdate struct {
	Type      WSMessageType `json:"type"`
	Path      string        `json:"path"`
	SessionID string        `json:"session_id"`
	State     PresenceState `json:"state"`
	Dirty     bool          `json:"dirty"`
}

// WSPresenceRelease is sent client->server to immediately drop a session's
// presence on a file (e.g. on buffer close), rather than waiting for the
// lease to expire.
type WSPresenceRelease struct {
	Type      WSMessageType `json:"type"`
	Path      string        `json:"path"`
	SessionID string        `json:"session_id"`
}

// WSEnvelope is used to sniff the "type" field before unmarshaling into a
// concrete message type.
type WSEnvelope struct {
	Type WSMessageType `json:"type"`
}
