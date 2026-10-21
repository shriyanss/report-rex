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
)

// Regression tests for issue #42 (multi-user sync under load).

func loginAs(t *testing.T, ts *httptest.Server, user string) *Client {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	c := NewClient(u.Hostname(), port, &tls.Config{InsecureSkipVerify: true})
	if _, err := c.Login(context.Background(), user, "test-password"); err != nil {
		t.Fatalf("login %s: %v", user, err)
	}
	return c
}

// twoUsers returns alice's and bob's engines over one server, both synced
// to the same initial files.
func twoUsers(t *testing.T, files map[string]string) (a, b *Engine, client *Client) {
	t.Helper()
	client, ts := newTestServer(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	writeFixture(t, dirA, files)
	a = newTestEngine(t, client, dirA)
	b = newTestEngine(t, loginAs(t, ts, "bob"), dirB)
	b.Username = "bob"
	mustReconcile(t, a)
	mustReconcile(t, b)
	return a, b, client
}

func mustReconcile(t *testing.T, e *Engine) []Event {
	t.Helper()
	evs, err := e.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile (%s): %v", e.Username, err)
	}
	return evs
}

func kinds(evs []Event, path string) []EventKind {
	var out []EventKind
	for _, ev := range evs {
		if ev.Path == path {
			out = append(out, ev.Kind)
		}
	}
	return out
}

func readLocal(t *testing.T, e *Engine, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.Dir, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func openConflicts(t *testing.T, c *Client) int {
	t.Helper()
	cs, err := c.ListConflicts(context.Background())
	if err != nil {
		t.Fatalf("ListConflicts: %v", err)
	}
	return len(cs)
}

// The issue's exact report: someone deleted index.md, and re-creating it
// produced a conflict. Two bugs combined: the deleter's own long-running
// engine never pushed the delete (a missing lastConflictHash key compared
// equal to a deleted file's empty hash), and a client that pulled the
// delete forgot the tombstone revision, so its re-create was rejected.
func TestReconcileDeleteThenRecreateOnAnotherClient(t *testing.T) {
	a, b, client := twoUsers(t, map[string]string{"index.md": "# v1\n"})

	if err := os.Remove(filepath.Join(a.Dir, "index.md")); err != nil {
		t.Fatal(err)
	}
	if got := kinds(mustReconcile(t, a), "index.md"); len(got) != 1 || got[0] != EventPushed {
		t.Fatalf("alice's delete: events = %v, want [pushed]", got)
	}
	if got := kinds(mustReconcile(t, b), "index.md"); len(got) != 1 || got[0] != EventDeleted {
		t.Fatalf("bob pulling the delete: events = %v, want [deleted]", got)
	}

	writeFixture(t, b.Dir, map[string]string{"index.md": "# re-created\n"})
	if got := kinds(mustReconcile(t, b), "index.md"); len(got) != 1 || got[0] != EventPushed {
		t.Fatalf("bob's re-create: events = %v, want [pushed]", got)
	}
	if n := openConflicts(t, client); n != 0 {
		t.Fatalf("open conflicts = %d, want 0", n)
	}
	mustReconcile(t, a)
	if got := readLocal(t, a, "index.md"); got != "# re-created\n" {
		t.Errorf("alice's copy = %q, want the re-created content", got)
	}
}

func TestReconcileAutoMergesNonOverlappingEdits(t *testing.T) {
	a, b, client := twoUsers(t, map[string]string{"notes.md": "one\ntwo\nthree\n"})

	writeFixture(t, b.Dir, map[string]string{"notes.md": "ONE (bob)\ntwo\nthree\n"})
	mustReconcile(t, b)
	writeFixture(t, a.Dir, map[string]string{"notes.md": "one\ntwo\nTHREE (alice)\n"})

	evs := mustReconcile(t, a)
	if got := kinds(evs, "notes.md"); len(got) != 1 || got[0] != EventMerged {
		t.Fatalf("alice: events = %v, want [merged]", got)
	}
	want := "ONE (bob)\ntwo\nTHREE (alice)\n"
	if got := readLocal(t, a, "notes.md"); got != want {
		t.Errorf("alice's merged copy = %q, want %q", got, want)
	}
	content, _, _, _, err := client.GetFile(context.Background(), "notes.md")
	if err != nil || string(content) != want {
		t.Errorf("server copy = %q (%v), want %q", content, err, want)
	}
	if n := openConflicts(t, client); n != 0 {
		t.Errorf("open conflicts = %d, want 0 (auto-merge resolves it)", n)
	}
	mustReconcile(t, b)
	if got := readLocal(t, b, "notes.md"); got != want {
		t.Errorf("bob's copy = %q, want %q", got, want)
	}
	// And alice is in sync afterwards: her next edit is a plain push.
	writeFixture(t, a.Dir, map[string]string{"notes.md": want + "four\n"})
	if got := kinds(mustReconcile(t, a), "notes.md"); len(got) != 1 || got[0] != EventPushed {
		t.Errorf("alice's follow-up edit: events = %v, want [pushed]", got)
	}
}

// Overlapping edits can't be merged; each further local save must update
// the one open conflict for (path, user) rather than file another.
func TestReconcileOverlappingEditsKeepOneOpenConflict(t *testing.T) {
	a, b, client := twoUsers(t, map[string]string{"hot.md": "same line\n"})

	writeFixture(t, b.Dir, map[string]string{"hot.md": "bob's line\n"})
	mustReconcile(t, b)

	var last string
	for i := 0; i < 5; i++ {
		last = "alice's line v" + strconv.Itoa(i) + "\n"
		writeFixture(t, a.Dir, map[string]string{"hot.md": last})
		if got := kinds(mustReconcile(t, a), "hot.md"); len(got) != 1 || got[0] != EventConflict {
			t.Fatalf("pass %d: events = %v, want [conflict]", i, got)
		}
	}
	cs, err := client.ListConflicts(context.Background())
	if err != nil || len(cs) != 1 {
		t.Fatalf("open conflicts = %d (%v), want exactly 1", len(cs), err)
	}
	if cs[0].LocalUsername != "alice" {
		t.Errorf("conflict owner = %q, want alice", cs[0].LocalUsername)
	}
	d, err := client.GetConflict(context.Background(), cs[0].ID, true, false, false)
	if err != nil || string(d.LocalContent) != last {
		t.Errorf("conflict local content = %q (%v), want the latest save %q", d.LocalContent, err, last)
	}
}

// Local and remote converging on their own (the same edit on both sides,
// or a `rex resolve`) must refresh the base, or the next local edit is a
// false conflict.
func TestReconcileConvergedStateRefreshesBase(t *testing.T) {
	a, b, client := twoUsers(t, map[string]string{"x.md": "v1\n"})

	writeFixture(t, b.Dir, map[string]string{"x.md": "v2\n"})
	mustReconcile(t, b)
	writeFixture(t, a.Dir, map[string]string{"x.md": "v2\n"})
	if got := kinds(mustReconcile(t, a), "x.md"); len(got) != 0 {
		t.Fatalf("converged pass: events = %v, want none", got)
	}

	writeFixture(t, a.Dir, map[string]string{"x.md": "v3\n"})
	if got := kinds(mustReconcile(t, a), "x.md"); len(got) != 1 || got[0] != EventPushed {
		t.Fatalf("edit after converging: events = %v, want [pushed]", got)
	}
	if n := openConflicts(t, client); n != 0 {
		t.Errorf("open conflicts = %d, want 0", n)
	}
}
