package syncclient

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/watcher"
)

const (
	watchDebounce = 400 * time.Millisecond
	// watchMaxWait caps how long a continuous stream of events (someone
	// typing with autosave, a script writing every 100ms) can hold the
	// debounce open. Without it the trigger never fires while writes keep
	// coming, and sync silently falls back to the 30s safety-net ticker.
	watchMaxWait = 2 * time.Second
)

// WatchWorkspace watches dir (recursively, picking up subdirectories
// created after startup) for changes to any non-excluded file -- unlike
// rex watch's narrower .md/.yaml/.yml filter, sync must react to
// every file type, since screenshots and other assets are first-class
// synchronized content.
//
// Rather than tracking which specific path changed, WatchWorkspace simply
// debounces/coalesces bursts of events (including an editor's atomic
// temp-file-then-rename save pattern, which arrives as multiple raw
// events for one logical save) and signals trigger once per quiet period.
// The caller re-runs a full Engine.Reconcile pass, which is idempotent and
// content-hash driven, so a spurious or duplicate trigger costs a cheap
// no-op diff rather than a wrong sync decision.
func WatchWorkspace(dir string, trigger chan<- struct{}, stop <-chan struct{}) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer w.Close()

	if err := watcher.AddRecursive(w, dir); err != nil {
		return fmt.Errorf("watching %s: %w", dir, err)
	}

	// quiet fires watchDebounce after the latest event; maxWait fires
	// watchMaxWait after the first event of a burst. Both are nil (block
	// forever) while nothing is pending.
	var quiet, maxWait <-chan time.Time
	fire := func() {
		quiet, maxWait = nil, nil
		select {
		case trigger <- struct{}{}:
		default:
		}
	}

	for {
		select {
		case <-stop:
			return nil
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = watcher.AddRecursive(w, event.Name)
				}
			}
			if !relevantForSync(dir, event) {
				continue
			}
			quiet = time.After(watchDebounce)
			if maxWait == nil {
				maxWait = time.After(watchMaxWait)
			}
		case <-quiet:
			fire()
		case <-maxWait:
			fire()
		case _, ok := <-w.Errors:
			if !ok {
				return nil
			}
			// Non-fatal: keep watching. The periodic reconcile ticker
			// (cmd/sync.go) still catches anything a dropped fsnotify
			// error might have missed.
		}
	}
}

func relevantForSync(root string, event fsnotify.Event) bool {
	if !(event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
		return false
	}
	rel, err := filepath.Rel(root, event.Name)
	if err != nil {
		return true
	}
	return !synccommon.IsExcluded(filepath.ToSlash(rel))
}
