package syncserver

import (
	"sync"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// PresenceTTL is how long an edit-presence lease stays valid without an
// update. PresenceHeartbeatInterval is the interval clients are expected
// to send updates at (informational; the server doesn't enforce it, it
// just expires leases that go quiet).
const (
	PresenceTTL               = 15 * time.Second
	PresenceHeartbeatInterval = 5 * time.Second
)

type presenceKey struct {
	username, sessionID, path string
}

// FilePresence is one editor session's advisory state on one file. It
// intentionally has no WorkspaceID: one rex-svr process serves exactly one
// workspace, so there's nothing to key it by.
// ponytail: add WorkspaceID if/when a single server ever serves multiple
// workspaces.
type FilePresence struct {
	Username  string
	SessionID string
	Path      string
	State     syncproto.PresenceState
	Dirty     bool
	ExpiresAt time.Time
}

// PresenceTracker is best-effort, in-memory, never-persisted edit-presence
// state: "alice/session-a is editing findings/XSS-001.md, unsaved." It is
// advisory only -- nothing in the sync protocol's correctness depends on
// it, and it's never checked before accepting a file write. The clock is
// injectable so lease-expiry math is unit-testable without real sleeps.
type PresenceTracker struct {
	mu     sync.Mutex
	leases map[presenceKey]FilePresence
	now    func() time.Time
}

func NewPresenceTracker() *PresenceTracker {
	return &PresenceTracker{leases: map[presenceKey]FilePresence{}, now: time.Now}
}

// Update (re)starts a session's PresenceTTL lease on path, recording its
// state and dirty flag. Also serves as the lease's heartbeat.
func (p *PresenceTracker) Update(username, sessionID, path string, state syncproto.PresenceState, dirty bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.leases[presenceKey{username, sessionID, path}] = FilePresence{
		Username: username, SessionID: sessionID, Path: path,
		State: state, Dirty: dirty, ExpiresAt: p.now().Add(PresenceTTL),
	}
}

// Release immediately removes a session's lease on path, if any, returning
// true if a lease was actually removed.
func (p *PresenceTracker) Release(username, sessionID, path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := presenceKey{username, sessionID, path}
	if _, ok := p.leases[k]; !ok {
		return false
	}
	delete(p.leases, k)
	return true
}

// Sweep removes every lease that has expired as of now and returns the
// entries that just went idle, so the caller can broadcast an "idle"
// presence event for each.
func (p *PresenceTracker) Sweep() []FilePresence {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	var expired []FilePresence
	for k, entry := range p.leases {
		if !now.Before(entry.ExpiresAt) {
			expired = append(expired, entry)
			delete(p.leases, k)
		}
	}
	return expired
}

// Active returns every currently-live lease.
func (p *PresenceTracker) Active() []FilePresence {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	var active []FilePresence
	for _, entry := range p.leases {
		if now.Before(entry.ExpiresAt) {
			active = append(active, entry)
		}
	}
	return active
}
