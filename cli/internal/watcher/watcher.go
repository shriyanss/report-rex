// Package watcher implements a tsc-watch-style rebuild loop for report-rex report
// directories.
package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounce = 400 * time.Millisecond

// Watch monitors dir (recursively) for changes to .md/.yaml/.yml files and calls
// build after each debounced change, printing a pass/fail status. It blocks until
// the process receives an interrupt (stop channel closes) or fsnotify errors out.
func Watch(dir string, build func() error, stop <-chan struct{}) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer w.Close()

	if err := AddRecursive(w, dir); err != nil {
		return fmt.Errorf("watching %s: %w", dir, err)
	}

	fmt.Printf("Watching %s for changes...\n", dir)
	runBuild(build)

	var debounceTimer *time.Timer
	pending := false

	for {
		select {
		case <-stop:
			return nil
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			// A directory created after startup isn't watched by fsnotify
			// automatically (it only watches directories explicitly added
			// to it); pick up new subdirectories as they appear so files
			// later created inside them are seen too.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = AddRecursive(w, event.Name)
				}
			}
			if !relevant(event) {
				continue
			}
			pending = true
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounce, func() {
				if pending {
					pending = false
					runBuild(build)
				}
			})
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "watch error: %v\n", err)
		}
	}
}

func runBuild(build func() error) {
	fmt.Println("Rebuilding...")
	if err := build(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		return
	}
	fmt.Println("Build succeeded. Watching for changes...")
}

func relevant(event fsnotify.Event) bool {
	if !(event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(event.Name))
	return ext == ".md" || ext == ".yaml" || ext == ".yml"
}

// AddRecursive registers root and every subdirectory under it (except
// .git) with w. It's exported so other watch-style consumers (e.g. the
// sync client's continuous-synchronization watcher) can reuse the same
// directory-walk logic instead of reimplementing it.
func AddRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".report-rex" {
				return filepath.SkipDir
			}
			return w.Add(path)
		}
		return nil
	})
}
