package syncclient

import (
	"context"
	"testing"
)

// TestPresenceEmptyWithNoHub confirms the REST presence snapshot degrades to
// an empty list (not an error) when the server has no WS presence hub wired
// up — newTestServer(nil, nil) is exactly that case — since presence is
// purely advisory everywhere else in this codebase.
func TestPresenceEmptyWithNoHub(t *testing.T) {
	client, _ := newTestServer(t)
	leases, err := client.Presence(context.Background())
	if err != nil {
		t.Fatalf("Presence: %v", err)
	}
	if len(leases) != 0 {
		t.Errorf("expected no presence leases from a server with no hub, got %v", leases)
	}
}
