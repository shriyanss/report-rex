package syncclient

import (
	"context"
	"testing"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func TestPresenceSessionUpdateIsBroadcastToOtherSessions(t *testing.T) {
	_, host, port := newWSTestServer(t)

	clientA := NewClient(host, port, testTLSConfig)
	_, err := clientA.Login(context.Background(), "alice", "test-password")
	if err != nil {
		t.Fatalf("login alice: %v", err)
	}
	clientB := NewClient(host, port, testTLSConfig)
	_, err = clientB.Login(context.Background(), "bob", "test-password")
	if err != nil {
		t.Fatalf("login bob: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan PresenceEvent, 4)
	connectedA := make(chan struct{}, 1)
	sessA := DialPresence(ctx, host, port, testTLSConfig, clientA, func(ev PresenceEvent) { events <- ev }, nil, func(s ConnStatus, _ error) {
		if s == StatusConnected {
			select {
			case connectedA <- struct{}{}:
			default:
			}
		}
	})
	_ = sessA

	connectedB := make(chan struct{}, 1)
	sessB := DialPresence(ctx, host, port, testTLSConfig, clientB, nil, nil, func(s ConnStatus, _ error) {
		if s == StatusConnected {
			select {
			case connectedB <- struct{}{}:
			default:
			}
		}
	})

	select {
	case <-connectedA:
	case <-time.After(3 * time.Second):
		t.Fatal("alice session never connected")
	}
	select {
	case <-connectedB:
	case <-time.After(3 * time.Second):
		t.Fatal("bob session never connected")
	}

	if err := sessB.Update("findings/XSS-001.md", "bob-session-1", syncproto.PresenceEditing, true); err != nil {
		t.Fatalf("Update: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Username != "bob" || ev.SessionID != "bob-session-1" || ev.Path != "findings/XSS-001.md" ||
			ev.State != syncproto.PresenceEditing || !ev.Dirty || ev.Action != "active" {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("alice never received bob's presence update")
	}

	if err := sessB.Release("findings/XSS-001.md", "bob-session-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Action != "idle" || ev.SessionID != "bob-session-1" {
			t.Errorf("unexpected release event: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("alice never received bob's presence release")
	}
}

func TestPresenceSessionSendFailsOpenWhenDisconnected(t *testing.T) {
	s := &PresenceSession{}
	if err := s.Update("a.md", "session-a", syncproto.PresenceViewing, false); err != ErrPresenceDisconnected {
		t.Errorf("Update on disconnected session = %v, want ErrPresenceDisconnected", err)
	}
	if err := s.Release("a.md", "session-a"); err != ErrPresenceDisconnected {
		t.Errorf("Release on disconnected session = %v, want ErrPresenceDisconnected", err)
	}
}
