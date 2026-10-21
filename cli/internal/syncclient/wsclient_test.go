package syncclient

import (
	"context"
	"crypto/tls"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/syncserver"
)

var testTLSConfig = &tls.Config{InsecureSkipVerify: true}

func newWSTestServer(t *testing.T) (*httptest.Server, string, int) {
	t.Helper()
	dir := t.TempDir()
	db, err := syncserver.OpenDB(dir + "/test.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	hash, err := syncserver.HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	hub := syncserver.NewWSHub()
	srv := syncserver.NewServer(syncserver.Config{Addr: ":0", PasswordHash: hash}, db, nil, hub)
	ts := httptest.NewTLSServer(srv.Handler())
	t.Cleanup(ts.Close)
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	return ts, u.Hostname(), port
}

func TestListenWithReconnectFiresConnectedAndImmediateTrigger(t *testing.T) {
	ts, host, port := newWSTestServer(t)
	_ = ts

	client := NewClient(host, port, testTLSConfig)
	_, err := client.Login(context.Background(), "alice", "test-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trigger := make(chan struct{}, 4)
	statuses := make(chan ConnStatus, 4)
	go ListenWithReconnect(ctx, host, port, testTLSConfig, client, trigger, func(s ConnStatus, err error) {
		select {
		case statuses <- s:
		default:
		}
	}, nil)

	select {
	case s := <-statuses:
		if s != StatusConnected {
			t.Fatalf("first status = %v, want StatusConnected", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for StatusConnected")
	}

	select {
	case <-trigger:
	case <-time.After(2 * time.Second):
		t.Fatal("expected an immediate trigger fired right after connecting")
	}
}

// TestListenWithReconnectReportsDisconnectAndRetries points at a port with
// nothing listening, so the very first dial fails -- this deterministically
// exercises the disconnected -> backoff -> reconnecting status transitions
// without depending on how reliably a real server teardown severs an
// already-hijacked websocket connection (which httptest.Server.Close()
// does not guarantee, making that scenario flaky to test directly here;
// it's covered instead by the ssh gcis live multi-client verification).
func TestListenWithReconnectReportsDisconnectAndRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trigger := make(chan struct{}, 8)
	statuses := make(chan ConnStatus, 8)
	go ListenWithReconnect(ctx, "127.0.0.1", 1, testTLSConfig, NewClient("127.0.0.1", 1, testTLSConfig), trigger, func(s ConnStatus, err error) {
		select {
		case statuses <- s:
		default:
		}
	}, nil)

	select {
	case s := <-statuses:
		if s != StatusDisconnected {
			t.Fatalf("first status = %v, want StatusDisconnected", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for StatusDisconnected")
	}

	select {
	case s := <-statuses:
		if s != StatusReconnecting {
			t.Fatalf("status = %v, want StatusReconnecting", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for StatusReconnecting")
	}
}

// TestClientReloginsWhenServerForgetsSession: rex-svr keeps sessions in its
// database, so after a restart on a fresh one a long-running client's token
// is unknown (issue #62). HTTP calls -- including ones with a body to replay --
// and the websocket must log in again instead of failing until restarted.
func TestClientReloginsWhenServerForgetsSession(t *testing.T) {
	_, host, port := newWSTestServer(t)
	ctx := context.Background()
	client := NewClient(host, port, testTLSConfig)
	if _, err := client.Login(ctx, "alice", "test-password"); err != nil {
		t.Fatalf("login: %v", err)
	}

	client.SetToken("forgotten-by-the-server")
	if _, _, err := client.PutFile(ctx, "index.md", []byte("body survives the retry"), nil); err != nil {
		t.Fatalf("PutFile after session loss: %v", err)
	}
	content, _, _, ok, err := client.GetFile(ctx, "index.md")
	if err != nil || !ok || string(content) != "body survives the retry" {
		t.Fatalf("GetFile = %q, %v, %v", content, ok, err)
	}

	client.SetToken("forgotten-by-the-server")
	wsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	connected := make(chan struct{}, 1)
	go ListenWithReconnect(wsCtx, host, port, testTLSConfig, client, make(chan struct{}, 1), func(s ConnStatus, _ error) {
		if s == StatusConnected {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	}, nil)
	select {
	case <-connected:
	case <-wsCtx.Done():
		t.Fatal("websocket never reconnected after the server forgot the session")
	}
}
