package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWatchPicksUpSubdirectoryCreatedAfterStartup covers a gap noted in the
// original implementation: fsnotify only watches directories explicitly
// registered with it, so a subdirectory created after Watch starts was
// previously invisible until the whole watcher restarted. Watch now adds
// newly created directories to the watcher as they appear.
func TestWatchPicksUpSubdirectoryCreatedAfterStartup(t *testing.T) {
	dir := t.TempDir()

	builds := make(chan struct{}, 10)
	build := func() error {
		builds <- struct{}{}
		return nil
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- Watch(dir, build, stop) }()

	// Drain the initial build triggered by Watch's startup.
	select {
	case <-builds:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial build")
	}

	newDir := filepath.Join(dir, "newsection")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Give fsnotify a moment to register the new directory before writing
	// into it -- this is exactly the race the fix closes.
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(newDir, "new.md"), []byte("# New"), 0o644); err != nil {
		t.Fatalf("write file in new subdir: %v", err)
	}

	select {
	case <-builds:
	case <-time.After(3 * time.Second):
		t.Fatal("expected a rebuild triggered by a file created in a post-startup subdirectory")
	}

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Watch returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not stop after stop channel closed")
	}
}
