package syncserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func TestBulkDeleteNeedsAdminPasswordAndSkipsHandoffs(t *testing.T) {
	m, store := newTestMaterializer(t)
	ctx := context.Background()
	hash, _ := HashPassword("test-password")

	// No --admin-password: disabled outright.
	off := httptest.NewServer(NewServer(Config{PasswordHash: hash}, m.db, m, nil).Handler())
	defer off.Close()
	req := syncproto.BulkDeleteRequest{Paths: []string{"index.md"}, AdminPassword: "x"}
	if code := postRevert(t, off, loginAndGetToken(t, off, "alice"), "admin/delete", req, nil); code != http.StatusForbidden {
		t.Fatalf("disabled: status %d, want 403", code)
	}

	adminHash, _ := HashPassword("admin-pw")
	ts := httptest.NewServer(NewServer(Config{PasswordHash: hash, AdminPasswordHash: adminHash}, m.db, m, nil).Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	put(t, store, m, "index.md", "# report", "alice")
	put(t, store, m, "reports/vuln-1.md", "finding", "alice")
	put(t, store, m, "handoffs/bob/h1.md", "---\nid: h1\nto: bob\nfrom: alice\n---\n", "alice")

	if code := postRevert(t, ts, token, "admin/delete", req, nil); code != http.StatusForbidden {
		t.Fatalf("wrong admin password: status %d, want 403", code)
	}

	req = syncproto.BulkDeleteRequest{
		Paths:         []string{"index.md", "reports/vuln-1.md", "handoffs/bob/h1.md", "nope.md", ".vscode/settings.json"},
		AdminPassword: "admin-pw",
	}
	var resp syncproto.BulkDeleteResponse
	if code := postRevert(t, ts, token, "admin/delete", req, &resp); code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if len(resp.Deleted) != 2 || len(resp.Skipped) != 3 || resp.Commit == "" {
		t.Fatalf("resp = %+v, want 2 deleted, 3 skipped, a commit", resp)
	}
	for _, p := range []string{"index.md", "reports/vuln-1.md"} {
		if rec, _ := store.Get(ctx, p); !rec.Deleted {
			t.Errorf("%s not deleted", p)
		}
	}
	if rec, _ := store.Get(ctx, "handoffs/bob/h1.md"); rec.Deleted {
		t.Error("bulk delete must leave handoffs alone")
	}
}

func TestAdminVerify(t *testing.T) {
	m, _ := newTestMaterializer(t)
	hash, _ := HashPassword("test-password")
	adminHash, _ := HashPassword("admin-pw")
	off := httptest.NewServer(NewServer(Config{PasswordHash: hash}, m.db, m, nil).Handler())
	defer off.Close()
	ts := httptest.NewServer(NewServer(Config{PasswordHash: hash, AdminPasswordHash: adminHash}, m.db, m, nil).Handler())
	defer ts.Close()
	for _, c := range []struct {
		srv  *httptest.Server
		pw   string
		want int
	}{{off, "admin-pw", http.StatusForbidden}, {ts, "nope", http.StatusForbidden}, {ts, "admin-pw", http.StatusOK}} {
		req := syncproto.AdminVerifyRequest{AdminPassword: c.pw}
		if code := postRevert(t, c.srv, loginAndGetToken(t, c.srv, "alice"), "admin/verify", req, nil); code != c.want {
			t.Errorf("pw %q: status %d, want %d", c.pw, code, c.want)
		}
	}
}

func TestDeleteFileRefusesMassDelete(t *testing.T) {
	store := newTestDB(t)
	hash, _ := HashPassword("test-password")
	ts := httptest.NewServer(NewServer(Config{PasswordHash: hash}, store.db, nil, nil).Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	var revs []int64
	for i := range 10 {
		rec, err := store.Put(context.Background(), fmt.Sprintf("f%d.md", i), []byte("x"), nil, "alice")
		if err != nil {
			t.Fatal(err)
		}
		revs = append(revs, rec.Revision)
	}
	del := func(i int) int {
		resp := doReq(t, ts, "DELETE", fmt.Sprintf("/api/v1/files/f%d.md", i), token,
			map[string]string{syncproto.HeaderBaseRevision: fmt.Sprint(revs[i])}, nil)
		resp.Body.Close()
		return resp.StatusCode
	}
	// The kth delete is refused once k/10 > 90%: nine go through, the last doesn't.
	for i := range 9 {
		if code := del(i); code != http.StatusOK {
			t.Fatalf("delete %d: status %d, want 200", i, code)
		}
	}
	if code := del(9); code != http.StatusUnprocessableEntity {
		t.Fatalf("last delete: status %d, want 422", code)
	}
	if rec, _ := store.Get(context.Background(), "f9.md"); rec.Deleted {
		t.Fatal("refused delete still tombstoned the file")
	}
}
