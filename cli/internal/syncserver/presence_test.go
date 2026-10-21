package syncserver

import (
	"testing"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func TestPresenceTrackerUpdateThenExpire(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := NewPresenceTracker()
	p.now = func() time.Time { return now }

	p.Update("alice", "session-a", "findings/XSS-001.md", syncproto.PresenceViewing, false)

	active := p.Active()
	if len(active) != 1 || active[0].Username != "alice" || active[0].SessionID != "session-a" || active[0].Path != "findings/XSS-001.md" {
		t.Fatalf("Active() = %+v, want one alice/session-a/findings/XSS-001.md lease", active)
	}

	// Advance time past the TTL without another update.
	now = now.Add(PresenceTTL + time.Second)

	expired := p.Sweep()
	if len(expired) != 1 || expired[0].Username != "alice" || expired[0].Path != "findings/XSS-001.md" {
		t.Fatalf("Sweep() = %+v, want the alice lease to have expired", expired)
	}
	if len(p.Active()) != 0 {
		t.Errorf("Active() after sweep = %v, want empty", p.Active())
	}
}

func TestPresenceTrackerUpdateRenewsLease(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := NewPresenceTracker()
	p.now = func() time.Time { return now }

	p.Update("bob", "session-b", "index.md", syncproto.PresenceViewing, false)

	// Advance partway through the TTL and renew.
	now = now.Add(PresenceHeartbeatInterval)
	p.Update("bob", "session-b", "index.md", syncproto.PresenceEditing, true)

	// Advance past where the lease would have expired if NOT renewed
	// (original update + TTL), but still within TTL of the renewal.
	now = now.Add(PresenceTTL - PresenceHeartbeatInterval/2)

	if expired := p.Sweep(); len(expired) != 0 {
		t.Errorf("Sweep() = %+v, want no expirations (lease was renewed)", expired)
	}
	active := p.Active()
	if len(active) != 1 || active[0].State != syncproto.PresenceEditing || !active[0].Dirty {
		t.Errorf("Active() = %+v, want renewed editing/dirty lease", active)
	}
}

func TestPresenceTrackerIndependentPathsPerUser(t *testing.T) {
	now := time.Now()
	p := NewPresenceTracker()
	p.now = func() time.Time { return now }

	p.Update("alice", "session-a", "a.md", syncproto.PresenceViewing, false)
	p.Update("alice", "session-a", "b.md", syncproto.PresenceViewing, false)
	p.Update("bob", "session-b", "a.md", syncproto.PresenceViewing, false)

	if len(p.Active()) != 3 {
		t.Fatalf("Active() len = %d, want 3", len(p.Active()))
	}
}

func TestPresenceTrackerMultipleSessionsPerUser(t *testing.T) {
	now := time.Now()
	p := NewPresenceTracker()
	p.now = func() time.Time { return now }

	// The same user/file, two distinct editor sessions -- these must not
	// collapse into a single lease.
	p.Update("alice", "session-a", "a.md", syncproto.PresenceViewing, false)
	p.Update("alice", "session-b", "a.md", syncproto.PresenceEditing, true)

	if len(p.Active()) != 2 {
		t.Fatalf("Active() len = %d, want 2 (one per session)", len(p.Active()))
	}
}

func TestPresenceTrackerRelease(t *testing.T) {
	now := time.Now()
	p := NewPresenceTracker()
	p.now = func() time.Time { return now }

	p.Update("alice", "session-a", "a.md", syncproto.PresenceEditing, true)
	if !p.Release("alice", "session-a", "a.md") {
		t.Fatal("Release() = false, want true for an active lease")
	}
	if len(p.Active()) != 0 {
		t.Errorf("Active() after release = %v, want empty", p.Active())
	}
	if p.Release("alice", "session-a", "a.md") {
		t.Error("Release() = true on an already-released lease, want false")
	}
}
