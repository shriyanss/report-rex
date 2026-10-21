//go:build e2e

package synctest

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// runHandoffs runs a `rex handoffs <subcommand>` subprocess and returns its
// combined output.
func runHandoffs(t *testing.T, port int, username string, args ...string) string {
	t.Helper()
	full := append([]string{"handoffs"}, args...)
	full = append(full, "--host", "localhost", "--port", strconv.Itoa(port), "--username", username, "--insecure")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, clientBinary, full...)
	cmd.Env = append(os.Environ(), "REX_PASSWORD=hunter2")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// runHandoffsExpectErr is runHandoffs, but also returns whether the process
// exited non-zero -- used for the metadata-mismatch case, where a non-zero
// exit is the point of the assertion, not a test failure.
func runHandoffsExpectErr(t *testing.T, port int, username string, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"handoffs"}, args...)
	full = append(full, "--host", "localhost", "--port", strconv.Itoa(port), "--username", username, "--insecure")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, clientBinary, full...)
	cmd.Env = append(os.Environ(), "REX_PASSWORD=hunter2")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeHandoff(t *testing.T, dir, assignee, title, body string) string {
	t.Helper()
	d := filepath.Join(dir, "handoffs", assignee)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, title+".md")
	content := "---\nto: " + assignee + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// extractID pulls a handoff's stable id out of a `rex handoffs list` table
// row, matching by title (the last column).
func extractID(t *testing.T, listing, title string) string {
	t.Helper()
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[4] == title {
			return fields[0]
		}
	}
	t.Fatalf("no handoff titled %q in listing:\n%s", title, listing)
	return ""
}

// TestE2EHandoffFullLifecycle drives the whole issue #16 workflow through
// real subprocesses and a real rex-svr: create (stamped from a `to:`-only
// file), discover, sync down, edit, and resolve via a local `rm` -- verifying
// the resolution shows up in git history and is never resurrected.
func TestE2EHandoffFullLifecycle(t *testing.T) {
	dir := t.TempDir()
	wsPath := filepath.Join(dir, "ws")
	srv := startServer(t, 19610, filepath.Join(dir, "rex.db"), wsPath)

	aliceDir := filepath.Join(dir, "alice")
	os.MkdirAll(aliceDir, 0o755)
	writeHandoff(t, aliceDir, "shriyanss", "investigate-nextjs-auth",
		"# Investigate Next.js auth\n\nThe app appears to use Next.js.")

	outCreate := syncOnce(t, srv.port, "alice", aliceDir, 5*time.Second)
	if !strings.Contains(outCreate, "pushed") && !strings.Contains(outCreate, "synchronized") {
		t.Fatalf("expected the handoff to be pushed, got:\n%s", outCreate)
	}

	stamped, err := os.ReadFile(filepath.Join(aliceDir, "handoffs", "shriyanss", "investigate-nextjs-auth.md"))
	if err != nil {
		t.Fatalf("read stamped handoff: %v", err)
	}
	for _, want := range []string{"rex_handoff_id:", "from: alice", "status: open"} {
		if !strings.Contains(string(stamped), want) {
			t.Errorf("stamped handoff missing %q:\n%s", want, stamped)
		}
	}

	// shriyanss discovers it without syncing anything locally.
	listOut := runHandoffs(t, srv.port, "shriyanss", "list")
	if !strings.Contains(listOut, "investigate-nextjs-auth") || !strings.Contains(listOut, "alice") {
		t.Fatalf("shriyanss's queue should show alice's handoff:\n%s", listOut)
	}
	id := extractID(t, listOut, "investigate-nextjs-auth")

	viewOut := runHandoffs(t, srv.port, "shriyanss", "view", id)
	if !strings.Contains(viewOut, "From:    alice") || !strings.Contains(viewOut, "Next.js") {
		t.Fatalf("view output missing expected content:\n%s", viewOut)
	}

	// He syncs it down through plain `rex sync` -- handoffs given to him are
	// in scope by default, no `rex handoffs` invocation required.
	shriDir := filepath.Join(dir, "shriyanss")
	os.MkdirAll(shriDir, 0o755)
	outPull := syncOnce(t, srv.port, "shriyanss", shriDir, 5*time.Second)
	if !strings.Contains(outPull, "pulled") {
		t.Fatalf("expected shriyanss to pull the handoff via rex sync, got:\n%s", outPull)
	}
	localPath := filepath.Join(shriDir, "handoffs", "shriyanss", "investigate-nextjs-auth.md")
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("handoff not materialized: %v", err)
	}

	// He edits it and syncs the change back.
	edited, _ := os.ReadFile(localPath)
	edited = append(edited, []byte("\n## Findings\n\nFound /api/admin/... unauthenticated.\n")...)
	os.WriteFile(localPath, edited, 0o644)
	syncOnce(t, srv.port, "shriyanss", shriDir, 5*time.Second)

	viewAfterEdit := runHandoffs(t, srv.port, "alice", "view", id)
	if !strings.Contains(viewAfterEdit, "/api/admin/") {
		t.Fatalf("alice should see shriyanss's edit:\n%s", viewAfterEdit)
	}

	// Resolving via `rm` + `rex sync` records the resolution rather than
	// pushing a plain deletion.
	os.Remove(localPath)
	syncOnce(t, srv.port, "shriyanss", shriDir, 5*time.Second)

	openListing := runHandoffs(t, srv.port, "shriyanss", "list")
	if strings.Contains(openListing, "investigate-nextjs-auth") {
		t.Fatalf("resolved handoff still shows as open:\n%s", openListing)
	}
	resolvedListing := runHandoffs(t, srv.port, "shriyanss", "list", "--resolved")
	if !strings.Contains(resolvedListing, "resolved") || !strings.Contains(resolvedListing, "investigate-nextjs-auth") {
		t.Fatalf("expected the resolved handoff in --resolved listing:\n%s", resolvedListing)
	}

	viewResolved := runHandoffs(t, srv.port, "shriyanss", "view", id)
	if !strings.Contains(viewResolved, "Status:  resolved") || !strings.Contains(viewResolved, "by shriyanss") {
		t.Fatalf("resolved handoff should retain content and resolution metadata:\n%s", viewResolved)
	}

	// It must not come back on the next reconciliation, for either party.
	syncOnce(t, srv.port, "shriyanss", shriDir, 5*time.Second)
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("resolved handoff was resurrected for the assignee (err = %v)", err)
	}
	syncOnce(t, srv.port, "alice", aliceDir, 5*time.Second)
	if _, err := os.Stat(filepath.Join(aliceDir, "handoffs", "shriyanss", "investigate-nextjs-auth.md")); !os.IsNotExist(err) {
		t.Fatalf("resolved handoff was not retired for the creator (err = %v)", err)
	}

	// Real git history: create, update, resolve.
	out, err := exec.Command("git", "-C", wsPath, "log", "--format=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	gitLog := string(out)
	for _, want := range []string{"handoff:"} {
		if !strings.Contains(gitLog, want) {
			t.Errorf("git history missing %q:\n%s", want, gitLog)
		}
	}
	bodyOut, err := exec.Command("git", "-C", wsPath, "log", "--format=%s%n%b").CombinedOutput()
	if err != nil {
		t.Fatalf("git log body: %v: %s", err, bodyOut)
	}
	for _, want := range []string{"handoff create", "handoff update", "handoff resolve", "[" + id + "]"} {
		if !strings.Contains(string(bodyOut), want) {
			t.Errorf("git history missing %q:\n%s", want, bodyOut)
		}
	}
}

// TestE2EHandoffMismatchedMetadataIsRejected exercises the issue's central
// validation requirement end to end: a handoff whose directory and `to`
// metadata disagree must be rejected with an explicit error, by both the
// client and the server, and never delivered anywhere.
func TestE2EHandoffMismatchedMetadataIsRejected(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19611, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))

	aliceDir := filepath.Join(dir, "alice")
	os.MkdirAll(filepath.Join(aliceDir, "handoffs", "alice"), 0o755)
	// Lives under alice/ but claims shriyanss -- Rex must not pick a winner.
	mismatched := filepath.Join(aliceDir, "handoffs", "alice", "nextjs.md")
	os.WriteFile(mismatched, []byte("---\nrex_handoff_id: aaa111\nfrom: alice\nto: shriyanss\n---\n\nbad\n"), 0o644)

	out, err := runHandoffsExpectErr(t, srv.port, "alice", "sync", "--dir", aliceDir)
	if err == nil {
		t.Fatalf("expected `rex handoffs sync` to exit non-zero on a mismatch, got:\n%s", out)
	}
	for _, want := range []string{"invalid handoff metadata", "Directory assignee: alice", "Metadata assignee:  shriyanss"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// Nothing was stored server-side.
	listing := runHandoffs(t, srv.port, "shriyanss", "list", "--all")
	if strings.Contains(listing, "nextjs") {
		t.Fatalf("a mismatched handoff must not be synchronized:\n%s", listing)
	}

	// Confirmed at the server boundary too, not just the client: an upload
	// with the same disagreement is rejected even bypassing the CLI's local
	// validation, by driving the real HTTP endpoint the client would hit.
	body := "---\nrex_handoff_id: bbb222\nfrom: alice\nto: shriyanss\n---\n\nbad\n"
	token := loginForAPI(t, srv.port, "alice")
	status := putFileRaw(t, srv.port, token, "handoffs/alice/other.md", body)
	if status != 422 {
		t.Fatalf("server PUT status = %d, want 422 for a dir/metadata mismatch", status)
	}
}

// TestE2EHandoffScopingAcrossThreeUsers verifies the default `rex sync`
// scope live, against a real server: a handoff between two other users must
// never appear in a third user's workspace, while handoffs given to or sent
// by that user do.
func TestE2EHandoffScopingAcrossThreeUsers(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19612, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))

	// bob hands carol something that has nothing to do with a third user.
	bobDir := filepath.Join(dir, "bob")
	os.MkdirAll(bobDir, 0o755)
	writeHandoff(t, bobDir, "carol", "inspect-ad", "# Inspect AD")
	syncOnce(t, srv.port, "bob", bobDir, 5*time.Second)

	// alice hands shriyanss something, and shriyanss hands alice something.
	aliceDir := filepath.Join(dir, "alice")
	os.MkdirAll(aliceDir, 0o755)
	writeHandoff(t, aliceDir, "shriyanss", "nextjs", "# Investigate Next.js")
	syncOnce(t, srv.port, "alice", aliceDir, 5*time.Second)

	meDir := filepath.Join(dir, "shriyanss")
	os.MkdirAll(meDir, 0o755)
	writeHandoff(t, meDir, "alice", "investigate-api", "# Over to you")
	syncOnce(t, srv.port, "shriyanss", meDir, 5*time.Second) // pushes mine, pulls alice's
	syncOnce(t, srv.port, "shriyanss", meDir, 5*time.Second) // settle

	for _, p := range []string{
		filepath.Join("handoffs", "shriyanss", "nextjs.md"),      // given to me
		filepath.Join("handoffs", "alice", "investigate-api.md"), // sent by me
	} {
		if _, err := os.Stat(filepath.Join(meDir, p)); err != nil {
			t.Errorf("rex sync should materialize %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(meDir, "handoffs", "carol", "inspect-ad.md")); !os.IsNotExist(err) {
		t.Errorf("a handoff between two other users must not appear in my workspace (err = %v)", err)
	}

	// carol's workspace is the mirror image: her handoff, nothing of mine.
	carolDir := filepath.Join(dir, "carol")
	os.MkdirAll(carolDir, 0o755)
	syncOnce(t, srv.port, "carol", carolDir, 5*time.Second)
	syncOnce(t, srv.port, "carol", carolDir, 5*time.Second)
	if _, err := os.Stat(filepath.Join(carolDir, "handoffs", "carol", "inspect-ad.md")); err != nil {
		t.Errorf("carol should receive the handoff addressed to her: %v", err)
	}
	for _, p := range []string{
		filepath.Join("handoffs", "shriyanss", "nextjs.md"),
		filepath.Join("handoffs", "alice", "investigate-api.md"),
	} {
		if _, err := os.Stat(filepath.Join(carolDir, p)); !os.IsNotExist(err) {
			t.Errorf("carol should not see %s (err = %v)", p, err)
		}
	}

	// `--all` opts back into everything, on demand.
	allListing := runHandoffs(t, srv.port, "shriyanss", "list", "--all")
	for _, want := range []string{"nextjs", "investigate-api", "inspect-ad"} {
		if !strings.Contains(allListing, want) {
			t.Errorf("--all listing missing %q:\n%s", want, allListing)
		}
	}
}

// TestE2EHandoffRawDeleteRefused confirms the server-side safety net: even a
// raw file DELETE (bypassing `rex handoffs remove` entirely) cannot destroy
// the canonical copy of an open handoff.
func TestE2EHandoffRawDeleteRefused(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, 19613, filepath.Join(dir, "rex.db"), filepath.Join(dir, "ws"))

	aliceDir := filepath.Join(dir, "alice")
	os.MkdirAll(aliceDir, 0o755)
	writeHandoff(t, aliceDir, "alice", "self", "# Self-assigned task")
	syncOnce(t, srv.port, "alice", aliceDir, 5*time.Second)

	token := loginForAPI(t, srv.port, "alice")
	status := deleteFileRaw(t, srv.port, token, "handoffs/alice/self.md")
	if status != 422 {
		t.Fatalf("raw DELETE status = %d, want 422 (retiring a handoff must go through resolve)", status)
	}

	listing := runHandoffs(t, srv.port, "alice", "list")
	if !strings.Contains(listing, "self") {
		t.Fatalf("handoff should have survived the refused delete:\n%s", listing)
	}
}

// loginForAPI logs in against the real running server over HTTP and returns
// a bearer token, for tests that need to drive the REST API directly rather
// than through a rex subprocess -- this is how the server-side validation
// (as opposed to the CLI's own pre-check) gets exercised end to end.
func loginForAPI(t *testing.T, port int, username string) string {
	t.Helper()
	resp, err := insecureHTTPClient.Post(localURL(port, "/api/v1/auth/login"), "application/json",
		strings.NewReader(`{"username":"`+username+`","password":"hunter2"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var lr struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return lr.Token
}

func putFileRaw(t *testing.T, port int, token, path, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, localURL(port, "/api/v1/files/"+path), strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := insecureHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func deleteFileRaw(t *testing.T, port int, token, path string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, localURL(port, "/api/v1/files/"+path), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Base-Revision", "1")
	resp, err := insecureHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func localURL(port int, path string) string {
	return "https://localhost:" + strconv.Itoa(port) + path
}
