package syncserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestStorePutCreatesRevisionOne(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if rec.Revision != 1 {
		t.Errorf("Revision = %d, want 1", rec.Revision)
	}
	if rec.ModifiedBy != "alice" {
		t.Errorf("ModifiedBy = %q, want alice", rec.ModifiedBy)
	}
}

func TestStorePutRequiresBaseRevisionForExisting(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	if _, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// No base revision on an existing path is treated as stale.
	if _, err := s.Put(ctx, "index.md", []byte("world"), nil, "bob"); !errors.Is(err, ErrStaleBase) {
		t.Errorf("Put without base on existing path: err = %v, want ErrStaleBase", err)
	}
}

func TestStorePutAcceptsMatchingBaseRevision(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	rec2, err := s.Put(ctx, "index.md", []byte("world"), &rec.Revision, "bob")
	if err != nil {
		t.Fatalf("Put with matching base: %v", err)
	}
	if rec2.Revision != 2 {
		t.Errorf("Revision = %d, want 2", rec2.Revision)
	}
}

func TestStorePutRejectsStaleBaseRevision(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(ctx, "index.md", []byte("v2"), &rec.Revision, "bob"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// alice tries to push again using the now-stale revision 1.
	if _, err := s.Put(ctx, "index.md", []byte("v3-from-alice"), &rec.Revision, "alice"); !errors.Is(err, ErrStaleBase) {
		t.Errorf("Put with stale base: err = %v, want ErrStaleBase", err)
	}
}

func TestStoreDeleteTombstones(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	del, err := s.Delete(ctx, "index.md", rec.Revision, "alice")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !del.Deleted {
		t.Error("expected Deleted = true")
	}

	got, err := s.Get(ctx, "index.md")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Deleted {
		t.Error("Get after delete: expected Deleted = true")
	}
}

func TestStoreDeleteIdempotentAtMatchingRevision(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	d1, err := s.Delete(ctx, "index.md", rec.Revision, "alice")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	d2, err := s.Delete(ctx, "index.md", d1.Revision, "alice")
	if err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if d1.Revision != d2.Revision {
		t.Errorf("idempotent delete revision mismatch: %d != %d", d1.Revision, d2.Revision)
	}
}

func TestStoreDeleteConcurrentEditIsStale(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("hello"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(ctx, "index.md", []byte("bob's edit"), &rec.Revision, "bob"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// alice tries to delete based on the now-stale revision.
	if _, err := s.Delete(ctx, "index.md", rec.Revision, "alice"); !errors.Is(err, ErrStaleBase) {
		t.Errorf("Delete with stale base: err = %v, want ErrStaleBase", err)
	}
}

func TestStoreManifestIncludesTombstones(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "a.md", []byte("a"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Delete(ctx, "a.md", rec.Revision, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Put(ctx, "b.md", []byte("b"), nil, "bob"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	manifest, err := s.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Fatalf("len(manifest) = %d, want 2", len(manifest))
	}
	var sawTombstone bool
	for _, m := range manifest {
		if m.Path == "a.md" {
			sawTombstone = true
			if !m.Deleted {
				t.Error("a.md should be a tombstone in manifest")
			}
		}
	}
	if !sawTombstone {
		t.Error("manifest missing tombstoned path a.md")
	}
}

// TestDBAloneReconstructsWorkspace validates the issue's core acceptance
// criterion: closing and reopening the DB file, with no other state,
// fully reconstructs the canonical workspace byte-for-byte.
func TestDBAloneReconstructsWorkspace(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "report-rex.db")

	db1, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	store1 := NewStore(db1)
	ctx := context.Background()

	files := map[string][]byte{
		"index.md":             []byte("# Report"),
		"findings/SQLI-001.md": []byte("SQL injection details"),
		"assets/proof.png":     {0x89, 'P', 'N', 'G', 1, 2, 3, 4},
	}
	for path, content := range files {
		if _, err := store1.Put(ctx, path, content, nil, "alice"); err != nil {
			t.Fatalf("Put(%s): %v", path, err)
		}
	}
	db1.Close()

	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("reopen OpenDB: %v", err)
	}
	defer db2.Close()
	store2 := NewStore(db2)

	manifest, err := store2.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != len(files) {
		t.Fatalf("len(manifest) = %d, want %d", len(manifest), len(files))
	}
	for path, want := range files {
		rec, err := store2.Get(ctx, path)
		if err != nil {
			t.Fatalf("Get(%s): %v", path, err)
		}
		if string(rec.Content) != string(want) {
			t.Errorf("Get(%s).Content = %q, want %q", path, rec.Content, want)
		}
	}
}

// A client that pulled a delete (or never saw the path) re-creates it with
// no base revision; that's a re-create over a tombstone, not a conflict
// (issue #42: re-creating a deleted index.md conflicted).
func TestStorePutWithoutBaseRecreatesTombstonedPath(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	rec, err := s.Put(ctx, "index.md", []byte("v1"), nil, "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	del, err := s.Delete(ctx, "index.md", rec.Revision, "alice")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	re, err := s.Put(ctx, "index.md", []byte("v2"), nil, "bob")
	if err != nil {
		t.Fatalf("re-create over tombstone: %v", err)
	}
	if re.Revision != del.Revision+1 || re.Deleted {
		t.Errorf("re-created record = rev %d deleted=%v, want rev %d live", re.Revision, re.Deleted, del.Revision+1)
	}
	// A stale explicit base over the tombstone is still a conflict.
	if _, err := s.Delete(ctx, "index.md", re.Revision, "bob"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Put(ctx, "index.md", []byte("v3"), &rec.Revision, "carol"); !errors.Is(err, ErrStaleBase) {
		t.Errorf("stale base over tombstone: err = %v, want ErrStaleBase", err)
	}
}

func TestCreateConflictDedupesPerPathAndUser(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()
	mk := func(user, content string) (int64, bool) {
		id, created, err := s.CreateConflict(ctx, Conflict{
			Path: "hot.md", Kind: ConflictKindDivergentEdit, LocalContent: []byte(content),
			LocalHash: content, LocalUsername: user, RemoteRevision: 2, RemoteHash: "r",
		})
		if err != nil {
			t.Fatalf("CreateConflict: %v", err)
		}
		return id, created
	}

	id1, created := mk("alice", "a1")
	if !created {
		t.Fatal("first conflict should be created")
	}
	id2, created := mk("alice", "a2")
	if created || id2 != id1 {
		t.Errorf("alice's second conflict: id=%d created=%v, want id=%d updated in place", id2, created, id1)
	}
	if id3, created := mk("bob", "b1"); !created || id3 == id1 {
		t.Errorf("bob's conflict: id=%d created=%v, want a separate new conflict", id3, created)
	}
	c, err := s.GetConflict(ctx, id1)
	if err != nil || string(c.LocalContent) != "a2" {
		t.Errorf("alice's conflict content = %q (%v), want latest a2", c.LocalContent, err)
	}
	open, _ := s.ListOpenConflicts(ctx)
	if len(open) != 2 {
		t.Errorf("open conflicts = %d, want 2", len(open))
	}
	// Once resolved, a new divergence is a new conflict again.
	if err := s.MarkResolved(ctx, id1, "use_remote", "alice"); err != nil {
		t.Fatal(err)
	}
	if id4, created := mk("alice", "a3"); !created || id4 == id1 {
		t.Errorf("after resolve: id=%d created=%v, want a fresh conflict", id4, created)
	}
}
