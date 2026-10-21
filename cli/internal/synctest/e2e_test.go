//go:build e2e

// Package synctest holds real subprocess end-to-end tests for the sync
// feature: it builds and runs actual rex-svr and rex
// binaries (not the in-process test servers used by internal/syncserver
// and internal/syncclient's own tests) to exercise the full stack,
// including real git materialization and real process startup/CLI
// parsing. Run via:
//
//	go test ./internal/synctest/... -tags e2e -v
//
// Requires a real `git` executable on PATH (installed by the e2e-sync CI
// job) and builds fresh rex/rex-svr binaries into t.TempDir().
package synctest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
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
)

// insecureHTTPClient trusts rex-svr's auto-generated self-signed certificate
// without pinning it, matching the --insecure flag every client subprocess
// below is spawned with.
var insecureHTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

var (
	repoRoot     string
	serverBinary string
	clientBinary string
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// internal/synctest -> cli
	repoRoot = filepath.Join(wd, "..", "..")

	tmp, err := os.MkdirTemp("", "synctest-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	serverBinary = filepath.Join(tmp, "rex-svr")
	clientBinary = filepath.Join(tmp, "rex")

	if err := goBuild(repoRoot, serverBinary, "./cmd/server"); err != nil {
		fmt.Fprintln(os.Stderr, "build server:", err)
		os.Exit(1)
	}
	if err := goBuild(repoRoot, clientBinary, "."); err != nil {
		fmt.Fprintln(os.Stderr, "build client:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func goBuild(dir, out, pkg string) error {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

type testServer struct {
	t       *testing.T
	cmd     *exec.Cmd
	port    int
	db      string
	ws      string
	baseURL string
}

func startServer(t *testing.T, port int, db, workspace string) *testServer {
	t.Helper()
	// Certs are scoped alongside db/workspace (both already under a
	// t.TempDir()) rather than the default ./rex-svr.crt/.key, so a test run
	// never leaves generated certificate files behind in the source tree.
	certDir := filepath.Dir(db)
	s := startServerIn(t, "", port, "--db", db, "--workspace", workspace,
		"--cert", filepath.Join(certDir, "rex-svr.crt"), "--key", filepath.Join(certDir, "rex-svr.key"))
	s.db, s.ws = db, workspace
	return s
}

// startServerIn starts rex-svr from working directory cwd ("" for the test's
// own), so relative defaults like ./rex-svr.db land there -- the way an
// operator actually runs it.
func startServerIn(t *testing.T, cwd string, port int, args ...string) *testServer {
	t.Helper()
	cmd := exec.Command(serverBinary, append([]string{"--port", strconv.Itoa(port), "--password", "hunter2"}, args...)...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	s := &testServer{t: t, cmd: cmd, port: port, baseURL: fmt.Sprintf("https://localhost:%d", port)}
	t.Cleanup(func() {
		if s.cmd.Process != nil {
			s.cmd.Process.Kill()
			s.cmd.Wait()
		}
		t.Logf("server output:\n%s", out.String())
	})
	s.waitHealthy()
	return s
}

func (s *testServer) stop() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
}

func (s *testServer) waitHealthy() {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := insecureHTTPClient.Get(s.baseURL + "/api/v1/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("server on port %d never became healthy", s.port)
}

// runClient runs a rex subcommand as a subprocess and returns its
// combined output. `sync` invocations should pass a timeout via the ctx.
func runClient(t *testing.T, ctx context.Context, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, clientBinary, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func syncOnce(t *testing.T, port int, username, dir string, timeout time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runClient(t, ctx, "sync", "--host", "localhost", "--port", strconv.Itoa(port),
		"--username", username, "--password", "hunter2", "--dir", dir, "--insecure")
}

func TestE2EInitialPushAndPull(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rex.db")
	wsPath := filepath.Join(dir, "ws")
	srv := startServer(t, 19601, dbPath, wsPath)

	clientA := filepath.Join(dir, "a")
	os.MkdirAll(filepath.Join(clientA, "reports"), 0o755)
	os.WriteFile(filepath.Join(clientA, "index.md"), []byte("# Report"), 0o644)
	os.WriteFile(filepath.Join(clientA, "reports", "vuln-1.md"), []byte("finding"), 0o644)

	outA := syncOnce(t, srv.port, "alice", clientA, 5*time.Second)
	if !strings.Contains(outA, "pushed") {
		t.Fatalf("expected pushed events, got:\n%s", outA)
	}

	clientB := filepath.Join(dir, "b")
	os.MkdirAll(clientB, 0o755)
	outB := syncOnce(t, srv.port, "bob", clientB, 5*time.Second)
	if !strings.Contains(outB, "pulled") {
		t.Fatalf("expected pulled events, got:\n%s", outB)
	}
	if _, err := os.Stat(filepath.Join(clientB, "reports", "vuln-1.md")); err != nil {
		t.Errorf("bob should have pulled reports/vuln-1.md: %v", err)
	}

	// Verify real git history exists in the server's workspace.
	gitLog := exec.Command("git", "-C", wsPath, "log", "--oneline")
	out, err := gitLog.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected git history in server workspace")
	}
}

func TestE2EConcurrentEditConflictAndResolve(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19602, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))

	clientA := filepath.Join(dir, "a")
	os.MkdirAll(clientA, 0o755)
	os.WriteFile(filepath.Join(clientA, "index.md"), []byte("base"), 0o644)
	syncOnce(t, srv.port, "alice", clientA, 5*time.Second)

	clientB := filepath.Join(dir, "b")
	os.MkdirAll(clientB, 0o755)
	syncOnce(t, srv.port, "bob", clientB, 5*time.Second) // bob pulls "base"

	// Both edit divergently, offline from each other.
	os.WriteFile(filepath.Join(clientA, "index.md"), []byte("alice's edit"), 0o644)
	syncOnce(t, srv.port, "alice", clientA, 5*time.Second)

	os.WriteFile(filepath.Join(clientB, "index.md"), []byte("bob's edit"), 0o644)
	outB := syncOnce(t, srv.port, "bob", clientB, 5*time.Second)
	if !strings.Contains(outB, "conflict") {
		t.Fatalf("expected a conflict event for bob's divergent push, got:\n%s", outB)
	}

	conflictsOut := runClient(t, context.Background(), "conflicts", "--host", "localhost",
		"--port", strconv.Itoa(srv.port), "--username", "bob", "--password", "hunter2", "--dir", clientB, "--insecure")
	if !strings.Contains(conflictsOut, "index.md") {
		t.Fatalf("expected index.md listed as a conflict, got:\n%s", conflictsOut)
	}

	resolveOut := runClient(t, context.Background(), "resolve", "index.md", "--host", "localhost",
		"--port", strconv.Itoa(srv.port), "--username", "bob", "--password", "hunter2", "--dir", clientB, "--local", "--insecure")
	if !strings.Contains(resolveOut, "Resolved") {
		t.Fatalf("expected resolution confirmation, got:\n%s", resolveOut)
	}

	// Alice should pull bob's now-canonical resolution on her next sync.
	syncOnce(t, srv.port, "alice", clientA, 5*time.Second)
	got, err := os.ReadFile(filepath.Join(clientA, "index.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "bob's edit" {
		t.Errorf("alice's local content = %q, want bob's resolved edit", got)
	}
}

// presenceBridge drives a real `rex presence` subprocess: it's the only
// thing in this repo that ever sends stdin lines to it and reads its
// stdout events, so this is the one place actually proving the CLI wiring
// (as opposed to the in-process syncclient.DialPresence tests) works.
type presenceBridge struct {
	t     *testing.T
	stdin *os.File
	lines chan map[string]any
}

func startPresenceBridge(t *testing.T, port int, username string) *presenceBridge {
	t.Helper()
	cmd := exec.Command(clientBinary, "presence", "--host", "localhost", "--port", strconv.Itoa(port), "--username", username, "--insecure")
	cmd.Env = append(os.Environ(), "REX_PASSWORD=hunter2")

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	cmd.Stdin = stdinR
	cmd.Stdout = stdoutW
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start rex presence: %v", err)
	}
	stdinR.Close()
	stdoutW.Close()

	lines := make(chan map[string]any, 32)
	go func() {
		scanner := bufio.NewScanner(stdoutR)
		for scanner.Scan() {
			var m map[string]any
			if json.Unmarshal(scanner.Bytes(), &m) == nil {
				lines <- m
			}
		}
	}()

	t.Cleanup(func() {
		stdinW.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
		stdoutR.Close()
		t.Logf("rex presence (%s) stderr:\n%s", username, stderr.String())
	})

	return &presenceBridge{t: t, stdin: stdinW, lines: lines}
}

func (b *presenceBridge) send(v map[string]any) {
	data, _ := json.Marshal(v)
	b.stdin.Write(append(data, '\n'))
}

func (b *presenceBridge) waitFor(timeout time.Duration, pred func(map[string]any) bool) map[string]any {
	b.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case m := <-b.lines:
			if pred(m) {
				return m
			}
		case <-deadline:
			b.t.Fatalf("timed out waiting for expected rex presence event")
			return nil
		}
	}
}

func TestE2EPresenceUpdateBroadcastAndRelease(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19606, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))

	alice := startPresenceBridge(t, srv.port, "alice")
	bob := startPresenceBridge(t, srv.port, "bob")

	isConnectedStatus := func(m map[string]any) bool { return m["type"] == "status" && m["connected"] == true }
	alice.waitFor(10*time.Second, isConnectedStatus)
	bob.waitFor(10*time.Second, isConnectedStatus)

	bob.send(map[string]any{"op": "update", "file": "findings/XSS-001.md", "state": "editing", "dirty": true})

	ev := alice.waitFor(5*time.Second, func(m map[string]any) bool {
		return m["type"] == "presence" && m["username"] == "bob"
	})
	if ev["state"] != "editing" || ev["dirty"] != true || ev["action"] != "active" {
		t.Fatalf("unexpected presence event: %+v", ev)
	}

	bob.send(map[string]any{"op": "release", "file": "findings/XSS-001.md"})

	idle := alice.waitFor(5*time.Second, func(m map[string]any) bool {
		return m["type"] == "presence" && m["username"] == "bob" && m["action"] == "idle"
	})
	if idle["file"] != "findings/XSS-001.md" {
		t.Fatalf("unexpected idle event: %+v", idle)
	}
}

func TestE2EServerRestartDriftRepair(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rex.db")
	wsPath := filepath.Join(dir, "ws")
	srv := startServer(t, 19603, dbPath, wsPath)

	clientA := filepath.Join(dir, "a")
	os.MkdirAll(clientA, 0o755)
	os.WriteFile(filepath.Join(clientA, "index.md"), []byte("v1"), 0o644)
	syncOnce(t, srv.port, "alice", clientA, 5*time.Second)

	// Tamper with the server-managed git workspace directly, simulating an
	// unexpected manual edit while the server is down.
	srv.stop()
	os.WriteFile(filepath.Join(wsPath, "index.md"), []byte("tampered while server was down"), 0o644)

	srv2 := startServer(t, 19604, dbPath, wsPath) // same db+workspace, restart
	defer srv2.stop()

	got, err := os.ReadFile(filepath.Join(wsPath, "index.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "v1" {
		t.Errorf("workspace content after restart = %q, want %q (drift repair must restore DB's canonical content)", got, "v1")
	}

	// The server must still accept new syncs after the restart.
	clientB := filepath.Join(dir, "b")
	os.MkdirAll(clientB, 0o755)
	outB := syncOnce(t, srv2.port, "bob", clientB, 5*time.Second)
	if !strings.Contains(outB, "pulled") {
		t.Fatalf("expected bob to pull after restart, got:\n%s", outB)
	}
}
