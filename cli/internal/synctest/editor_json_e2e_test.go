//go:build e2e

package synctest

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The machine-readable surfaces the VS Code extension drives (issue #42):
// `rex sync --json`, `rex status --json`, `rex conflicts --json/--side`.

func connArgs(port int, user, pass string) []string {
	return []string{"--host", "localhost", "--port", strconv.Itoa(port), "--username", user, "--password", pass, "--insecure"}
}

func TestE2ESyncJSONEventsAndStdinControl(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19620, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))
	ws := filepath.Join(dir, "alice")
	os.MkdirAll(ws, 0o755)
	os.WriteFile(filepath.Join(ws, "index.md"), []byte("# Report\n"), 0o644)

	cmd := exec.Command(clientBinary, append([]string{"sync", "--json", "--dir", ws}, connArgs(srv.port, "alice", "hunter2")...)...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	lines := make(chan map[string]any, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("non-JSON line in --json mode: %q", sc.Text())
				continue
			}
			lines <- m
		}
		close(lines)
	}()
	waitFor := func(pred func(map[string]any) bool, what string) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case m, ok := <-lines:
				if !ok {
					t.Fatalf("stdout closed before %s", what)
				}
				if pred(m) {
					return
				}
			case <-deadline:
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	waitFor(func(m map[string]any) bool { return m["type"] == "ready" && m["user"] == "alice" }, "ready")
	waitFor(func(m map[string]any) bool {
		return m["type"] == "event" && m["kind"] == "pushed" && m["path"] == "index.md"
	}, "pushed index.md")
	waitFor(func(m map[string]any) bool { return m["type"] == "status" && m["state"] == "connected" }, "connected")

	// "Sync now" over stdin picks up a change without waiting on fsnotify.
	os.WriteFile(filepath.Join(ws, "notes.md"), []byte("n\n"), 0o644)
	stdin.Write([]byte(`{"op":"reconcile"}` + "\n"))
	waitFor(func(m map[string]any) bool {
		return m["type"] == "event" && m["kind"] == "pushed" && m["path"] == "notes.md"
	}, "pushed notes.md")

	// Closing stdin (the editor went away) stops the process.
	stdin.Close()
	waitFor(func(m map[string]any) bool { return m["type"] == "status" && m["state"] == "stopped" }, "stopped")
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("rex sync --json kept running after stdin EOF")
	}
}

func runJSON(t *testing.T, out any, args ...string) (exitCode int) {
	t.Helper()
	cmd := exec.Command(clientBinary, args...)
	raw, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%v: bad JSON %q: %v", args, raw, err)
		}
	}
	return exitCode
}

type statusJSON struct {
	Reachable     bool   `json:"reachable"`
	Authenticated bool   `json:"authenticated"`
	Files         int    `json:"files"`
	Hint          string `json:"hint"`
	Error         string `json:"error"`
	Health        struct {
		Status string `json:"status"`
	} `json:"health"`
	Recent []struct {
		Path       string `json:"path"`
		ModifiedBy string `json:"modified_by"`
	} `json:"recent"`
	Conflicts []struct {
		ID            int64  `json:"id"`
		Path          string `json:"path"`
		LocalUsername string `json:"local_username"`
	} `json:"conflicts"`
}

func TestE2EStatusAndConflictsJSON(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19621, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))
	ws := filepath.Join(dir, "alice")
	os.MkdirAll(ws, 0o755)
	os.WriteFile(filepath.Join(ws, "index.md"), []byte("alice\n"), 0o644)
	syncOnce(t, srv.port, "alice", ws, 4*time.Second)

	// bob uploads over index.md with no base: a conflict owned by bob.
	if code := putFileRaw(t, srv.port, loginForAPI(t, srv.port, "bob"), "index.md", "bob\n"); code != 409 {
		t.Fatalf("bob's blind upload: status %d, want 409", code)
	}

	var st statusJSON
	if code := runJSON(t, &st, append([]string{"status", "--json"}, connArgs(srv.port, "alice", "hunter2")...)...); code != 0 {
		t.Fatalf("status exit %d: %+v", code, st)
	}
	if !st.Reachable || !st.Authenticated || st.Health.Status != "ok" || st.Files != 1 {
		t.Errorf("status = %+v", st)
	}
	if len(st.Recent) != 1 || st.Recent[0].Path != "index.md" || st.Recent[0].ModifiedBy != "alice" {
		t.Errorf("recent = %+v", st.Recent)
	}
	if len(st.Conflicts) != 1 || st.Conflicts[0].LocalUsername != "bob" {
		t.Fatalf("conflicts = %+v", st.Conflicts)
	}

	var cl struct {
		Conflicts []struct {
			ID            int64  `json:"id"`
			LocalUsername string `json:"local_username"`
		} `json:"conflicts"`
	}
	runJSON(t, &cl, append([]string{"conflicts", "--json"}, connArgs(srv.port, "alice", "hunter2")...)...)
	if len(cl.Conflicts) != 1 || cl.Conflicts[0].ID != st.Conflicts[0].ID {
		t.Fatalf("conflicts --json = %+v", cl)
	}
	id := strconv.FormatInt(cl.Conflicts[0].ID, 10)
	for side, want := range map[string]string{"local": "bob\n", "remote": "alice\n"} {
		out := runClient(t, context.Background(), append([]string{"conflicts", "--id", id, "--side", side}, connArgs(srv.port, "alice", "hunter2")...)...)
		if out != want {
			t.Errorf("--side %s = %q, want %q", side, out, want)
		}
	}

	// Failures say which layer broke.
	for _, c := range []struct {
		args []string
		hint string
	}{
		{connArgs(srv.port, "alice", "wrong"), "auth"},
		{connArgs(19629, "alice", "hunter2"), "unreachable"},
		{[]string{"--host", "localhost", "--port", strconv.Itoa(srv.port), "--username", "alice", "--password", "hunter2"}, "tls"},
	} {
		var bad statusJSON
		if code := runJSON(t, &bad, append([]string{"status", "--json"}, c.args...)...); code == 0 || bad.Hint != c.hint {
			t.Errorf("%s case: exit=%d hint=%q err=%q", c.hint, code, bad.Hint, bad.Error)
		}
	}
}

// `rex resolve --merge` twice: the first run writes markers, the second
// (after the user edits them out) submits the file as-is. Previously the
// second run re-merged and wrote markers over the user's resolution, and a
// run *with* markers still on disk submitted them to the server.
func TestE2EResolveMergeHandEditFlow(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19622, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))
	alice, bob := filepath.Join(dir, "alice"), filepath.Join(dir, "bob")
	os.MkdirAll(alice, 0o755)
	os.MkdirAll(bob, 0o755)
	os.WriteFile(filepath.Join(alice, "hot.md"), []byte("title\nsame line\n"), 0o644)
	syncOnce(t, srv.port, "alice", alice, 4*time.Second)
	syncOnce(t, srv.port, "bob", bob, 4*time.Second)
	os.WriteFile(filepath.Join(bob, "hot.md"), []byte("title\nbob's line\n"), 0o644)
	syncOnce(t, srv.port, "bob", bob, 4*time.Second)
	os.WriteFile(filepath.Join(alice, "hot.md"), []byte("title\nalice's line\n"), 0o644)
	if out := syncOnce(t, srv.port, "alice", alice, 4*time.Second); !strings.Contains(out, "conflict") {
		t.Fatalf("expected a conflict, got:\n%s", out)
	}

	resolve := func() string {
		return runClient(t, context.Background(), append([]string{"resolve", "hot.md", "--merge", "--dir", alice}, connArgs(srv.port, "alice", "hunter2")...)...)
	}
	if out := resolve(); !strings.Contains(out, "Automatic merge failed") {
		t.Fatalf("first --merge:\n%s", out)
	}
	if b, _ := os.ReadFile(filepath.Join(alice, "hot.md")); !strings.Contains(string(b), "<<<<<<< LOCAL") {
		t.Fatalf("expected markers on disk, got %q", b)
	}
	if out := resolve(); !strings.Contains(out, "still has") {
		t.Fatalf("--merge with markers still on disk should refuse, got:\n%s", out)
	}
	os.WriteFile(filepath.Join(alice, "hot.md"), []byte("title\nalice + bob\n"), 0o644)
	if out := resolve(); !strings.Contains(out, "Resolved hot.md (merge)") {
		t.Fatalf("second --merge:\n%s", out)
	}
	token := loginForAPI(t, srv.port, "alice")
	req, _ := http.NewRequest(http.MethodGet, localURL(srv.port, "/api/v1/files/hot.md"), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := insecureHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "title\nalice + bob\n" {
		t.Errorf("server copy = %q, want the hand-resolved content", body)
	}
}
