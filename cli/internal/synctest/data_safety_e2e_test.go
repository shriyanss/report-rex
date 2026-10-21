//go:build e2e

package synctest

// Data-safety end-to-end tests (issue #62 and the bugs found alongside it):
// every way a rex-svr restart or deployment mistake could make the server's
// view of the workspace disagree with reality, and that disagreement turn
// into deleted files. Run on their own by the e2e-data-safety CI job:
//
//	go test ./internal/synctest/... -tags e2e -run TestE2EDataSafety -v

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, for the direct-DB test hook below
)

// expectServerRefuses runs rex-svr from cwd and asserts it exits with an
// error instead of serving; it returns the combined output.
func expectServerRefuses(t *testing.T, cwd string, port int, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, serverBinary, append([]string{"--port", strconv.Itoa(port), "--password", "hunter2"}, args...)...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("rex-svr %v kept running, want it to refuse to start; output:\n%s", args, out)
	}
	if err == nil {
		t.Fatalf("rex-svr %v exited cleanly, want a refusal; output:\n%s", args, out)
	}
	return string(out)
}

// writeReport puts n findings plus an index.md into dir.
func writeReport(t *testing.T, dir string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Report"), 0o644)
	for i := 1; i <= n; i++ {
		os.WriteFile(filepath.Join(dir, "reports", fmt.Sprintf("vuln-%d.md", i)), []byte(fmt.Sprintf("finding %d", i)), 0o644)
	}
}

// reportFiles lists dir's files the way the sync sees them.
func reportFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".report-rex") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// serverManifest returns the live (non-deleted) paths the server knows about.
func serverManifest(t *testing.T, port int) map[string]bool {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, localURL(port, "/api/v1/manifest"), nil)
	req.Header.Set("Authorization", "Bearer "+loginForAPI(t, port, "inspector"))
	resp, err := insecureHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("GET manifest: %v", err)
	}
	defer resp.Body.Close()
	var mr struct {
		Files []struct {
			Path    string `json:"path"`
			Deleted bool   `json:"deleted"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	live := map[string]bool{}
	for _, f := range mr.Files {
		if !f.Deleted {
			live[f.Path] = true
		}
	}
	return live
}

// TestE2EDataSafetyWorkspaceDotRefused is issue #62 itself. With
// `rex-svr --workspace .` and every other flag at its default, ./rex-svr.db
// sat inside the workspace, and the first start's drift check deleted the
// live database (the process kept writing to the unlinked file). The next
// restart came up with an empty database, and every client deleted its
// files. The same layout with --db moved out still put the TLS key there:
// committed to git, then deleted and regenerated on restart, which locked out
// every client pinned with --ca-cert. Both must be refused before anything is
// created.
func TestE2EDataSafetyWorkspaceDotRefused(t *testing.T) {
	for i, extra := range [][]string{nil, {"--db", "../rex.db"}} {
		dir := filepath.Join(t.TempDir(), "testing")
		os.MkdirAll(dir, 0o755)
		out := expectServerRefuses(t, dir, 19640+i, append([]string{"--workspace", "."}, extra...)...)
		if !strings.Contains(out, "inside --workspace") {
			t.Errorf("--workspace . %v: refusal doesn't explain the layout problem:\n%s", extra, out)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("--workspace . %v: refused start still created %d entries in the workspace", extra, len(left))
		}
	}
}

// TestE2EDataSafetyRestartFromAnotherDirectoryKeepsClientFiles: rex-svr's
// defaults are relative to its working directory, so restarting it with the
// same command line from somewhere else brings it up on a brand-new database
// and workspace. Clients treated every path missing from that empty manifest
// as deleted on the server and wiped their workspaces; they must push their
// files back instead.
func TestE2EDataSafetyRestartFromAnotherDirectoryKeepsClientFiles(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	os.MkdirAll(first, 0o755)
	os.MkdirAll(second, 0o755)

	srv := startServerIn(t, first, 19642)
	alice := filepath.Join(root, "alice")
	writeReport(t, alice, 6)
	syncOnce(t, srv.port, "alice", alice, 5*time.Second)
	srv.stop()

	srv2 := startServerIn(t, second, 19642)
	out := syncOnce(t, srv2.port, "alice", alice, 6*time.Second)
	if strings.Contains(out, "deleted") {
		t.Errorf("alice deleted files after the server came back empty:\n%s", out)
	}
	if got := reportFiles(t, alice); len(got) != 7 {
		t.Errorf("alice has %d files after reconnecting, want all 7: %v", len(got), got)
	}
	if got := serverManifest(t, srv2.port); len(got) != 7 {
		t.Errorf("restarted server has %d files, want alice's 7 pushed back: %v", len(got), got)
	}
}

// TestE2EDataSafetyManifestAnomalyRefusesToStart is issue #37's regression
// test, tightened by #62: a server whose database lost track of most of its
// workspace (the wrong or a fresh --db) used to log a warning and then serve
// that empty manifest anyway, and every client deleted its copy. It must not
// start at all, and must leave the workspace untouched.
func TestE2EDataSafetyManifestAnomalyRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rex.db")
	wsPath := filepath.Join(dir, "ws")
	srv := startServer(t, 19643, dbPath, wsPath)

	clientA := filepath.Join(dir, "a")
	writeReport(t, clientA, 8)
	syncOnce(t, srv.port, "alice", clientA, 5*time.Second)
	srv.stop()

	// Every synced file stays materialized on disk, but the files table
	// comes back empty, same as if the server read the wrong database.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM files`); err != nil {
		t.Fatalf("simulate manifest anomaly: %v", err)
	}
	db.Close()

	out := expectServerRefuses(t, "", 19643, "--db", dbPath, "--workspace", wsPath,
		"--cert", filepath.Join(dir, "rex-svr.crt"), "--key", filepath.Join(dir, "rex-svr.key"))
	if !strings.Contains(out, "not in the database") {
		t.Errorf("refusal doesn't explain the database mismatch:\n%s", out)
	}
	if got := reportFiles(t, wsPath); len(got) != 9 {
		t.Errorf("workspace has %d files after the refused start, want all 9 untouched: %v", len(got), got)
	}
}

// TestE2EDataSafetyForeignWorkspaceRefused: pointing --workspace at an
// existing report folder (fewer files than the mass-delete guard's minimum)
// deleted them all on the first start, permanently, since they had never
// been committed; if the folder was the user's own git repo, rex-svr
// committed the deletion into it.
func TestE2EDataSafetyForeignWorkspaceRefused(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "myreport")
	writeReport(t, plain, 3)
	repo := filepath.Join(root, "myrepo")
	writeReport(t, repo, 3)
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	for i, ws := range []string{plain, repo} {
		out := expectServerRefuses(t, root, 19644+i, "--workspace", ws, "--db", filepath.Join(root, fmt.Sprintf("rex%d.db", i)))
		if !strings.Contains(out, "new or empty directory") {
			t.Errorf("%s: refusal doesn't say what to do instead:\n%s", ws, out)
		}
		if got := reportFiles(t, ws); len(got) != 4 {
			t.Errorf("%s has %d files after the refused start, want all 4: %v", ws, len(got), got)
		}
	}
	if _, err := os.Stat(filepath.Join(plain, ".git")); err == nil {
		t.Error("refused start still ran git init in the user's folder")
	}
}

// TestE2EDataSafetyClientInServerDirectorySkipsServerState: running
// `rex sync` in the directory rex-svr was started from uploaded the
// server's private key and live database (with every session token) to the
// whole team, re-pushed the database on every write, and synced the server's
// own git workspace back into itself.
func TestE2EDataSafetyClientInServerDirectorySkipsServerState(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, 2)
	srv := startServerIn(t, dir, 19646)

	syncOnce(t, srv.port, "alice", dir, 6*time.Second)
	for path := range serverManifest(t, srv.port) {
		if strings.HasPrefix(path, "rex-svr") {
			t.Errorf("server state %s was synced to the team", path)
		}
	}
	if got := serverManifest(t, srv.port); len(got) != 3 {
		t.Errorf("server has %v, want just the 3 report files", got)
	}
}

// TestE2EDataSafetyLongRunningClientSurvivesServerReset: rex-svr keeps
// sessions in its database, so a server brought back on a fresh one no
// longer knows any token. A long-running `rex sync` (what the editor
// extensions run) used to fail every request with 401 until restarted by
// hand. It must log in again and carry on -- pushing its files back and any
// new edits along with them.
func TestE2EDataSafetyLongRunningClientSurvivesServerReset(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "one"), 0o755)
	os.MkdirAll(filepath.Join(root, "two"), 0o755)
	srv := startServer(t, 19647, filepath.Join(root, "one", "rex.db"), filepath.Join(root, "one", "ws"))

	alice := filepath.Join(root, "alice")
	writeReport(t, alice, 2)
	cmd := exec.Command(clientBinary, "sync", "--host", "localhost", "--port", "19647",
		"--username", "alice", "--password", "hunter2", "--dir", alice, "--insecure")
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rex sync: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		t.Logf("rex sync output:\n%s", out.String())
	})

	waitUntil(t, 15*time.Second, "initial push", func() bool { return len(serverManifest(t, srv.port)) == 3 })
	srv.stop()

	srv2 := startServer(t, 19647, filepath.Join(root, "two", "rex.db"), filepath.Join(root, "two", "ws"))
	os.WriteFile(filepath.Join(alice, "written-after-reset.md"), []byte("new"), 0o644)

	waitUntil(t, 45*time.Second, "the same rex sync process to repopulate the reset server", func() bool {
		m := serverManifest(t, srv2.port)
		return len(m) == 4 && m["written-after-reset.md"]
	})
}

func waitUntil(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
