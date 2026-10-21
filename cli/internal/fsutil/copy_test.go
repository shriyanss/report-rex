package fsutil

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestCopyFS(t *testing.T) {
	src := fstest.MapFS{
		"a.txt":          {Data: []byte("hello")},
		"sub/b.txt":      {Data: []byte("world")},
		"sub/deep/c.txt": {Data: []byte("!")},
	}

	dest := t.TempDir()
	if err := CopyFS(src, dest); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}

	for path, want := range map[string]string{
		"a.txt":          "hello",
		"sub/b.txt":      "world",
		"sub/deep/c.txt": "!",
	} {
		got, err := os.ReadFile(filepath.Join(dest, path))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}
