package syncserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func loginAndGetToken(t *testing.T, ts *httptest.Server, username string) string {
	t.Helper()
	body, _ := json.Marshal(syncproto.LoginRequest{Username: username, Password: "test-password"})
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var lr syncproto.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return lr.Token
}

func TestHandleLoginRejectsBadPassword(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body, _ := json.Marshal(syncproto.LoginRequest{Username: "alice", Password: "wrong"})
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHandleManifestRequiresAuth(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/manifest")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func doReq(t *testing.T, ts *httptest.Server, method, path, token string, headers map[string]string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestPutGetDeleteFileRoundTrip(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	content := []byte("# Hello Report")
	resp := doReq(t, ts, "POST", "/api/v1/files/index.md", token,
		map[string]string{syncproto.HeaderSHA256: synccommon.HashBytes(content)}, content)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}
	var putResp syncproto.PutFileResponse
	json.NewDecoder(resp.Body).Decode(&putResp)
	resp.Body.Close()
	if putResp.Revision != 1 {
		t.Errorf("Revision = %d, want 1", putResp.Revision)
	}

	resp = doReq(t, ts, "GET", "/api/v1/files/index.md", token, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d", resp.StatusCode)
	}
	got, _ := readAll(resp)
	if string(got) != string(content) {
		t.Errorf("GET body = %q, want %q", got, content)
	}
	if rev := resp.Header.Get(syncproto.HeaderRevision); rev != "1" {
		t.Errorf("X-Revision = %q, want 1", rev)
	}

	resp = doReq(t, ts, "DELETE", "/api/v1/files/index.md", token,
		map[string]string{syncproto.HeaderBaseRevision: "1"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d", resp.StatusCode)
	}

	resp = doReq(t, ts, "GET", "/api/v1/files/index.md", token, nil, nil)
	if resp.StatusCode != http.StatusGone {
		t.Errorf("GET after delete status = %d, want 410", resp.StatusCode)
	}
}

func TestPutFileStaleBaseReturnsConflict(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	content := []byte("v1")
	resp := doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, content)
	resp.Body.Close()

	// No base revision on an existing path -> stale/conflict.
	resp = doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v2"))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	var ce syncproto.ConflictError
	json.NewDecoder(resp.Body).Decode(&ce)
	resp.Body.Close()
	if ce.ConflictID == 0 {
		t.Error("expected a conflict_id")
	}

	// The conflict should now be listed.
	resp = doReq(t, ts, "GET", "/api/v1/conflicts", token, nil, nil)
	var lr syncproto.ConflictsResponse
	json.NewDecoder(resp.Body).Decode(&lr)
	resp.Body.Close()
	if len(lr.Conflicts) != 1 {
		t.Fatalf("len(conflicts) = %d, want 1", len(lr.Conflicts))
	}
}

func TestPutFileRejectsPathTraversal(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	resp := doReq(t, ts, "POST", "/api/v1/files/../../etc/passwd", token, nil, []byte("pwned"))
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 400 or 404 (mux-normalized)", resp.StatusCode)
	}
}

func TestResolveConflictUseRemoteDoesNotChangeServerState(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v1")).Body.Close()
	resp := doReq(t, ts, "POST", "/api/v1/files/x.md", token, nil, []byte("v2-conflicting"))
	var ce syncproto.ConflictError
	json.NewDecoder(resp.Body).Decode(&ce)
	resp.Body.Close()

	reqBody, _ := json.Marshal(syncproto.ResolveRequest{Resolution: syncproto.ResolutionUseRemote})
	resp = doReq(t, ts, "POST", "/api/v1/conflicts/1/resolve", token, nil, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Server-side revision should still be 1 (unchanged).
	resp = doReq(t, ts, "GET", "/api/v1/files/x.md", token, nil, nil)
	if rev := resp.Header.Get(syncproto.HeaderRevision); rev != "1" {
		t.Errorf("revision after use_remote = %q, want 1 (unchanged)", rev)
	}
	resp.Body.Close()
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var hr syncproto.HealthResponse
	json.NewDecoder(resp.Body).Decode(&hr)
	if !hr.DBOK {
		t.Error("expected DBOK = true")
	}
}

func readAll(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}
