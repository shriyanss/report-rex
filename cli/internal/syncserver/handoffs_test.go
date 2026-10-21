package syncserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func handoffBody(id, from, to string) []byte {
	return []byte(fmt.Sprintf("---\nrex_handoff_id: %s\nfrom: %s\nto: %s\nstatus: open\ncreated_at: \"2026-08-22T17:15:00-04:00\"\n---\n\n# Investigate\n\nSome context.\n", id, from, to))
}

// putHandoff uploads a handoff and returns the response status plus the new
// revision (0 when the upload was rejected).
func putHandoff(t *testing.T, ts *httptest.Server, token, path string, body []byte, base int64) (int, int64) {
	t.Helper()
	headers := map[string]string{}
	if base > 0 {
		headers[syncproto.HeaderBaseRevision] = strconv.FormatInt(base, 10)
	}
	resp := doReq(t, ts, "POST", "/api/v1/files/"+path, token, headers, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, 0
	}
	var pr syncproto.PutFileResponse
	json.NewDecoder(resp.Body).Decode(&pr)
	return resp.StatusCode, pr.Revision
}

func listHandoffs(t *testing.T, ts *httptest.Server, token, query string) syncproto.HandoffListResponse {
	t.Helper()
	resp := doReq(t, ts, "GET", "/api/v1/handoffs"+query, token, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /handoffs%s status = %d", query, resp.StatusCode)
	}
	var lr syncproto.HandoffListResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode handoff list: %v", err)
	}
	return lr
}

func TestHandoffUploadRejectsDirMetadataMismatch(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	// The file lives in alice's queue but claims shriyanss. Rex must not
	// pick a winner, and must not store it either.
	resp := doReq(t, ts, "POST", "/api/v1/files/handoffs/alice/nextjs.md", token, nil,
		handoffBody("8f83c1", "alice", "shriyanss"))
	body, _ := readAll(resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", resp.StatusCode, body)
	}
	for _, want := range []string{"Directory assignee: alice", "Metadata assignee:  shriyanss"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}

	// Nothing was stored, so nothing can be synchronized.
	if _, err := s.Store().Get(t.Context(), "handoffs/alice/nextjs.md"); err == nil {
		t.Error("a mismatched handoff must not be stored")
	}
}

func TestHandoffUploadRejectsSpoofedCreator(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	status, _ := putHandoff(t, ts, token, "handoffs/shriyanss/x.md", handoffBody("aaa111", "bob", "shriyanss"), 0)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for a `from` that isn't the authenticated user", status)
	}
}

func TestHandoffUploadRejectsMalformedNamespacePaths(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	for _, path := range []string{"handoffs/loose.md", "handoffs/alice/sub/deep.md", "handoffs/alice/notes.txt"} {
		status, _ := putHandoff(t, ts, token, path, handoffBody("aaa111", "alice", "alice"), 0)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", path, status)
		}
	}
}

func TestHandoffUploadRejectsIdentityMutationAndHandEditedResolve(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	alice := loginAndGetToken(t, ts, "alice")
	bob := loginAndGetToken(t, ts, "bob")

	path := "handoffs/shriyanss/x.md"
	status, rev := putHandoff(t, ts, alice, path, handoffBody("8f83c1", "alice", "shriyanss"), 0)
	if status != http.StatusOK {
		t.Fatalf("create status = %d", status)
	}

	// Another user editing the handoff can't take over authorship.
	if status, _ := putHandoff(t, ts, bob, path, handoffBody("8f83c1", "bob", "shriyanss"), rev); status != http.StatusUnprocessableEntity {
		t.Errorf("from-change status = %d, want 422", status)
	}
	// Nor can the stable id be swapped out from under it.
	if status, _ := putHandoff(t, ts, alice, path, handoffBody("ffffff", "alice", "shriyanss"), rev); status != http.StatusUnprocessableEntity {
		t.Errorf("id-change status = %d, want 422", status)
	}
	// Hand-editing `status: resolved` would leave the file materialized in
	// every subscriber's workspace, so it has to go through resolve.
	handEdited := []byte("---\nrex_handoff_id: 8f83c1\nfrom: alice\nto: shriyanss\nstatus: resolved\n---\n\nbody\n")
	if status, _ := putHandoff(t, ts, alice, path, handEdited, rev); status != http.StatusUnprocessableEntity {
		t.Errorf("hand-edited resolve status = %d, want 422", status)
	}

	// A legitimate body edit still goes through.
	updated := []byte("---\nrex_handoff_id: 8f83c1\nfrom: alice\nto: shriyanss\nstatus: open\n---\n\n## Findings\n\nFound an endpoint.\n")
	if status, _ := putHandoff(t, ts, bob, path, updated, rev); status != http.StatusOK {
		t.Errorf("body-edit status = %d, want 200", status)
	}
}

func TestHandoffListScopes(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	alice := loginAndGetToken(t, ts, "alice")
	shriyanss := loginAndGetToken(t, ts, "shriyanss")

	putHandoff(t, ts, alice, "handoffs/shriyanss/nextjs.md", handoffBody("8f83c1", "alice", "shriyanss"), 0)
	putHandoff(t, ts, shriyanss, "handoffs/alice/api.md", handoffBody("c183bf", "shriyanss", "alice"), 0)
	putHandoff(t, ts, alice, "handoffs/charlie/ad.md", handoffBody("6dac91", "alice", "charlie"), 0)

	// Default scope: only the authenticated user's own queue.
	own := listHandoffs(t, ts, shriyanss, "")
	if len(own.Handoffs) != 1 || own.Handoffs[0].ID != "8f83c1" {
		t.Fatalf("default scope = %+v, want just 8f83c1", own.Handoffs)
	}
	if own.Handoffs[0].From != "alice" || own.Handoffs[0].Title != "nextjs" {
		t.Errorf("unexpected entry: %+v", own.Handoffs[0])
	}

	// Another user's queue.
	other := listHandoffs(t, ts, shriyanss, "?user=alice")
	if len(other.Handoffs) != 1 || other.Handoffs[0].ID != "c183bf" {
		t.Fatalf("--user alice scope = %+v", other.Handoffs)
	}

	// Everyone.
	all := listHandoffs(t, ts, shriyanss, "?all=1")
	if len(all.Handoffs) != 3 {
		t.Fatalf("--all scope returned %d handoffs, want 3", len(all.Handoffs))
	}
}

func TestHandoffListReportsInvalidFiles(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	// Sneak a mismatched handoff past the upload validator by writing it
	// straight to the store -- this stands in for a file that was hand-edited
	// in the server's git workspace.
	if _, err := s.Store().Put(t.Context(), "handoffs/alice/nextjs.md",
		handoffBody("8f83c1", "alice", "shriyanss"), nil, "alice"); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	lr := listHandoffs(t, ts, token, "?all=1")
	if len(lr.Handoffs) != 0 {
		t.Errorf("an invalid handoff must not appear as a valid one: %+v", lr.Handoffs)
	}
	if len(lr.Invalid) != 1 {
		t.Fatalf("len(invalid) = %d, want 1", len(lr.Invalid))
	}
	if lr.Invalid[0].DirAssignee != "alice" || lr.Invalid[0].MetaAssignee != "shriyanss" {
		t.Errorf("unexpected invalid entry: %+v", lr.Invalid[0])
	}
}

func TestHandoffViewWorksWithoutSyncing(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	alice := loginAndGetToken(t, ts, "alice")
	shriyanss := loginAndGetToken(t, ts, "shriyanss")

	putHandoff(t, ts, alice, "handoffs/shriyanss/nextjs.md", handoffBody("8f83c1", "alice", "shriyanss"), 0)

	resp := doReq(t, ts, "GET", "/api/v1/handoffs/8f83c1", shriyanss, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view status = %d", resp.StatusCode)
	}
	var d syncproto.HandoffDetail
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if d.From != "alice" || d.To != "shriyanss" || d.Status != handoff.StatusOpen {
		t.Errorf("unexpected detail: %+v", d.HandoffEntry)
	}
	if !strings.Contains(string(d.Content), "Some context.") {
		t.Errorf("content not returned: %q", d.Content)
	}

	resp = doReq(t, ts, "GET", "/api/v1/handoffs/nope99", shriyanss, nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown id status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHandoffResolveRetiresPathButKeepsHistory(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	alice := loginAndGetToken(t, ts, "alice")
	shriyanss := loginAndGetToken(t, ts, "shriyanss")

	path := "handoffs/shriyanss/nextjs.md"
	putHandoff(t, ts, alice, path, handoffBody("8f83c1", "alice", "shriyanss"), 0)

	resp := doReq(t, ts, "POST", "/api/v1/handoffs/8f83c1/resolve", shriyanss, nil, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := readAll(resp)
		t.Fatalf("resolve status = %d: %s", resp.StatusCode, body)
	}
	var rr syncproto.HandoffResolveResponse
	json.NewDecoder(resp.Body).Decode(&rr)
	resp.Body.Close()
	if rr.Path != path {
		t.Errorf("resolve response path = %q", rr.Path)
	}

	// The active path is retired, so subscribers stop materializing it.
	rec, err := s.Store().Get(t.Context(), path)
	if err != nil {
		t.Fatalf("Get after resolve: %v", err)
	}
	if !rec.Deleted {
		t.Error("resolve should tombstone the active path")
	}

	// It disappears from the default (open-only) listing...
	if open := listHandoffs(t, ts, shriyanss, ""); len(open.Handoffs) != 0 {
		t.Errorf("resolved handoff still listed as open: %+v", open.Handoffs)
	}
	// ...but stays visible, with its resolution metadata, when asked for.
	res := listHandoffs(t, ts, shriyanss, "?resolved=1")
	if len(res.Handoffs) != 1 {
		t.Fatalf("--resolved listing = %+v, want 1 entry", res.Handoffs)
	}
	if got := res.Handoffs[0]; got.Status != handoff.StatusResolved || got.ResolvedBy != "shriyanss" || got.ResolvedAt == "" {
		t.Errorf("unexpected resolution metadata: %+v", got)
	}

	// And the content is still readable -- resolution retains history.
	resp = doReq(t, ts, "GET", "/api/v1/handoffs/8f83c1", shriyanss, nil, nil)
	var d syncproto.HandoffDetail
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if !strings.Contains(string(d.Content), "Some context.") {
		t.Errorf("resolved handoff content not retained: %q", d.Content)
	}
	if d.ResolvedBy != "shriyanss" {
		t.Errorf("resolved detail = %+v", d.HandoffEntry)
	}

	// Resolving twice is an error, not a silent second tombstone.
	resp = doReq(t, ts, "POST", "/api/v1/handoffs/8f83c1/resolve", shriyanss, nil, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second resolve status = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHandoffResolveEnqueuesGitOperations(t *testing.T) {
	s := newTestServer(t)
	rec := &recordingMaterializer{}
	s.materializer = rec
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	putHandoff(t, ts, token, "handoffs/alice/self.md", handoffBody("8f83c1", "alice", "alice"), 0)
	doReq(t, ts, "POST", "/api/v1/handoffs/8f83c1/resolve", token, nil, nil).Body.Close()

	// create, resolved-metadata write, tombstone -- all three versioned.
	if len(rec.ops) != 3 || rec.ops[0] != "write" || rec.ops[1] != "write" || rec.ops[2] != "delete" {
		t.Errorf("materializer ops = %v, want [write write delete]", rec.ops)
	}
}

type recordingMaterializer struct{ ops []string }

func (r *recordingMaterializer) Enqueue(path, op string, revision int64, username string) {
	r.ops = append(r.ops, op)
}
func (r *recordingMaterializer) Healthy() bool     { return true }
func (r *recordingMaterializer) PendingCount() int { return 0 }

func TestCommitMessageUsesHandoffVocabulary(t *testing.T) {
	msg := commitMessage([]pendingChange{
		{path: "handoffs/shriyanss/nextjs.md", op: "write", username: "alice", revision: 1, handoffID: "8f83c1"},
	})
	if !strings.HasPrefix(msg, "handoff: 1 file(s) by alice") {
		t.Errorf("subject = %q", strings.SplitN(msg, "\n", 2)[0])
	}
	if !strings.Contains(msg, "- handoff create handoffs/shriyanss/nextjs.md [8f83c1] (alice)") {
		t.Errorf("body missing the create line:\n%s", msg)
	}

	msg = commitMessage([]pendingChange{
		{path: "handoffs/shriyanss/nextjs.md", op: "delete", username: "shriyanss", revision: 3, handoffID: "8f83c1"},
	})
	if !strings.Contains(msg, "- handoff resolve handoffs/shriyanss/nextjs.md [8f83c1] (shriyanss)") {
		t.Errorf("body missing the resolve line:\n%s", msg)
	}

	// A mixed batch keeps the generic subject and per-path verbs.
	msg = commitMessage([]pendingChange{
		{path: "handoffs/shriyanss/nextjs.md", op: "write", username: "alice", revision: 2, handoffID: "8f83c1"},
		{path: "reports/vuln-1.md", op: "write", username: "alice", revision: 4},
	})
	if !strings.HasPrefix(msg, "sync: 2 file(s) by alice") {
		t.Errorf("mixed-batch subject = %q", strings.SplitN(msg, "\n", 2)[0])
	}
	if !strings.Contains(msg, "- handoff update handoffs/shriyanss/nextjs.md [8f83c1]") ||
		!strings.Contains(msg, "- modified reports/vuln-1.md") {
		t.Errorf("mixed-batch body wrong:\n%s", msg)
	}
}

func TestWSPrefixScopesSubscription(t *testing.T) {
	_, ts, token := newTestServerWithHub(t)

	// A client that only materializes shriyanss's handoff queue subscribes
	// to that prefix and must not be woken by anything else.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	u, _ := url.Parse(wsURL)
	q := u.Query()
	q.Set("token", token)
	q.Set("prefix", handoff.ScopePrefix("shriyanss"))
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	// Out of scope: another user's handoff, and an ordinary report file.
	putHandoff(t, ts, token, "handoffs/alice/other.md", handoffBody("aaa111", "alice", "alice"), 0)
	doReq(t, ts, "POST", "/api/v1/files/reports/vuln-1.md", token, nil, []byte("x")).Body.Close()
	// In scope.
	putHandoff(t, ts, token, "handoffs/shriyanss/mine.md", handoffBody("bbb222", "alice", "shriyanss"), 0)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got syncproto.WSFileUpdated
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	if got.Path != "handoffs/shriyanss/mine.md" {
		t.Errorf("first delivered event = %q, want only the in-scope handoff", got.Path)
	}
}

// TestHandoffRawDeleteIsRefused: the canonical file is retained on the server,
// so a client can't retire a handoff by deleting the path -- it has to resolve
// it, which records who finished it and when.
func TestHandoffRawDeleteIsRefused(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	path := "handoffs/alice/self.md"
	_, rev := putHandoff(t, ts, token, path, handoffBody("8f83c1", "alice", "alice"), 0)

	resp := doReq(t, ts, "DELETE", "/api/v1/files/"+path, token,
		map[string]string{syncproto.HeaderBaseRevision: strconv.FormatInt(rev, 10)}, nil)
	body, _ := readAll(resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "resolve") {
		t.Errorf("error should point at the resolve operation: %s", body)
	}

	// Still there, still open.
	rec, err := s.Store().Get(t.Context(), path)
	if err != nil || rec.Deleted {
		t.Fatalf("handoff should survive a refused delete (err = %v)", err)
	}

	// Resolving works, and *its* tombstone is allowed through.
	doReq(t, ts, "POST", "/api/v1/handoffs/8f83c1/resolve", token, nil, nil).Body.Close()
	rec, err = s.Store().Get(t.Context(), path)
	if err != nil || !rec.Deleted {
		t.Fatalf("resolve should retire the path (err = %v)", err)
	}
}
