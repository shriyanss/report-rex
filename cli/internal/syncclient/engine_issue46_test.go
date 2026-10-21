package syncclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// syncTen pushes ten files and returns the engine plus the workspace dir.
func syncTen(t *testing.T) (*Engine, *Client, string) {
	t.Helper()
	client, _ := newTestServer(t)
	dir := t.TempDir()
	files := map[string]string{}
	for i := 0; i < 10; i++ {
		files[fmt.Sprintf("reports/vuln-%d.md", i)] = fmt.Sprintf("finding %d", i)
	}
	writeFixture(t, dir, files)
	engine := newTestEngine(t, client, dir)
	if _, err := engine.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile: %v", err)
	}
	return engine, client, dir
}

func liveCount(t *testing.T, client *Client) int {
	t.Helper()
	manifest, err := client.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	n := 0
	for _, f := range manifest {
		if !f.Deleted {
			n++
		}
	}
	return n
}

func TestReconcileRefusesMassDelete(t *testing.T) {
	engine, client, dir := syncTen(t)
	if err := os.RemoveAll(filepath.Join(dir, "reports")); err != nil {
		t.Fatal(err)
	}
	events, err := engine.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var skipped int
	for _, ev := range events {
		if ev.Kind == EventPushed {
			t.Fatalf("pushed %s while mass delete should be refused", ev.Path)
		}
		if ev.Kind == EventSkipped && ev.Path == "" {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("want one mass-delete warning, got events %+v", events)
	}
	if n := liveCount(t, client); n != 10 {
		t.Fatalf("server has %d live files, want 10", n)
	}
}

func TestReconcileStillPushesSmallDelete(t *testing.T) {
	engine, client, dir := syncTen(t)
	for _, i := range []int{0, 1} {
		if err := os.Remove(filepath.Join(dir, "reports", fmt.Sprintf("vuln-%d.md", i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n := liveCount(t, client); n != 8 {
		t.Fatalf("server has %d live files, want 8", n)
	}
}

func TestReconcileMovedWorkspaceDeletesNothing(t *testing.T) {
	engine, client, dir := syncTen(t)
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(dir+"-moved", dir) })
	if _, err := engine.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile on a moved-away workspace: want error, got nil")
	}
	if n := liveCount(t, client); n != 10 {
		t.Fatalf("server has %d live files, want 10", n)
	}
}
