package synccommon

import (
	"path"
	"strings"
)

// excludedDirs are directory names that, anywhere in a path, remove that
// path (and everything under it) from sync scope.
var excludedDirs = map[string]bool{
	".git":        true,
	".report-rex": true,
	// Editor workspace settings are per-user (the VS Code extension keeps
	// host/username/sync-enabled there); syncing them would hand every
	// teammate your identity and make the file itself a conflict magnet.
	".vscode": true,
	// rex-svr's default --workspace. Running `rex sync` in the directory
	// rex-svr was started from would otherwise upload the server's own git
	// working copy back into itself, one nesting level per pass.
	"rex-svr-git-workspace": true,
}

// excludedNamePatterns are shell-glob patterns matched against a path's
// final segment.
var excludedNamePatterns = []string{
	".DS_Store",
	"*.swp",
	"*.swo",
	"*.tmp",
	// rex-svr's default --db (plus its -wal/-shm/-journal siblings),
	// --cert and --key. Syncing the directory rex-svr runs in must never
	// hand every teammate the server's private key and session tokens, nor
	// re-push its database every time a write changes it.
	"rex-svr.db*",
	"rex-svr.crt",
	"rex-svr.key",
}

// IsExcluded reports whether a normalized, workspace-relative path should
// never be synchronized. Used identically by client scan, client watcher,
// and server-side upload validation.
func IsExcluded(normalizedPath string) bool {
	segs := strings.Split(normalizedPath, "/")
	for _, seg := range segs {
		if excludedDirs[seg] {
			return true
		}
	}

	base := segs[len(segs)-1]
	for _, pat := range excludedNamePatterns {
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}

	return false
}
