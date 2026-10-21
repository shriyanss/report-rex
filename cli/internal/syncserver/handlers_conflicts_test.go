package syncserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func createDivergentConflict(t *testing.T, ts *httptest.Server, token string) syncproto.ConflictError {
	t.Helper()
	doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v1")).Body.Close()
	resp := doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v2-conflicting"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	var ce syncproto.ConflictError
	json.NewDecoder(resp.Body).Decode(&ce)
	return ce
}

func TestResolveConflictUseLocalCreatesNewRevision(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	ce := createDivergentConflict(t, ts, token)

	reqBody, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionUseLocal})
	resp := doReq(t, ts, "POST", "/api/v1/conflicts/"+itoa(ce.ConflictID)+"/resolve", token, nil, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d", resp.StatusCode)
	}
	var rr syncproto.ResolveResponse
	json.NewDecoder(resp.Body).Decode(&rr)
	resp.Body.Close()
	if rr.NewRevision == nil || *rr.NewRevision != 2 {
		t.Fatalf("NewRevision = %v, want 2", rr.NewRevision)
	}

	resp = doReq(t, ts, "GET", "/api/v1/files/x.md", token, nil, nil)
	body, _ := readAll(resp)
	if string(body) != "v2-conflicting" {
		t.Errorf("content after use_local = %q, want %q", body, "v2-conflicting")
	}
}

func TestResolveConflictKeepBothCreatesDerivedPath(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	doReq(t, ts, "POST", "/api/v1/files/screenshots/proof.png", token, nil, []byte("remote-bytes")).Body.Close()
	resp := doReq(t, ts, "POST", "/api/v1/files/screenshots/proof.png", token, nil, []byte("local-bytes"))
	var ce syncproto.ConflictError
	json.NewDecoder(resp.Body).Decode(&ce)
	resp.Body.Close()

	reqBody, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionKeepBoth, KeepBothSuffix: "alice"})
	resp = doReq(t, ts, "POST", "/api/v1/conflicts/"+itoa(ce.ConflictID)+"/resolve", token, nil, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d", resp.StatusCode)
	}
	var rr syncproto.ResolveResponse
	json.NewDecoder(resp.Body).Decode(&rr)
	resp.Body.Close()

	wantPath := "screenshots/proof.conflict-alice.png"
	if rr.NewPath != wantPath {
		t.Fatalf("NewPath = %q, want %q", rr.NewPath, wantPath)
	}

	// Original path keeps the remote bytes; new path holds the local bytes.
	resp = doReq(t, ts, "GET", "/api/v1/files/screenshots/proof.png", token, nil, nil)
	orig, _ := readAll(resp)
	if string(orig) != "remote-bytes" {
		t.Errorf("original content = %q, want remote-bytes", orig)
	}
	resp = doReq(t, ts, "GET", "/api/v1/files/"+wantPath, token, nil, nil)
	derived, _ := readAll(resp)
	if string(derived) != "local-bytes" {
		t.Errorf("derived content = %q, want local-bytes", derived)
	}
}

func TestResolveConflictDeleteEditKeepDeletion(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v1")).Body.Close()
	doReq(t, ts, "DELETE", "/api/v1/files/x.md", token, map[string]string{syncproto.HeaderBaseRevision: "1"}, nil).Body.Close()

	// Simulate a delete/edit conflict directly against the store, since
	// producing one via HTTP requires an offline client with a stale base;
	// the handler-level behavior is what's under test here.
	baseRev := int64(1)
	_, _, err := s.store.CreateConflict(context.Background(), Conflict{
		Path: "x.md", Kind: ConflictKindDeleteEdit, BaseRevision: &baseRev,
		LocalContent: []byte("bob offline edit"), LocalHash: "h", LocalUsername: "bob",
		RemoteRevision: 2, RemoteHash: "",
	})
	if err != nil {
		t.Fatalf("CreateConflict: %v", err)
	}

	conflicts, err := s.store.ListOpenConflicts(context.Background())
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("ListOpenConflicts: %v, len=%d", err, len(conflicts))
	}
	id := conflicts[0].ID

	reqBody, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionKeepDeletion})
	resp := doReq(t, ts, "POST", "/api/v1/conflicts/"+itoa(id)+"/resolve", token, nil, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doReq(t, ts, "GET", "/api/v1/files/x.md", token, nil, nil)
	if resp.StatusCode != http.StatusGone {
		t.Errorf("status after keep_deletion = %d, want 410 (still deleted)", resp.StatusCode)
	}
	resp.Body.Close()
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// A merge built on remote revision N must not land once someone has pushed
// N+1: it never saw that edit, and writing it would silently drop it.
func TestResolveMergeRefusesStaleRemote(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	ce := createDivergentConflict(t, ts, token) // x.md is at rev 1
	built := ce.RemoteRevision
	// bob pushes rev 2 while alice is merging against rev 1.
	doReq(t, ts, "POST", "/api/v1/files/x.md", token, map[string]string{syncproto.HeaderBaseRevision: "1"}, []byte("bob")).Body.Close()

	stale, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionMerge, MergedContent: []byte("merged"), ExpectedRemoteRevision: &built})
	resp := doReq(t, ts, "POST", "/api/v1/conflicts/"+itoa(ce.ConflictID)+"/resolve", token, nil, stale)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale merge: status = %d, want 409", resp.StatusCode)
	}
	resp = doReq(t, ts, "GET", "/api/v1/files/x.md", token, nil, nil)
	if body, _ := readAll(resp); string(body) != "bob" {
		t.Fatalf("content after refused merge = %q, want bob's edit intact", body)
	}

	// Re-fetching the detail gives the current revision to merge on top of.
	resp = doReq(t, ts, "GET", "/api/v1/conflicts/"+itoa(ce.ConflictID)+"?include=remote", token, nil, nil)
	var d syncproto.ConflictDetail
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if d.RemoteContentRevision != 2 {
		t.Fatalf("remote_content_revision = %d, want 2", d.RemoteContentRevision)
	}
	fresh, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionMerge, MergedContent: []byte("merged"), ExpectedRemoteRevision: &d.RemoteContentRevision})
	resp = doReq(t, ts, "POST", "/api/v1/conflicts/"+itoa(ce.ConflictID)+"/resolve", token, nil, fresh)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh merge: status = %d, want 200", resp.StatusCode)
	}
}
