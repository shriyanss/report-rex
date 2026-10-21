package syncserver

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cfg := Config{Addr: ":0", PasswordHash: hash}
	return NewServer(cfg, db, nil, nil)
}
