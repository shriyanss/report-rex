package cmd

import (
	"context"
	"crypto/tls"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncserver"
)

var testTLSConfig = &tls.Config{InsecureSkipVerify: true}

func TestHandoffScopeMembership(t *testing.T) {
	const me = "shriyanss"
	given := handoff.Meta{From: "alice", To: me}     // someone gave me work
	sent := handoff.Meta{From: me, To: "alice"}      // I gave someone work
	others := handoff.Meta{From: "bob", To: "carol"} // none of my business

	// Default scope spans both of my directions, and nothing else. This is
	// the rule `rex sync` uses for the handoffs/ namespace.
	def := handoffScope{}
	if !def.allows(given, me) {
		t.Error("default scope must include handoffs given to me")
	}
	if !def.allows(sent, me) {
		t.Error("default scope must include handoffs I gave someone else")
	}
	if def.allows(others, me) {
		t.Error("default scope must exclude handoffs between two other people")
	}

	// --user is queue-shaped: whatever is assigned to that person.
	byUser := handoffScope{User: "carol"}
	if !byUser.allows(others, me) {
		t.Error("--user carol must include handoffs assigned to carol")
	}
	if byUser.allows(given, me) || byUser.allows(sent, me) {
		t.Error("--user carol must not include handoffs assigned elsewhere")
	}

	// --all includes everything, and wins over --user.
	all := handoffScope{All: true}
	both := handoffScope{User: "carol", All: true}
	for _, m := range []handoff.Meta{given, sent, others} {
		if !all.allows(m, me) || !both.allows(m, me) {
			t.Errorf("--all must include %+v", m)
		}
	}
}

func TestHandoffScopeResolvePermission(t *testing.T) {
	const me = "shriyanss"
	given := handoff.Meta{From: "alice", To: me}
	sent := handoff.Meta{From: me, To: "alice"}

	// Deleting resolves only what's in the queue you subscribed to.
	if !(handoffScope{}).canResolve(given, me) {
		t.Error("deleting a handoff assigned to me should resolve it")
	}
	if (handoffScope{}).canResolve(sent, me) {
		t.Error("deleting my copy of a handoff I sent must not resolve it")
	}
	if !(handoffScope{User: "alice"}).canResolve(sent, me) {
		t.Error("--user alice subscribes to alice's queue, so deletion resolves there")
	}
	if !(handoffScope{All: true}).canResolve(sent, me) {
		t.Error("--all should allow resolving anything")
	}
}

func TestHandoffScopeWSPrefix(t *testing.T) {
	// A single-queue subscription narrows the server-side event stream; the
	// default scope spans two directions, so it subscribes to the namespace
	// and filters locally.
	if got := (handoffScope{User: "alice"}).wsPrefix("shriyanss"); got != "handoffs/alice/" {
		t.Errorf("--user prefix = %q", got)
	}
	if got := (handoffScope{}).wsPrefix("shriyanss"); got != "handoffs/" {
		t.Errorf("default prefix = %q", got)
	}
	if got := (handoffScope{All: true}).wsPrefix("shriyanss"); got != "handoffs/" {
		t.Errorf("--all prefix = %q", got)
	}
}

// newHandoffTestServer spins up a real sync server (SQLite + HTTP handlers,
// no git materialization) and returns a logged-in client for username.
func newHandoffTestServer(t *testing.T, username string) (*syncclient.Client, *httptest.Server) {
	t.Helper()
	db, err := syncserver.OpenDB(filepath.Join(t.TempDir(), "server.db"))
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
	client := syncclient.NewClient(u.Hostname(), port, testTLSConfig)
	if _, err := client.Login(context.Background(), username, "test-password"); err != nil {
		t.Fatalf("login: %v", err)
	}
	return client, ts
}

// loginAs returns a second client against the same server, authenticated as
// a different user.
func loginAs(t *testing.T, ts *httptest.Server, username string) *syncclient.Client {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	c := syncclient.NewClient(u.Hostname(), port, testTLSConfig)
	if _, err := c.Login(context.Background(), username, "test-password"); err != nil {
		t.Fatalf("login as %s: %v", username, err)
	}
	return c
}

// syncOnce runs one handoffSyncPass for a workspace, as `rex handoffs sync`
// does. Returns the pass's error so callers can assert on rejection.
func syncOnce(t *testing.T, client *syncclient.Client, dir, username string, scope handoffScope) error {
	t.Helper()
	// The subcommand reads scope from package-level flag state; restore it so
	// tests don't leak into each other.
	prev := handoffsScope
	handoffsScope = scope
	t.Cleanup(func() { handoffsScope = prev })

	flags := syncFlags{Host: "unused", Port: 0, Username: username, Dir: dir}
	local, err := syncclient.OpenLocalStore(dir)
	if err != nil {
		t.Fatalf("OpenLocalStore: %v", err)
	}
	defer local.Close()
	engine := &syncclient.Engine{Dir: dir, Client: client, Local: local, Username: username}
	return handoffSyncPass(context.Background(), flags, client, local, engine, scope)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestHandoffLifecycle walks the issue's example workflow end to end: alice
// writes a handoff with nothing but `to:`, shriyanss syncs it down, edits it,
// then deletes it, and it never comes back.
func TestHandoffLifecycle(t *testing.T) {
	aliceClient, ts := newHandoffTestServer(t, "alice")
	shriClient := loginAs(t, ts, "shriyanss")
	aliceDir, shriDir := t.TempDir(), t.TempDir()
	ctx := context.Background()

	// --- alice creates a handoff, writing only the assignee ---
	write(t, aliceDir, "handoffs/shriyanss/investigate-nextjs-auth.md",
		"---\nto: shriyanss\n---\n\n# Investigate Next.js auth\n\nPlease look at this.\n")
	if err := syncOnce(t, aliceClient, aliceDir, "alice", handoffScope{}); err != nil {
		t.Fatalf("alice sync: %v", err)
	}

	// Rex stamped the identity metadata in place, deriving `from` from the
	// authenticated client rather than trusting the file.
	stamped, err := os.ReadFile(filepath.Join(aliceDir, "handoffs/shriyanss/investigate-nextjs-auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	meta := handoff.ParseMeta(stamped)
	if meta.From != "alice" || meta.To != "shriyanss" || meta.Status != handoff.StatusOpen || meta.ID == "" {
		t.Fatalf("handoff not stamped on create: %+v", meta)
	}
	id := meta.ID

	// --- shriyanss sees it in his queue and syncs it down ---
	entries, _, err := shriClient.ListHandoffs(ctx, "", false, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != id || entries[0].From != "alice" {
		t.Fatalf("shriyanss's queue = %+v", entries)
	}

	if err := syncOnce(t, shriClient, shriDir, "shriyanss", handoffScope{}); err != nil {
		t.Fatalf("shriyanss sync: %v", err)
	}
	local := filepath.Join(shriDir, "handoffs/shriyanss/investigate-nextjs-auth.md")
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("handoff not materialized locally: %v", err)
	}

	// --- he edits it; the change goes back to the server ---
	body := string(stamped) + "\n## Findings\n\nFound /api/admin.\n"
	if err := os.WriteFile(local, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncOnce(t, shriClient, shriDir, "shriyanss", handoffScope{}); err != nil {
		t.Fatalf("shriyanss sync after edit: %v", err)
	}
	d, err := shriClient.GetHandoff(ctx, id)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !strings.Contains(string(d.Content), "Found /api/admin.") {
		t.Errorf("edit did not reach the server: %q", d.Content)
	}

	// --- he deletes the file: that resolves the handoff ---
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	if err := syncOnce(t, shriClient, shriDir, "shriyanss", handoffScope{}); err != nil {
		t.Fatalf("shriyanss sync after delete: %v", err)
	}

	// Gone from the open queue...
	open, _, err := shriClient.ListHandoffs(ctx, "", false, false)
	if err != nil {
		t.Fatalf("list after resolve: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("resolved handoff still open: %+v", open)
	}
	// ...but retained, with its resolution metadata and content intact.
	res, _, err := shriClient.ListHandoffs(ctx, "", false, true)
	if err != nil {
		t.Fatalf("list --resolved: %v", err)
	}
	if len(res) != 1 || res[0].Status != handoff.StatusResolved || res[0].ResolvedBy != "shriyanss" {
		t.Fatalf("--resolved listing = %+v", res)
	}
	d, err = shriClient.GetHandoff(ctx, id)
	if err != nil {
		t.Fatalf("view after resolve: %v", err)
	}
	if !strings.Contains(string(d.Content), "Found /api/admin.") {
		t.Errorf("resolved handoff lost its content: %q", d.Content)
	}

	// --- and it must not be resurrected by the next reconciliation ---
	for i := 0; i < 2; i++ {
		if err := syncOnce(t, shriClient, shriDir, "shriyanss", handoffScope{}); err != nil {
			t.Fatalf("post-resolve sync: %v", err)
		}
		if _, err := os.Stat(local); !os.IsNotExist(err) {
			t.Fatalf("resolved handoff was resurrected (err = %v)", err)
		}
	}

	// alice's own copy is retired too, once she syncs again.
	if err := syncOnce(t, aliceClient, aliceDir, "alice", handoffScope{}); err != nil {
		t.Fatalf("alice sync after resolve: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aliceDir, "handoffs/shriyanss/investigate-nextjs-auth.md")); !os.IsNotExist(err) {
		t.Errorf("alice's copy of a resolved handoff should be retired (err = %v)", err)
	}
}

// TestHandoffSyncRefusesMismatchedMetadata covers the validation the issue
// calls out explicitly: a handoff whose directory and `to` disagree must
// error and must not be synchronized.
func TestHandoffSyncRefusesMismatchedMetadata(t *testing.T) {
	client, _ := newHandoffTestServer(t, "alice")
	dir := t.TempDir()

	write(t, dir, "handoffs/alice/nextjs.md",
		"---\nrex_handoff_id: 8f83c1\nfrom: alice\nto: shriyanss\n---\n\nbody\n")

	err := syncOnce(t, client, dir, "alice", handoffScope{All: true})
	if err == nil {
		t.Fatal("expected a non-nil error for a dir/metadata mismatch")
	}
	if !strings.Contains(err.Error(), "inconsistent metadata") {
		t.Errorf("error = %v", err)
	}

	// Nothing reached the server.
	entries, _, listErr := client.ListHandoffs(context.Background(), "", true, true)
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if len(entries) != 0 {
		t.Errorf("a mismatched handoff was synchronized: %+v", entries)
	}
}

// TestHandoffSyncScopeMaterializesOnlyRequestedQueue is the subscription
// property: syncing your own queue must not drag in someone else's.
func TestHandoffSyncScopeMaterializesOnlyRequestedQueue(t *testing.T) {
	aliceClient, ts := newHandoffTestServer(t, "alice")
	shriClient := loginAs(t, ts, "shriyanss")
	aliceDir, shriDir := t.TempDir(), t.TempDir()

	write(t, aliceDir, "handoffs/shriyanss/mine.md", "---\nto: shriyanss\n---\n\nfor shriyanss\n")
	write(t, aliceDir, "handoffs/bob/theirs.md", "---\nto: bob\n---\n\nfor bob\n")
	if err := syncOnce(t, aliceClient, aliceDir, "alice", handoffScope{}); err != nil {
		t.Fatalf("alice sync: %v", err)
	}

	// Default scope: only shriyanss's queue lands.
	if err := syncOnce(t, shriClient, shriDir, "shriyanss", handoffScope{}); err != nil {
		t.Fatalf("shriyanss sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shriDir, "handoffs/shriyanss/mine.md")); err != nil {
		t.Errorf("own queue not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shriDir, "handoffs/bob/theirs.md")); !os.IsNotExist(err) {
		t.Errorf("another user's queue leaked into the workspace (err = %v)", err)
	}

	// --all materializes everything.
	allDir := t.TempDir()
	if err := syncOnce(t, shriClient, allDir, "shriyanss", handoffScope{All: true}); err != nil {
		t.Fatalf("--all sync: %v", err)
	}
	for _, p := range []string{"handoffs/shriyanss/mine.md", "handoffs/bob/theirs.md"} {
		if _, err := os.Stat(filepath.Join(allDir, filepath.FromSlash(p))); err != nil {
			t.Errorf("--all should materialize %s: %v", p, err)
		}
	}

	// --user materializes just that queue.
	bobDir := t.TempDir()
	if err := syncOnce(t, shriClient, bobDir, "shriyanss", handoffScope{User: "bob"}); err != nil {
		t.Fatalf("--user sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bobDir, "handoffs/bob/theirs.md")); err != nil {
		t.Errorf("--user bob should materialize bob's queue: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bobDir, "handoffs/shriyanss/mine.md")); !os.IsNotExist(err) {
		t.Errorf("--user bob should not materialize shriyanss's queue (err = %v)", err)
	}
}

// TestDeletingSomeoneElsesHandoffDoesNotResolveIt: dropping your own copy of
// a handoff you sent must not finish the assignee's work for them.
func TestDeletingSomeoneElsesHandoffDoesNotResolveIt(t *testing.T) {
	aliceClient, ts := newHandoffTestServer(t, "alice")
	shriClient := loginAs(t, ts, "shriyanss")
	aliceDir := t.TempDir()
	ctx := context.Background()

	write(t, aliceDir, "handoffs/shriyanss/mine.md", "---\nto: shriyanss\n---\n\nfor shriyanss\n")
	if err := syncOnce(t, aliceClient, aliceDir, "alice", handoffScope{}); err != nil {
		t.Fatalf("alice sync: %v", err)
	}
	if err := os.Remove(filepath.Join(aliceDir, "handoffs/shriyanss/mine.md")); err != nil {
		t.Fatal(err)
	}
	if err := syncOnce(t, aliceClient, aliceDir, "alice", handoffScope{}); err != nil {
		t.Fatalf("alice sync after delete: %v", err)
	}

	open, _, err := shriClient.ListHandoffs(ctx, "", false, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("the assignee's handoff should still be open: %+v", open)
	}
}

// syncWorkspaceOnce runs one whole-workspace `rex sync` pass.
func syncWorkspaceOnce(t *testing.T, client *syncclient.Client, dir, username string) error {
	t.Helper()
	flags := syncFlags{Host: "unused", Username: username, Dir: dir}
	local, err := syncclient.OpenLocalStore(dir)
	if err != nil {
		t.Fatalf("OpenLocalStore: %v", err)
	}
	defer local.Close()
	engine := &syncclient.Engine{Dir: dir, Client: client, Local: local, Username: username}
	return reconcileWorkspace(context.Background(), flags, client, local, engine)
}

// TestSyncMaterializesOnlyMyHandoffs is the rule `rex sync` follows for the
// handoffs/ namespace: handoffs given to me, handoffs I gave someone else, and
// nothing else -- while the rest of the workspace syncs in full.
func TestSyncMaterializesOnlyMyHandoffs(t *testing.T) {
	aliceClient, ts := newHandoffTestServer(t, "alice")
	bobClient := loginAs(t, ts, "bob")
	carolClient := loginAs(t, ts, "carol")
	meClient := loginAs(t, ts, "shriyanss")

	// alice gives me work; I give alice work; bob and carol trade work that
	// has nothing to do with me. Plus an ordinary report file.
	aliceDir := t.TempDir()
	write(t, aliceDir, "handoffs/shriyanss/given-to-me.md", "---\nto: shriyanss\n---\n\nfrom alice\n")
	write(t, aliceDir, "reports/vuln-1.md", "# shared report content\n")
	if err := syncWorkspaceOnce(t, aliceClient, aliceDir, "alice"); err != nil {
		t.Fatalf("alice sync: %v", err)
	}

	bobDir := t.TempDir()
	write(t, bobDir, "handoffs/carol/bob-to-carol.md", "---\nto: carol\n---\n\nnot my business\n")
	if err := syncWorkspaceOnce(t, bobClient, bobDir, "bob"); err != nil {
		t.Fatalf("bob sync: %v", err)
	}

	myDir := t.TempDir()
	write(t, myDir, "handoffs/alice/i-sent-this.md", "---\nto: alice\n---\n\nover to you\n")
	if err := syncWorkspaceOnce(t, meClient, myDir, "shriyanss"); err != nil {
		t.Fatalf("my sync: %v", err)
	}
	// A second pass pulls whatever the first pass learned about.
	if err := syncWorkspaceOnce(t, meClient, myDir, "shriyanss"); err != nil {
		t.Fatalf("my second sync: %v", err)
	}

	for _, p := range []string{
		"handoffs/shriyanss/given-to-me.md", // given to me
		"handoffs/alice/i-sent-this.md",     // sent by me
		"reports/vuln-1.md",                 // ordinary shared content
	} {
		if _, err := os.Stat(filepath.Join(myDir, filepath.FromSlash(p))); err != nil {
			t.Errorf("rex sync should materialize %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(myDir, "handoffs/carol/bob-to-carol.md")); !os.IsNotExist(err) {
		t.Errorf("a handoff between two other people must not appear in my workspace (err = %v)", err)
	}

	// Symmetric check: carol sees the one addressed to her, not mine.
	carolDir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := syncWorkspaceOnce(t, carolClient, carolDir, "carol"); err != nil {
			t.Fatalf("carol sync: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(carolDir, "handoffs/carol/bob-to-carol.md")); err != nil {
		t.Errorf("carol should get the handoff addressed to her: %v", err)
	}
	for _, p := range []string{"handoffs/shriyanss/given-to-me.md", "handoffs/alice/i-sent-this.md"} {
		if _, err := os.Stat(filepath.Join(carolDir, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Errorf("carol should not see %s (err = %v)", p, err)
		}
	}
}

// TestSyncRetiresResolvedHandoffs: once a handoff is resolved, `rex sync`
// drops the local copy instead of raising a conflict on a tombstoned path.
func TestSyncRetiresResolvedHandoffs(t *testing.T) {
	aliceClient, ts := newHandoffTestServer(t, "alice")
	meClient := loginAs(t, ts, "shriyanss")
	aliceDir, myDir := t.TempDir(), t.TempDir()
	ctx := context.Background()

	write(t, aliceDir, "handoffs/shriyanss/x.md", "---\nto: shriyanss\n---\n\nwork\n")
	if err := syncWorkspaceOnce(t, aliceClient, aliceDir, "alice"); err != nil {
		t.Fatalf("alice sync: %v", err)
	}
	if err := syncWorkspaceOnce(t, meClient, myDir, "shriyanss"); err != nil {
		t.Fatalf("my sync: %v", err)
	}
	mine := filepath.Join(myDir, "handoffs/shriyanss/x.md")
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("handoff not materialized: %v", err)
	}

	entries, _, err := meClient.ListHandoffs(ctx, "", false, false)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list = %+v, err = %v", entries, err)
	}
	if _, err := meClient.ResolveHandoff(ctx, entries[0].ID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Both sides drop their copy, and it stays gone.
	for i := 0; i < 2; i++ {
		if err := syncWorkspaceOnce(t, meClient, myDir, "shriyanss"); err != nil {
			t.Fatalf("my sync after resolve: %v", err)
		}
		if _, err := os.Stat(mine); !os.IsNotExist(err) {
			t.Fatalf("resolved handoff still present for the assignee (err = %v)", err)
		}
	}
	if err := syncWorkspaceOnce(t, aliceClient, aliceDir, "alice"); err != nil {
		t.Fatalf("alice sync after resolve: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aliceDir, "handoffs/shriyanss/x.md")); !os.IsNotExist(err) {
		t.Errorf("resolved handoff still present for the creator (err = %v)", err)
	}
	// The resolved copy is still readable server-side.
	if res, _, err := meClient.ListHandoffs(ctx, "", false, true); err != nil || len(res) != 1 {
		t.Errorf("resolved listing = %+v, err = %v", res, err)
	}
}
