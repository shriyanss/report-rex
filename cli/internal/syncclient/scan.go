package syncclient

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/shriyanss/report-rex/cli/internal/synccommon"
)

// LocalFile describes one file found during a workspace scan.
type LocalFile struct {
	Path string // normalized, workspace-relative
	Hash string
	Size int64
}

// ScanWorkspace walks dir and returns every non-excluded regular file,
// normalized-path -> LocalFile. Unreadable files are logged and skipped
// (never fatal) so permission errors on one path don't block the rest of
// the sync.
//
// A missing workspace root is an error, not an empty scan: WalkDir would
// otherwise hand the root's own lstat failure to the callback below, which
// skips it, and a moved-away workspace would look like every file was
// deleted (issue #46).
func ScanWorkspace(dir string) (map[string]LocalFile, error) {
	if info, err := os.Stat(dir); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	out := map[string]LocalFile{}

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("sync: skipping %s: %v", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)

		if d.IsDir() {
			if synccommon.IsExcluded(slashRel) {
				return filepath.SkipDir
			}
			return nil
		}
		if synccommon.IsExcluded(slashRel) {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Symlinks are not synchronized: their target may not exist on
			// another client, and "relative path + bytes" doesn't model
			// them cleanly.
			return nil
		}

		content, readErr := os.ReadFile(p)
		if readErr != nil {
			log.Printf("sync: skipping unreadable file %s: %v", p, readErr)
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		out[slashRel] = LocalFile{
			Path: slashRel,
			Hash: synccommon.HashBytes(content),
			Size: info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	return out, nil
}

// ReadFile reads and returns the bytes of a workspace-relative path.
func ReadFile(workspaceDir, relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(workspaceDir, filepath.FromSlash(relPath)))
}

// WriteFile writes content to a workspace-relative path, creating parent
// directories as needed.
func WriteFile(workspaceDir, relPath string, content []byte) error {
	full := filepath.Join(workspaceDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o644)
}

// RemoveFile deletes a workspace-relative path if present, then prunes any
// parent directories left empty (up to, never including, workspaceDir), so
// a folder renamed on another client doesn't linger here empty (#44).
func RemoveFile(workspaceDir, relPath string) error {
	full := filepath.Join(workspaceDir, filepath.FromSlash(relPath))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	// os.Remove refuses non-empty directories, which is what stops the walk.
	root := filepath.Clean(workspaceDir)
	for d := filepath.Dir(full); d != root && os.Remove(d) == nil; d = filepath.Dir(d) {
	}
	return nil
}
