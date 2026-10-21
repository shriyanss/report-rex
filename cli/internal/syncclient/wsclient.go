package syncclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// ConnStatus describes a transition in the WebSocket connection's state,
// reported via ListenWithReconnect's onStatus callback so the CLI can
// print a "Connection lost... Reconnecting... Synced" style status line.
type ConnStatus int

const (
	StatusConnected ConnStatus = iota
	StatusDisconnected
	StatusReconnecting
)

// wsReadTimeout is how long a client waits without hearing anything from
// the server before treating the connection as dead and reconnecting. The
// server pings every 30s, so this tolerates two missed pings; without a
// deadline, a half-open connection (laptop sleep, NAT timeout) would block
// ReadMessage forever and the client would never notice it went deaf.
const wsReadTimeout = 75 * time.Second

// keepAlive arms conn's read deadline and extends it on every server ping.
// Callers also extend it after each message read.
func keepAlive(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPingHandler(func(data string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		err := conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
}

// PresenceEvent is a best-effort, advisory "who's editing what" hint. It
// is never required for correct synchronization -- a dropped or delayed
// presence event has no effect on file sync.
type PresenceEvent struct {
	Username  string
	SessionID string
	Path      string
	State     syncproto.PresenceState
	Dirty     bool
	Action    string // "active" | "idle"
}

// listenForChanges connects to the server's /api/v1/ws endpoint, reports
// StatusConnected once the handshake completes, and signals trigger
// (non-blocking) whenever a file_updated or conflict_created event
// arrives. It also fires trigger once immediately after connecting, so a
// reconnect always forces a full reconcile pass rather than waiting for
// the next unrelated server event; this is what actually reconciles any
// changes made while offline. Advisory presence messages are reported via
// onPresence instead of triggering a reconcile, since they carry no sync
// state of their own.
//
// The client deliberately doesn't parse file_updated/conflict_created
// messages into incremental state; it just triggers a full
// Engine.Reconcile pass, which is idempotent and cheap when nothing
// actually changed. This keeps sync-loop-prevention simple: a change the
// client itself just pushed shows up as a WS echo, but the resulting
// reconcile finds local/remote already equal and does nothing.
//
// A non-empty prefix narrows the subscription to paths under it, so a
// client that only materializes part of the tree isn't woken by the rest.
func listenForChanges(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, prefix string, trigger chan<- struct{}, onStatus func(ConnStatus, error), onPresence func(PresenceEvent)) error {
	conn, err := dialWS(ctx, host, port, tlsConfig, client, prefix)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	keepAlive(conn)
	onStatus(StatusConnected, nil)

	select {
	case trigger <- struct{}{}:
	default:
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("websocket closed: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		handleIncomingMessage(data, trigger, onPresence)
	}
}

// dialWS opens /api/v1/ws with client's current token. A 401 means the
// server no longer knows that token (restarted on a fresh database), so it
// logs in again and the caller's next reconnect attempt uses the new one.
func dialWS(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, prefix string) (*websocket.Conn, error) {
	u := url.URL{Scheme: "wss", Host: fmt.Sprintf("%s:%d", host, port), Path: "/api/v1/ws"}
	q := u.Query()
	q.Set("token", client.Token())
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	u.RawQuery = q.Encode()

	dialer := &websocket.Dialer{TLSClientConfig: tlsConfig}
	conn, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			_ = client.Relogin(ctx)
		}
		return nil, fmt.Errorf("connect websocket: %w", err)
	}
	return conn, nil
}

func handleIncomingMessage(data []byte, trigger chan<- struct{}, onPresence func(PresenceEvent)) {
	var env syncproto.WSEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case syncproto.WSTypePresence:
		if onPresence == nil {
			return
		}
		var p syncproto.WSPresence
		if err := json.Unmarshal(data, &p); err != nil {
			return
		}
		onPresence(PresenceEvent{
			Username: p.Username, SessionID: p.SessionID, Path: p.Path,
			State: p.State, Dirty: p.Dirty, Action: p.Action,
		})
	case syncproto.WSTypeFileUpdated, syncproto.WSTypeConflictCreated:
		select {
		case trigger <- struct{}{}:
		default:
		}
	}
}

// ListenWithReconnect runs listenForChanges in a loop with exponential
// backoff, reporting connect/disconnect/reconnecting transitions via
// onStatus (pass nil for no status reporting) and advisory presence events
// via onPresence (pass nil to ignore them). It never returns until ctx is
// canceled -- callers rely on the periodic reconcile ticker and filesystem
// watcher as fallbacks while a reconnect is in progress, so the user can
// keep editing locally the entire time the server is unreachable. On every
// successful reconnect, the caller's next Engine.Reconcile pass performs
// the full "rescan local, refetch remote, reconcile offline changes" flow
// via the immediate post-connect trigger above.
func ListenWithReconnect(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, trigger chan<- struct{}, onStatus func(ConnStatus, error), onPresence func(PresenceEvent)) {
	ListenWithReconnectScoped(ctx, host, port, tlsConfig, client, "", trigger, onStatus, onPresence)
}

// ListenWithReconnectScoped is ListenWithReconnect narrowed to one path
// prefix, for a client that only materializes part of the workspace (e.g.
// `rex handoffs sync --user alice`). An empty prefix means "everything".
func ListenWithReconnectScoped(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, prefix string, trigger chan<- struct{}, onStatus func(ConnStatus, error), onPresence func(PresenceEvent)) {
	if onStatus == nil {
		onStatus = func(ConnStatus, error) {}
	}
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		err := listenForChanges(ctx, host, port, tlsConfig, client, prefix, trigger, onStatus, onPresence)
		if ctx.Err() != nil {
			return
		}
		onStatus(StatusDisconnected, err)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		onStatus(StatusReconnecting, nil)
	}
}
