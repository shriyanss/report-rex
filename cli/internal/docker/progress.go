package docker

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// withProgress runs fn while printing an animated elapsed-time indicator to
// stderr, so a long silent pandoc/XeLaTeX render doesn't look hung. Skipped
// when stderr isn't a terminal (redirected output, CI) or quiet is true.
func withProgress(label string, quiet bool, fn func() error) error {
	if quiet || !term.IsTerminal(int(os.Stderr.Fd())) {
		if !quiet {
			fmt.Fprintf(os.Stderr, "%s...\n", label)
		}
		return fn()
	}

	// ponytail: the child's own stdout/stderr write straight to the same fd
	// as this spinner (not proxied through it), so a rare interleave is
	// possible if it prints while running — in practice pandoc/xelatex are
	// silent until they fail, right as the spinner below stops and clears.
	// Upgrade path if that ever bites: pipe the child's output through a
	// reader and serialize writes with the spinner instead of writing directly.
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		frames := []rune{'|', '/', '-', '\\'}
		start := time.Now()
		t := time.NewTicker(150 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			select {
			case <-stop:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-t.C:
				fmt.Fprintf(os.Stderr, "\r%c %s (%ds)", frames[i%len(frames)], label, int(time.Since(start).Seconds()))
			}
		}
	}()
	err := fn()
	close(stop)
	<-stopped
	return err
}
