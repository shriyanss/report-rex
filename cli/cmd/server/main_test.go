package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSecret(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(f, []byte("  s3cret\n"), 0o600)

	if got, err := readSecret("admin-password", "", f); err != nil || got != "s3cret" {
		t.Errorf("file: got %q, %v", got, err)
	}
	if got, err := readSecret("admin-password", "flag", ""); err != nil || got != "flag" {
		t.Errorf("flag: got %q, %v", got, err)
	}
	if got, err := readSecret("admin-password", "", ""); err != nil || got != "" {
		t.Errorf("unset: got %q, %v", got, err)
	}
	if _, err := readSecret("admin-password", "flag", f); err == nil {
		t.Error("both set: want mutually-exclusive error")
	}
	if _, err := readSecret("admin-password", "", f+"-missing"); err == nil {
		t.Error("missing file: want error")
	}
}
