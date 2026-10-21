package docker

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shriyanss/report-rex/cli/internal/embedded"
)

// ImageExists reports whether a local image with the given tag is present.
func ImageExists(tag string) bool {
	cmd := exec.Command("docker", "image", "inspect", tag)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// BuildImage materializes the embedded Docker build context (Dockerfile, core/,
// templates/, styles/, .vale.ini) to a temp directory and runs `docker build`.
func BuildImage(tag string) error {
	tmpDir, err := os.MkdirTemp("", "report-rex-build-context-")
	if err != nil {
		return fmt.Errorf("creating temp build context: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractFS(embedded.BuildContextFS(), tmpDir); err != nil {
		return fmt.Errorf("materializing embedded build context: %w", err)
	}

	fmt.Printf("Building report-rex image %q from embedded source (this may take a few minutes on first run)...\n", tag)
	return run("build", "-t", tag, tmpDir)
}

// extractFS writes every file in src to dest, preserving the directory tree.
func extractFS(src fs.FS, dest string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
