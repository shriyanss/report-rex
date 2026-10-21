package syncserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

func newTestServerWithHub(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir + "/test.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	hub := NewWSHub()
	srv := NewServer(Config{Addr: ":0", PasswordHash: hash}, db, nil, hub)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	token := loginAndGetToken(t, ts, "alice")
	return srv, ts, token
}

func TestWSHubBroadcastsFileUpdated(t *testing.T) {
	_, ts, token := newTestServerWithHub(t)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	u, _ := url.Parse(wsURL)
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	// Give the server a moment to register the connection before we push.
	time.Sleep(50 * time.Millisecond)

	resp := doReq(t, ts, "POST", "/api/v1/files/index.md", token, nil, []byte("hello"))
	if resp.StatusCode != 200 {
		t.Fatalf("put status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	var fu syncproto.WSFileUpdated
	if err := json.Unmarshal(msg, &fu); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fu.Type != syncproto.WSTypeFileUpdated || fu.Path != "index.md" || fu.Revision != 1 {
		t.Errorf("unexpected message: %+v", fu)
	}
}

func TestWSHubBroadcastsPresenceFromUpdate(t *testing.T) {
	_, ts, tokenAlice := newTestServerWithHub(t)
	tokenBob := loginAndGetToken(t, ts, "bob")

	dial := func(token string) *websocket.Conn {
		u, _ := url.Parse("ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws")
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return conn
	}

	aliceConn := dial(tokenAlice)
	defer aliceConn.Close()
	bobConn := dial(tokenBob)
	defer bobConn.Close()

	time.Sleep(50 * time.Millisecond)

	upd := syncproto.WSPresenceUpdate{
		Type: syncproto.WSTypePresenceUpdate, Path: "findings/XSS-001.md",
		SessionID: "bob-session-1", State: syncproto.PresenceEditing, Dirty: true,
	}
	b, _ := json.Marshal(upd)
	if err := bobConn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write presence_update: %v", err)
	}

	aliceConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := aliceConn.ReadMessage()
	if err != nil {
		t.Fatalf("alice read presence: %v", err)
	}
	var p syncproto.WSPresence
	if err := json.Unmarshal(msg, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Type != syncproto.WSTypePresence || p.Username != "bob" || p.SessionID != "bob-session-1" ||
		p.Path != "findings/XSS-001.md" || p.State != syncproto.PresenceEditing || !p.Dirty || p.Action != "active" {
		t.Errorf("unexpected presence message: %+v", p)
	}
}

func TestWSHubBroadcastsPresenceIdleOnRelease(t *testing.T) {
	_, ts, tokenAlice := newTestServerWithHub(t)
	tokenBob := loginAndGetToken(t, ts, "bob")

	dial := func(token string) *websocket.Conn {
		u, _ := url.Parse("ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws")
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return conn
	}

	aliceConn := dial(tokenAlice)
	defer aliceConn.Close()
	bobConn := dial(tokenBob)
	defer bobConn.Close()

	time.Sleep(50 * time.Millisecond)

	upd := syncproto.WSPresenceUpdate{
		Type: syncproto.WSTypePresenceUpdate, Path: "findings/XSS-001.md",
		SessionID: "bob-session-1", State: syncproto.PresenceEditing, Dirty: true,
	}
	b, _ := json.Marshal(upd)
	bobConn.WriteMessage(websocket.TextMessage, b)

	aliceConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	aliceConn.ReadMessage() // the "active" broadcast from the update above

	rel := syncproto.WSPresenceRelease{Type: syncproto.WSTypePresenceRelease, Path: "findings/XSS-001.md", SessionID: "bob-session-1"}
	rb, _ := json.Marshal(rel)
	if err := bobConn.WriteMessage(websocket.TextMessage, rb); err != nil {
		t.Fatalf("write presence_release: %v", err)
	}

	aliceConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := aliceConn.ReadMessage()
	if err != nil {
		t.Fatalf("alice read idle presence: %v", err)
	}
	var p syncproto.WSPresence
	if err := json.Unmarshal(msg, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Action != "idle" || p.Username != "bob" || p.SessionID != "bob-session-1" {
		t.Errorf("unexpected idle presence message: %+v", p)
	}
}

func TestHandleListPresenceReturnsActiveLeases(t *testing.T) {
	_, ts, tokenAlice := newTestServerWithHub(t)
	tokenBob := loginAndGetToken(t, ts, "bob")

	u, _ := url.Parse("ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws")
	q := u.Query()
	q.Set("token", tokenBob)
	u.RawQuery = q.Encode()
	bobConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer bobConn.Close()
	time.Sleep(50 * time.Millisecond)

	upd := syncproto.WSPresenceUpdate{
		Type: syncproto.WSTypePresenceUpdate, Path: "findings/XSS-001.md",
		SessionID: "bob-session-1", State: syncproto.PresenceEditing, Dirty: true,
	}
	b, _ := json.Marshal(upd)
	bobConn.WriteMessage(websocket.TextMessage, b)
	time.Sleep(50 * time.Millisecond) // let the server process the update

	resp := doReq(t, ts, "GET", "/api/v1/presence", tokenAlice, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var pr syncproto.PresenceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(pr.Presence) != 1 {
		t.Fatalf("Presence = %+v, want 1 entry", pr.Presence)
	}
	e := pr.Presence[0]
	if e.Username != "bob" || e.SessionID != "bob-session-1" || e.Path != "findings/XSS-001.md" ||
		e.State != syncproto.PresenceEditing || !e.Dirty {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestWSRejectedWhenNoHubConfigured(t *testing.T) {
	s := newTestServer(t) // uses NoopNotifier
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := loginAndGetToken(t, ts, "alice")

	u, _ := url.Parse("ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws")
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	_, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err == nil {
		t.Fatal("expected dial to fail without a configured hub")
	}
	if resp != nil && resp.StatusCode != 501 {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}
