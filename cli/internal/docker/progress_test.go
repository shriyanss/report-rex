package docker

import (
	"errors"
	"testing"
)

func TestWithProgressQuietRunsFnAndReturnsError(t *testing.T) {
	wantErr := errors.New("boom")
	called := false
	err := withProgress("Testing", true, func() error {
		called = true
		return wantErr
	})
	if !called {
		t.Fatal("fn was not called")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
}

func TestWithProgressQuietSuccess(t *testing.T) {
	called := false
	if err := withProgress("Testing", true, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
}

// go test's stderr isn't a terminal, so quiet=false exercises the non-TTY
// fallback (a single printed line, no spinner goroutine) rather than the
// interactive spinner path.
func TestWithProgressNonTTYRunsFn(t *testing.T) {
	called := false
	if err := withProgress("Testing", false, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
}
