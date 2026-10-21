package syncclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
)

// TestReconcilePathFilterLeavesOutOfScopePathsAlone is the property that
// makes scoped handoff sync possible: a filtered path is neither pushed nor
// pulled nor deleted, so a client can materialize one slice of the workspace
// without disturbing the rest.
func TestReconcilePathFilterLeavesOutOfScopePathsAlone(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		// Stamped the way `rex handoffs sync` stamps them -- the server
		// rejects a handoff that arrives without its identity metadata.
		"handoffs/alice/mine.md": "---\nrex_handoff_id: aaa111\nfrom: alice\nto: alice\nstatus: open\n---\n\nmine\n",
		"handoffs/bob/theirs.md": "---\nrex_handoff_id: bbb222\nfrom: alice\nto: bob\nstatus: open\n---\n\ntheirs\n",
		"reports/vuln-1.md":      "# unrelated",
	})

	engine := newTestEngine(t, client, dir)
	engine.PathFilter = func(p string) bool {
		return strings.HasPrefix(p, handoff.ScopePrefix("alice"))
	}

	events, err := engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	for _, ev := range events {
		if ev.Kind == EventSkipped {
			t.Fatalf("unexpected skip: %+v", ev)
		}
	}

	manifest, err := client.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != 1 || manifest[0].Path != "handoffs/alice/mine.md" {
		t.Fatalf("remote manifest = %+v, want only handoffs/alice/mine.md", manifest)
	}

	// Out-of-scope local files are untouched, not deleted.
	for _, p := range []string{"handoffs/bob/theirs.md", "reports/vuln-1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s should be left alone: %v", p, err)
		}
	}
}

// TestReconcileDoesNotResurrectFilteredRemotePath covers the resolved-handoff
// case: the canonical path still exists server-side, but a filter that
// excludes it means reconciliation never materializes it again.
func TestReconcileDoesNotResurrectFilteredRemotePath(t *testing.T) {
	client, _ := newTestServer(t)
	ctx := context.Background()
	dir := t.TempDir()

	remote := "handoffs/alice/resolved.md"
	if _, _, err := client.PutFile(ctx, remote, []byte("---\nto: alice\nrex_handoff_id: aaa111\nfrom: alice\n---\n\nbody\n"), nil); err != nil {
		t.Fatalf("seed remote: %v", err)
	}

	engine := newTestEngine(t, client, dir)
	engine.PathFilter = func(p string) bool { return p != remote }

	if _, err := engine.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, remote)); !os.IsNotExist(err) {
		t.Errorf("a filtered remote path must not be materialized locally (err = %v)", err)
	}
}
