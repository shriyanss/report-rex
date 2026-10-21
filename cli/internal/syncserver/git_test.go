package syncserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH, skipping real-git integration test")
	}
}

func newTestMaterializer(t *testing.T) (*GitMaterializer, *Store) {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "report-rex.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)

	workspace := filepath.Join(dir, "workspace")
	m, err := NewGitMaterializer(workspace, db, store)
	if err != nil {
		t.Fatalf("NewGitMaterializer: %v", err)
	}
	return m, store
}

func TestGitMaterializerInitCreatesRepo(t *testing.T) {
	m, _ := newTestMaterializer(t)
	if _, err := os.Stat(filepath.Join(m.workspace, ".git")); err != nil {
		t.Errorf(".git not created: %v", err)
	}
}

func TestGitMaterializerFlushWritesAndCommits(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, err := store.Put(ctx, "index.md", []byte("# Hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	m.Enqueue("index.md", "write", rec.Revision, "alice")

	m.Flush(ctx)

	full := filepath.Join(m.workspace, "index.md")
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if string(got) != "# Hello" {
		t.Errorf("materialized content = %q, want %q", got, "# Hello")
	}

	log, err := m.runner.Run("log", "--oneline")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if log == "" {
		t.Error("expected at least one commit in git log")
	}

	if m.PendingCount() != 0 {
		t.Errorf("PendingCount() = %d, want 0 after successful flush", m.PendingCount())
	}
}

func TestGitMaterializerFlushHandlesDelete(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, _ := store.Put(ctx, "a.md", []byte("content"), nil, "alice")
	m.Enqueue("a.md", "write", rec.Revision, "alice")
	m.Flush(ctx)

	del, err := store.Delete(ctx, "a.md", rec.Revision, "alice")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	m.Enqueue("a.md", "delete", del.Revision, "alice")
	m.Flush(ctx)

	if _, err := os.Stat(filepath.Join(m.workspace, "a.md")); !os.IsNotExist(err) {
		t.Errorf("expected a.md removed from workspace, stat err = %v", err)
	}
}

func TestGitMaterializerDriftRepairRewritesFromDB(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, _ := store.Put(ctx, "index.md", []byte("canonical"), nil, "alice")
	m.Enqueue("index.md", "write", rec.Revision, "alice")
	m.Flush(ctx)

	// Simulate manual tampering with the server-managed git tree.
	full := filepath.Join(m.workspace, "index.md")
	if err := os.WriteFile(full, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	repaired, err := m.DriftCheckAndRepair(ctx)
	if err != nil {
		t.Fatalf("DriftCheckAndRepair: %v", err)
	}
	if repaired != 1 {
		t.Errorf("repaired = %d, want 1", repaired)
	}

	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "canonical" {
		t.Errorf("content after repair = %q, want %q (DB must win)", got, "canonical")
	}
}

func TestGitMaterializerDriftRepairRemovesUnexpectedFile(t *testing.T) {
	m, _ := newTestMaterializer(t)

	stray := filepath.Join(m.workspace, "stray.md")
	if err := os.WriteFile(stray, []byte("shouldn't be here"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	repaired, err := m.DriftCheckAndRepair(context.Background())
	if err != nil {
		t.Fatalf("DriftCheckAndRepair: %v", err)
	}
	if repaired != 1 {
		t.Errorf("repaired = %d, want 1", repaired)
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("expected stray.md removed, stat err = %v", err)
	}
}

func TestGitMaterializerDriftRepairNoOpWhenClean(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, _ := store.Put(ctx, "index.md", []byte("canonical"), nil, "alice")
	m.Enqueue("index.md", "write", rec.Revision, "alice")
	m.Flush(ctx)

	repaired, err := m.DriftCheckAndRepair(ctx)
	if err != nil {
		t.Fatalf("DriftCheckAndRepair: %v", err)
	}
	if repaired != 0 {
		t.Errorf("repaired = %d, want 0 on a clean workspace", repaired)
	}
}

// TestGitMaterializerDriftRepairRefusesMassDelete simulates a DB-manifest
// read anomaly (not real drift): every synced file is still correctly on
// disk, but the files table comes back empty. Without a safety guard, the
// old drift-delete loop would treat every on-disk path as "unwanted" and
// wipe the entire workspace in one commit -- exactly the incident reported
// in issue #37. The guard must refuse the delete and leave the workspace
// untouched.
func TestGitMaterializerDriftRepairRefusesMassDelete(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	const n = 10
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("reports/vuln-%d.md", i)
		rec, err := store.Put(ctx, path, []byte(fmt.Sprintf("finding %d", i)), nil, "alice")
		if err != nil {
			t.Fatalf("Put %s: %v", path, err)
		}
		m.Enqueue(path, "write", rec.Revision, "alice")
	}
	m.Flush(ctx)

	// Simulate the manifest anomaly: the files table loses every row while
	// the on-disk workspace (and its git history) is left exactly as-is.
	if _, err := m.db.Exec(`DELETE FROM files`); err != nil {
		t.Fatalf("simulate manifest anomaly: %v", err)
	}

	// Refused as an error, not just logged: serving the emptier manifest is
	// what made every client delete its own copy in issue #62.
	repaired, err := m.DriftCheckAndRepair(ctx)
	if err == nil {
		t.Fatal("DriftCheckAndRepair succeeded, want an error refusing the mass delete")
	}
	if repaired != 0 {
		t.Errorf("repaired = %d, want 0 (mass delete must be refused)", repaired)
	}
	for i := 0; i < n; i++ {
		path := filepath.Join(m.workspace, fmt.Sprintf("reports/vuln-%d.md", i))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to survive the refused drift repair, stat err = %v", path, err)
		}
	}
}

// TestGitMaterializerDriftRepairMixedMassDeleteGuard checks the guard's
// count-and-fraction threshold in a mixed scenario: a file the DB still
// knows about alongside a majority of stray files it doesn't. Deleting them
// would cross the mass-delete threshold, so the whole pass is refused before
// anything is touched -- strays and the tampered known file alike. Small-scale
// legitimate cleanup (TestGitMaterializerDriftRepairRemovesUnexpectedFile)
// must keep working.
func TestGitMaterializerDriftRepairMixedMassDeleteGuard(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, _ := store.Put(ctx, "index.md", []byte("canonical"), nil, "alice")
	m.Enqueue("index.md", "write", rec.Revision, "alice")
	m.Flush(ctx)

	// Tamper with the one DB-known file so it also needs a repair-write.
	knownPath := filepath.Join(m.workspace, "index.md")
	if err := os.WriteFile(knownPath, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	var strays []string
	for i := 0; i < 5; i++ {
		stray := filepath.Join(m.workspace, fmt.Sprintf("stray-%d.md", i))
		if err := os.WriteFile(stray, []byte("shouldn't be here"), 0o644); err != nil {
			t.Fatalf("write stray file: %v", err)
		}
		strays = append(strays, stray)
	}

	repaired, err := m.DriftCheckAndRepair(ctx)
	if err == nil {
		t.Fatal("DriftCheckAndRepair succeeded, want an error refusing the mass delete")
	}
	if repaired != 0 {
		t.Errorf("repaired = %d, want 0 (a refused pass must not half-repair)", repaired)
	}
	got, err := os.ReadFile(knownPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "tampered" {
		t.Errorf("content after refused repair = %q, want it untouched", got)
	}
	for _, stray := range strays {
		if _, err := os.Stat(stray); err != nil {
			t.Errorf("expected stray %s to survive (guard should have tripped), stat err = %v", stray, err)
		}
	}
}

// TestGitMaterializerDriftRepairCommitListsPaths verifies the drift-repair
// commit is self-diagnosing: it must list the actual paths touched, not
// just a bare count, so a future incident is diagnosable from `git log`
// alone.
func TestGitMaterializerDriftRepairCommitListsPaths(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, _ := store.Put(ctx, "index.md", []byte("canonical"), nil, "alice")
	m.Enqueue("index.md", "write", rec.Revision, "alice")
	strayRec, _ := store.Put(ctx, "stray.md", []byte("was legit, DB forgot it"), nil, "alice")
	m.Enqueue("stray.md", "write", strayRec.Revision, "alice")
	m.Flush(ctx)

	if err := os.WriteFile(filepath.Join(m.workspace, "index.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	// Simulate the DB losing track of a single already-committed file --
	// small-scale, well under the mass-delete threshold, so this must still
	// be repaired (removed) and show up in the commit.
	if _, err := m.db.Exec(`DELETE FROM files WHERE path = ?`, "stray.md"); err != nil {
		t.Fatalf("simulate single dropped row: %v", err)
	}

	if _, err := m.DriftCheckAndRepair(ctx); err != nil {
		t.Fatalf("DriftCheckAndRepair: %v", err)
	}

	body, err := m.runner.Run("log", "-1", "--format=%B")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(body, "wrote index.md") {
		t.Errorf("commit body missing repaired-write path, got:\n%s", body)
	}
	if !strings.Contains(body, "removed stray.md") {
		t.Errorf("commit body missing repaired-delete path, got:\n%s", body)
	}
}

// TestPendingMaterializationsSurviveCrash simulates a process crash
// between the DB accept and the git commit: a pending_materializations row
// exists with no corresponding commit. A fresh materializer pointed at the
// same DB+workspace must drain it on the next Flush (standing in for
// startup recovery).
func TestPendingMaterializationsSurviveCrash(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, err := store.Put(ctx, "index.md", []byte("recovered content"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Insert the pending row directly, simulating Enqueue having run but
	// the process crashing before Flush ever executed.
	if _, err := m.db.Exec(`INSERT INTO pending_materializations (path, revision, op, created_at) VALUES (?, ?, ?, ?)`,
		"index.md", rec.Revision, "write", now()); err != nil {
		t.Fatalf("insert pending row: %v", err)
	}

	if got := m.PendingCount(); got != 1 {
		t.Fatalf("PendingCount() = %d, want 1 before recovery", got)
	}

	m.Flush(ctx)

	if got := m.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0 after recovery flush", got)
	}
	full := filepath.Join(m.workspace, "index.md")
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read recovered file: %v", err)
	}
	if string(got) != "recovered content" {
		t.Errorf("recovered content = %q, want %q", got, "recovered content")
	}
}

// TestCheckWorkspaceLayout covers issue #62: `rex-svr --workspace .` put the
// default ./rex-svr.db inside the workspace, and the first start's drift
// check deleted the live database.
func TestCheckWorkspaceLayout(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	outside := func(name string) string { return filepath.Join(dir, name) }
	cases := []struct {
		name             string
		ws, db, crt, key string
		wantFlag         string
	}{
		{"all outside", ws, outside("rex.db"), outside("c.crt"), outside("c.key"), ""},
		{"db inside", ws, filepath.Join(ws, "rex-svr.db"), outside("c.crt"), outside("c.key"), "--db"},
		{"db nested inside", ws, filepath.Join(ws, "state", "rex.db"), outside("c.crt"), outside("c.key"), "--db"},
		{"key inside", ws, outside("rex.db"), outside("c.crt"), filepath.Join(ws, "rex-svr.key"), "--key"},
		{"workspace is the parent dir", dir, outside("rex.db"), outside("c.crt"), outside("c.key"), "--db"},
		{"sibling with shared prefix", ws, filepath.Join(dir, "ws2", "rex.db"), outside("c.crt"), outside("c.key"), ""},
	}
	for _, c := range cases {
		err := CheckWorkspaceLayout(c.ws, c.db, c.crt, c.key)
		if c.wantFlag == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantFlag) {
			t.Errorf("%s: error = %v, want one naming %s", c.name, err, c.wantFlag)
		}
	}

	// `--workspace .` with the defaults, relative to the working directory.
	t.Chdir(dir)
	if err := CheckWorkspaceLayout(".", "./rex-svr.db", "./rex-svr.crt", "./rex-svr.key"); err == nil {
		t.Error("--workspace . with default --db must be refused")
	}
}

// TestNewGitMaterializerRefusesForeignDirectory: pointing --workspace at an
// existing report folder used to delete its files on the first start (fewer
// than driftDeleteMinCount of them slipped under the mass-delete guard).
func TestNewGitMaterializerRefusesForeignDirectory(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "rex.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)

	plain := filepath.Join(dir, "report")
	os.MkdirAll(plain, 0o755)
	os.WriteFile(filepath.Join(plain, "index.md"), []byte("# mine"), 0o644)
	if _, err := NewGitMaterializer(plain, db, store); err == nil {
		t.Error("non-empty directory without .git must be refused")
	}

	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, err := NewGitMaterializer(repo, db, store); err == nil {
		t.Error("a git repository rex-svr didn't create must be refused")
	}

	if _, err := os.Stat(filepath.Join(plain, "index.md")); err != nil {
		t.Errorf("existing file must survive: %v", err)
	}

	// Empty (or only junk the sync ignores) is fine, and reopening our own
	// workspace is fine.
	fresh := filepath.Join(dir, "fresh")
	os.MkdirAll(fresh, 0o755)
	os.WriteFile(filepath.Join(fresh, ".DS_Store"), nil, 0o644)
	if _, err := NewGitMaterializer(fresh, db, store); err != nil {
		t.Fatalf("empty workspace: %v", err)
	}
	if _, err := NewGitMaterializer(fresh, db, store); err != nil {
		t.Errorf("reopening rex-svr's own workspace: %v", err)
	}
}

func TestGitMaterializerBlameAttributesUsers(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()

	rec, err := store.Put(ctx, "a.md", []byte("one\ntwo\n"), nil, "alice")
	if err != nil {
		t.Fatal(err)
	}
	m.Enqueue("a.md", "write", rec.Revision, "alice")
	m.Flush(ctx)
	base := rec.Revision
	rec, err = store.Put(ctx, "a.md", []byte("one\nTWO\nthree\n"), &base, "bob")
	if err != nil {
		t.Fatal(err)
	}
	m.Enqueue("a.md", "write", rec.Revision, "bob")
	m.Flush(ctx)

	lines, err := m.Blame(ctx, "a.md")
	if err != nil {
		t.Fatalf("Blame: %v", err)
	}
	want := []struct{ user, text string }{{"alice", "one"}, {"bob", "TWO"}, {"bob", "three"}}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, w := range want {
		l := lines[i]
		if l.Line != i+1 || l.User != w.user || l.Text != w.text || l.Commit == "" || l.Time == "" {
			t.Errorf("line %d = %+v, want user %s text %q", i+1, l, w.user, w.text)
		}
	}
}

func TestCommitUser(t *testing.T) {
	body := "sync: 2 file(s) by alice, bob\n\n- modified a.md (alice)\n- modified b/a.md (bob)\n"
	for path, want := range map[string]string{"a.md": "alice", "b/a.md": "bob", "c.md": "rex-svr"} {
		if got := commitUser(body, path); got != want {
			t.Errorf("commitUser(%s) = %q, want %q", path, got, want)
		}
	}
	if got := commitUser("rex-svr: startup drift repair (1 path(s))\n\n- wrote a.md\n", "a.md"); got != "repair" {
		t.Errorf("drift repair user = %q", got)
	}
}
