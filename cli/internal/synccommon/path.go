package synccommon

import (
	"fmt"
	"path"
	"strings"
)

// NormalizePath converts a workspace-relative path to the canonical form
// used as file identity throughout the sync protocol: forward-slash
// separated, cleaned, with no leading "/" and no ".." traversal.
//
// It rejects absolute paths and any path that escapes the workspace root.
func NormalizePath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path must not be empty")
	}

	// Normalize Windows-style separators before cleaning so traversal
	// segments expressed as "..\\.." are caught too.
	slashed := strings.ReplaceAll(p, "\\", "/")

	if strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("path must be workspace-relative, got absolute path %q", p)
	}

	cleaned := path.Clean(slashed)

	if cleaned == "." {
		return "", fmt.Errorf("path must not be empty")
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the workspace root", p)
	}

	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", fmt.Errorf("path %q contains a traversal segment", p)
		}
	}

	return cleaned, nil
}
