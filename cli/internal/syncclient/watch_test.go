package syncclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchWorkspaceTriggersOnAssetChange(t *testing.T) {
	dir := t.TempDir()
	trigger := make(chan struct{}, 10)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- WatchWorkspace(dir, trigger, stop) }()

	// Give the watcher time to register the root directory.
	time.Sleep(150 * time.Millisecond)

	assetDir := filepath.Join(dir, "assets", "screenshots")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(assetDir, "proof.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-trigger:
	case <-time.After(3 * time.Second):
		t.Fatal("expected a trigger for a binary asset change in a post-startup subdirectory")
	}

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("WatchWorkspace returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WatchWorkspace did not stop")
	}
}

func TestWatchWorkspaceIgnoresExcludedPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".report-rex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	trigger := make(chan struct{}, 10)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- WatchWorkspace(dir, trigger, stop) }()
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, ".report-rex", "sync.db"), []byte("junk"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-trigger:
		t.Fatal("did not expect a trigger for a write inside .report-rex")
	case <-time.After(800 * time.Millisecond):
		// expected: no trigger
	}

	close(stop)
	<-done
}

// TestWatchWorkspaceFiresDuringContinuousWrites covers issue #42's load
// pattern: a write every 100ms resets the 400ms quiet debounce forever, so
// without the max-wait cap the trigger never fires while writes continue.
func TestWatchWorkspaceFiresDuringContinuousWrites(t *testing.T) {
	dir := t.TempDir()
	trigger := make(chan struct{}, 10)
	stop := make(chan struct{})
	defer close(stop)
	go func() { _ = WatchWorkspace(dir, trigger, stop) }()
	time.Sleep(150 * time.Millisecond)

	writerStop := make(chan struct{})
	defer close(writerStop)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-writerStop:
				return
			case <-time.After(100 * time.Millisecond):
				_ = os.WriteFile(filepath.Join(dir, "busy.md"), []byte{byte(i)}, 0o644)
			}
		}
	}()

	select {
	case <-trigger:
	case <-time.After(watchMaxWait + 2*time.Second):
		t.Fatal("no trigger while writes kept arriving every 100ms")
	}
}
