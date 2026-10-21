package syncserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func postRevert(t *testing.T, ts *httptest.Server, token, endpoint string, req, out any) int {
	t.Helper()
	body, _ := json.Marshal(req)
	hr, _ := http.NewRequest("POST", ts.URL+"/api/v1/"+endpoint, bytes.NewReader(body))
	hr.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatalf("%s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", endpoint, err)
		}
	}
	return resp.StatusCode
}

func put(t *testing.T, s *Store, m *GitMaterializer, path, content, user string) *FileRecord {
	t.Helper()
	var base *int64
	if cur, err := s.Get(context.Background(), path); err == nil && !cur.Deleted {
		base = &cur.Revision
	}
	rec, err := s.Put(context.Background(), path, []byte(content), base, user)
	if err != nil {
		t.Fatalf("Put %s: %v", path, err)
	}
	m.Enqueue(path, "write", rec.Revision, user)
	m.Flush(context.Background())
	return rec
}

func TestRevertRestoresPointInTimeAsNewRevisions(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()
	adminHash, _ := HashPassword("admin-pw")
	hash, _ := HashPassword("test-password")
	srv := NewServer(Config{PasswordHash: hash, AdminPasswordHash: adminHash}, m.db, m, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "admin")

	put(t, store, m, "index.md", "# good", "alice")
	good := put(t, store, m, "reports/vuln-1.md", "good finding", "alice")
	// The bad stuff: an edit, a new file, a deletion, and a handoff.
	put(t, store, m, "reports/vuln-1.md", "BAD finding", "mallory")
	put(t, store, m, "reports/vuln-2.md", "junk", "mallory")
	idx, _ := store.Get(ctx, "index.md")
	if _, err := store.Delete(ctx, "index.md", idx.Revision, "mallory"); err != nil {
		t.Fatal(err)
	}
	put(t, store, m, "handoffs/bob/h1.md", "---\nid: h1\nto: bob\nfrom: alice\n---\n", "alice")

	req := syncproto.RevertRequest{Path: good.Path, Revision: good.Revision, AdminPassword: "admin-pw"}
	var p syncproto.RevertPreview
	if code := postRevert(t, ts, token, "revert/preview", req, &p); code != http.StatusOK {
		t.Fatalf("preview status = %d", code)
	}
	actions := map[string]string{}
	for _, op := range p.Ops {
		actions[op.Path] = op.Action
	}
	want := map[string]string{"index.md": "write", "reports/vuln-1.md": "write", "reports/vuln-2.md": "delete"}
	if len(actions) != len(want) {
		t.Fatalf("ops = %v, want %v", actions, want)
	}
	for k, v := range want {
		if actions[k] != v {
			t.Errorf("op[%s] = %q, want %q", k, actions[k], v)
		}
	}
	if !strings.Contains(p.Commit, "sync:") {
		t.Errorf("preview commit = %q, want the commit that recorded vuln-1 rev 1", p.Commit)
	}
	if len(p.GitLog) == 0 {
		t.Error("preview git log is empty")
	}

	// Anything changing after the preview invalidates its plan.
	put(t, store, m, "reports/vuln-2.md", "more junk", "mallory")
	req.Plan = p.Plan
	if code := postRevert(t, ts, token, "revert", req, nil); code != http.StatusConflict {
		t.Fatalf("stale apply status = %d, want 409", code)
	}
	postRevert(t, ts, token, "revert/preview", req, &p)
	req.Plan = p.Plan

	var res syncproto.RevertResponse
	if code := postRevert(t, ts, token, "revert", req, &res); code != http.StatusOK {
		t.Fatalf("apply status = %d", code)
	}
	if len(res.Ops) != 3 || res.Commit == "" {
		t.Fatalf("apply result = %+v", res)
	}

	v1, _ := store.Get(ctx, "reports/vuln-1.md")
	if string(v1.Content) != "good finding" || v1.Revision != 3 || v1.ModifiedBy != "admin" {
		t.Errorf("vuln-1 = %q rev %d by %s", v1.Content, v1.Revision, v1.ModifiedBy)
	}
	if ix, _ := store.Get(ctx, "index.md"); ix.Deleted || string(ix.Content) != "# good" {
		t.Errorf("index.md not restored: %+v", ix)
	}
	if v2, _ := store.Get(ctx, "reports/vuln-2.md"); !v2.Deleted {
		t.Error("vuln-2 should be deleted")
	}
	if h, _ := store.Get(ctx, "handoffs/bob/h1.md"); h.Deleted {
		t.Error("revert must leave handoffs alone")
	}
	// History is appended to, never rewritten.
	if c, _ := store.RevisionContent(ctx, "reports/vuln-1.md", 2); string(c) != "BAD finding" {
		t.Errorf("rex history lost the bad revision: %q", c)
	}
	log, _ := m.runner.Run("log", "--format=%s")
	if !strings.HasPrefix(log, "Revert to reports/vuln-1.md rev 1 by admin\n") || !strings.Contains(log, "by mallory") {
		t.Errorf("git log = %q", log)
	}

	// Reverting to where we already are is a no-op.
	postRevert(t, ts, token, "revert/preview", req, &p)
	if len(p.Ops) != 0 {
		t.Errorf("second preview ops = %+v, want none", p.Ops)
	}

	// Undoing the very first change empties the report (handoffs aside).
	first := syncproto.RevertRequest{Path: "index.md", Revision: 1, Before: true, AdminPassword: "admin-pw"}
	postRevert(t, ts, token, "revert/preview", first, &p)
	if len(p.Ops) != 2 || p.Ops[0].Action != "delete" || p.Ops[1].Action != "delete" {
		t.Errorf("before-first ops = %+v, want index.md and vuln-1 deleted", p.Ops)
	}
}

func TestRevertRequiresAdminPassword(t *testing.T) {
	s := newTestServer(t) // no AdminPasswordHash
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")
	req := syncproto.RevertRequest{Path: "index.md", Revision: 1, AdminPassword: "x"}
	if code := postRevert(t, ts, token, "revert/preview", req, nil); code != http.StatusForbidden {
		t.Errorf("disabled: status = %d, want 403", code)
	}

	s.cfg.AdminPasswordHash, _ = HashPassword("admin-pw")
	if code := postRevert(t, ts, token, "revert", req, nil); code != http.StatusForbidden {
		t.Errorf("wrong password: status = %d, want 403", code)
	}
	req.AdminPassword = "admin-pw"
	if code := postRevert(t, ts, token, "revert/preview", req, nil); code != http.StatusNotFound {
		t.Errorf("unknown revision: status = %d, want 404", code)
	}
}
