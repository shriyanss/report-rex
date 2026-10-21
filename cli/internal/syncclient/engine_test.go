package syncclient

import (
	"context"
	"crypto/tls"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/syncserver"
)

// newTestServer spins up a real syncserver.Server (SQLite + HTTP handlers,
// no git materialization) behind httptest, and returns a syncclient.Client
// already logged in as "alice".
func newTestServer(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	db, err := syncserver.OpenDB(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	hash, err := syncserver.HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	srv := syncserver.NewServer(syncserver.Config{Addr: ":0", PasswordHash: hash}, db, nil, nil)
	ts := httptest.NewTLSServer(srv.Handler())
	t.Cleanup(ts.Close)

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	client := NewClient(u.Hostname(), port, &tls.Config{InsecureSkipVerify: true})
	ctx := context.Background()
	if _, err := client.Login(ctx, "alice", "test-password"); err != nil {
		t.Fatalf("login: %v", err)
	}
	return client, ts
}

func newTestEngine(t *testing.T, client *Client, dir string) *Engine {
	t.Helper()
	local, err := OpenLocalStore(dir)
	if err != nil {
		t.Fatalf("OpenLocalStore: %v", err)
	}
	t.Cleanup(func() { local.Close() })
	return &Engine{Dir: dir, Client: client, Local: local, Username: "alice"}
}

func TestReconcileInitialPushLocalToEmptyRemote(t *testing.T) {
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		"index.md":         "# Report",
		"assets/proof.png": "PNGDATA",
	})
	engine := newTestEngine(t, client, dir)

	events, err := engine.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var pushed int
	for _, ev := range events {
		if ev.Kind == EventPushed {
			pushed++
		}
	}
	if pushed != 2 {
		t.Fatalf("pushed = %d, want 2; events = %+v", pushed, events)
	}

	manifest, err := client.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Fatalf("remote manifest len = %d, want 2", len(manifest))
	}
}

func TestReconcileInitialPullEmptyLocalFromRemote(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()

	if _, _, err := client.PutFile(ctx, "index.md", []byte("# Existing"), nil); err != nil {
		t.Fatalf("seed remote: %v", err)
	}

	dir := t.TempDir()
	engine := newTestEngine(t, client, dir)

	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var pulled int
	for _, ev := range events {
		if ev.Kind == EventPulled {
			pulled++
		}
	}
	if pulled != 1 {
		t.Fatalf("pulled = %d, want 1; events = %+v", pulled, events)
	}

	got, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(got) != "# Existing" {
		t.Errorf("content = %q, want %q", got, "# Existing")
	}
}

func TestReconcileNoOpWhenIdentical(t *testing.T) {
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "same"})
	engine := newTestEngine(t, client, dir)

	if _, err := engine.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	// Second pass: nothing changed, expect no pushed/pulled events.
	events, err := engine.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	for _, ev := range events {
		if ev.Kind == EventPushed || ev.Kind == EventPulled {
			t.Errorf("unexpected %s event on unchanged path: %+v", ev.Kind, ev)
		}
	}
}

func TestReconcileLocalEditPushesAfterInitialSync(t *testing.T) {
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "v1"})
	engine := newTestEngine(t, client, dir)
	ctx := context.Background()

	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}

	writeFixture(t, dir, map[string]string{"index.md": "v2"})
	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	var sawPush bool
	for _, ev := range events {
		if ev.Kind == EventPushed && ev.Path == "index.md" {
			sawPush = true
		}
	}
	if !sawPush {
		t.Fatalf("expected a pushed event for the local edit; events = %+v", events)
	}

	content, _, _, ok, err := client.GetFile(ctx, "index.md")
	if err != nil || !ok {
		t.Fatalf("GetFile: ok=%v err=%v", ok, err)
	}
	if string(content) != "v2" {
		t.Errorf("remote content = %q, want v2", content)
	}
}

func TestReconcileDivergentEditCreatesConflictNotSilentOverwrite(t *testing.T) {
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "base"})
	engine := newTestEngine(t, client, dir)
	ctx := context.Background()

	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}

	// Remote changes out from under the client (simulating another user).
	rec, _, _, _, err := client.GetFile(ctx, "index.md")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	_ = rec
	manifest, _ := client.Manifest(ctx)
	rev := manifest[0].Revision
	if _, _, err := client.PutFile(ctx, "index.md", []byte("bob's change"), &rev); err != nil {
		t.Fatalf("simulate remote change: %v", err)
	}

	// Local also changes.
	writeFixture(t, dir, map[string]string{"index.md": "alice's change"})

	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var sawConflict bool
	for _, ev := range events {
		if ev.Kind == EventConflict {
			sawConflict = true
		}
	}
	if !sawConflict {
		t.Fatalf("expected a conflict event; events = %+v", events)
	}

	// Remote must still hold bob's change -- never silently overwritten.
	content, _, _, _, err := client.GetFile(ctx, "index.md")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if string(content) != "bob's change" {
		t.Errorf("remote content = %q, want unchanged %q", content, "bob's change")
	}
}

// TestReconcileOfflineThenReconnectPushesAndPullsInOnePass simulates the
// core Phase 8 scenario: the client edits locally while "offline" (no
// server calls happen in between, as if the network were down), and
// independently another user pushes an unrelated file to the server. A
// single reconcile pass after "reconnecting" must both push the offline
// local edit and pull the unrelated remote change -- never silently drop
// either side, and never require the disconnected period to be tracked
// specially since sync_state + fresh manifest + fresh local scan is
// sufficient to reconstruct exactly what happened.
func TestReconcileOfflineThenReconnectPushesAndPullsInOnePass(t *testing.T) {
	client, ts := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "v1"})
	engine := newTestEngine(t, client, dir)
	ctx := context.Background()

	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}

	// "Offline": local edit happens, but nothing is pushed yet.
	writeFixture(t, dir, map[string]string{"index.md": "v2 offline edit"})

	// Meanwhile, an unrelated file appears on the server from another
	// client -- simulated with a second syncclient.Client hitting the same
	// httptest server.
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	bobClient := NewClient(u.Hostname(), port, &tls.Config{InsecureSkipVerify: true})
	if _, err := bobClient.Login(ctx, "bob", "test-password"); err != nil {
		t.Fatalf("bob login: %v", err)
	}
	if _, _, err := bobClient.PutFile(ctx, "reports/new-finding.md", []byte("bob's finding"), nil); err != nil {
		t.Fatalf("bob push: %v", err)
	}

	// "Reconnect": a single reconcile pass must both push the offline
	// local edit and pull bob's unrelated remote change.
	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("reconnect Reconcile: %v", err)
	}
	var sawPush, sawPull bool
	for _, ev := range events {
		if ev.Kind == EventPushed && ev.Path == "index.md" {
			sawPush = true
		}
		if ev.Kind == EventPulled && ev.Path == "reports/new-finding.md" {
			sawPull = true
		}
	}
	if !sawPush {
		t.Errorf("expected offline local edit to push; events = %+v", events)
	}
	if !sawPull {
		t.Errorf("expected bob's remote change to pull; events = %+v", events)
	}

	remoteIndex, _, _, _, err := client.GetFile(ctx, "index.md")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if string(remoteIndex) != "v2 offline edit" {
		t.Errorf("remote index.md = %q, want the offline edit preserved", remoteIndex)
	}
	localFinding, err := os.ReadFile(filepath.Join(dir, "reports/new-finding.md"))
	if err != nil {
		t.Fatalf("read pulled finding: %v", err)
	}
	if string(localFinding) != "bob's finding" {
		t.Errorf("local reports/new-finding.md = %q, want bob's finding", localFinding)
	}
}

// TestReconcileDoesNotSpamDuplicateConflictsOnRepeatedPasses guards against
// a real bug found via e2e testing: a long-running `sync` process calls
// Reconcile() repeatedly (fs events, WS notifications, the fallback
// ticker). Without de-duplication, an unresolved conflict whose local
// content never changes would be re-flagged -- and a brand new server-side
// conflict record created -- on every single pass.
func TestReconcileDoesNotSpamDuplicateConflictsOnRepeatedPasses(t *testing.T) {
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "base"})
	engine := newTestEngine(t, client, dir)
	ctx := context.Background()

	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}

	// Remote changes out from under the client.
	manifest, _ := client.Manifest(ctx)
	rev := manifest[0].Revision
	if _, _, err := client.PutFile(ctx, "index.md", []byte("bob's change"), &rev); err != nil {
		t.Fatalf("simulate remote change: %v", err)
	}
	// Local also changes, diverging.
	writeFixture(t, dir, map[string]string{"index.md": "alice's change"})

	// First reconcile: should flag exactly one new conflict.
	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile (first conflict): %v", err)
	}
	conflictsAfterFirst, err := client.ListConflicts(ctx)
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	if len(conflictsAfterFirst) != 1 {
		t.Fatalf("open conflicts after first pass = %d, want 1", len(conflictsAfterFirst))
	}

	// Several more reconcile passes with nothing changed in between --
	// simulating repeated ticks of a long-running sync process.
	for i := 0; i < 5; i++ {
		if _, err := engine.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (repeat %d): %v", i, err)
		}
	}

	conflictsAfterRepeats, err := client.ListConflicts(ctx)
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	if len(conflictsAfterRepeats) != 1 {
		t.Fatalf("open conflicts after 5 more passes = %d, want still 1 (no duplicate spam)", len(conflictsAfterRepeats))
	}
}

// TestReconcileServerStateLossRepushesInsteadOfDeleting is issue #62's
// client half: a server restarted on a fresh or different database has no
// record of anything (not even tombstones), and treating that as "deleted on
// the server" made every client wipe its workspace.
func TestReconcileServerStateLossRepushesInsteadOfDeleting(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestServer(t)
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{"index.md": "# Report", "a.md": "a", "b.md": "b"})
	engine := newTestEngine(t, client, dir)
	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}
	os.Remove(filepath.Join(dir, "b.md")) // deleted locally while the server is away

	// Same workspace, same local sync state, but the server forgot everything.
	fresh, _ := newTestServer(t)
	engine.Client = fresh
	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile against fresh server: %v", err)
	}
	for _, ev := range events {
		if ev.Kind != EventPushed {
			t.Errorf("unexpected event %+v (want only re-pushes)", ev)
		}
	}
	for _, p := range []string{"index.md", "a.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s was removed locally: %v", p, err)
		}
	}
	manifest, err := fresh.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Errorf("fresh server manifest = %+v, want index.md and a.md re-pushed", manifest)
	}

	// b.md's stale base is gone, so when a teammate re-creates it, it's a
	// plain pull rather than a delete/edit conflict.
	if _, _, err := fresh.PutFile(ctx, "b.md", []byte("b again"), nil); err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	events, err = engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(events) != 1 || events[0].Kind != EventPulled || events[0].Path != "b.md" {
		t.Errorf("events = %+v, want a single pull of b.md", events)
	}
}
