package syncclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// ErrPresenceDisconnected is returned by PresenceSession.Update/Release
// while no WebSocket connection is currently live. Presence is fail-open,
// so callers should log and ignore this rather than treat it as fatal.
var ErrPresenceDisconnected = fmt.Errorf("presence: not connected")

// PresenceSession is a send-capable connection to the presence protocol on
// /api/v1/ws, used by `rex presence`. Unlike ListenWithReconnect
// (receive-only, used by `rex sync`), it can also send
// presence_update/presence_release messages. The underlying connection is
// swapped out transparently across reconnects; Send methods fail open
// (return an error, never block or panic) whenever no connection is
// currently live.
type PresenceSession struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (s *PresenceSession) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Held across the write, not just the read of s.conn: gorilla allows
	// one concurrent writer, and the stdin loop and the reconnect replay
	// both send.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return ErrPresenceDisconnected
	}
	return s.conn.WriteMessage(websocket.TextMessage, b)
}

// Update reports (or renews) presence for path under sessionID.
func (s *PresenceSession) Update(path, sessionID string, state syncproto.PresenceState, dirty bool) error {
	return s.send(syncproto.WSPresenceUpdate{
		Type: syncproto.WSTypePresenceUpdate, Path: path, SessionID: sessionID, State: state, Dirty: dirty,
	})
}

// Release immediately drops sessionID's presence on path.
func (s *PresenceSession) Release(path, sessionID string) error {
	return s.send(syncproto.WSPresenceRelease{
		Type: syncproto.WSTypePresenceRelease, Path: path, SessionID: sessionID,
	})
}

func (s *PresenceSession) setConn(conn *websocket.Conn) {
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
}

func (s *PresenceSession) clearConn(conn *websocket.Conn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.mu.Unlock()
}

// ServerEvent is a non-presence broadcast (file_updated or
// conflict_created) surfaced to presence clients, so an editor holding the
// always-on presence connection can show live team activity and conflicts
// even while its own `rex sync` is stopped.
type ServerEvent struct {
	Type       syncproto.WSMessageType
	Path       string
	Revision   int64
	ModifiedBy string
	Deleted    bool
	ConflictID int64
	Kind       syncproto.ConflictKind
}

// DialPresence starts a background reconnect-with-backoff loop against
// /api/v1/ws and returns a PresenceSession immediately, so the caller can
// start calling Update/Release right away (they fail open while
// disconnected). onEvent receives inbound presence broadcasts from other
// sessions; onServer (optional) receives file_updated/conflict_created
// broadcasts. onStatus receives connect status transitions -- including on
// every reconnect, which is the caller's cue to replay its own
// currently-tracked file states via Update, since a dropped connection
// also drops the server's leases for this session.
func DialPresence(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, onEvent func(PresenceEvent), onServer func(ServerEvent), onStatus func(ConnStatus, error)) *PresenceSession {
	if onStatus == nil {
		onStatus = func(ConnStatus, error) {}
	}
	s := &PresenceSession{}

	go func() {
		backoff := time.Second
		const maxBackoff = 30 * time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			err := s.connectAndPump(ctx, host, port, tlsConfig, client, onEvent, onServer, onStatus)
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
	}()

	return s
}

func (s *PresenceSession) connectAndPump(ctx context.Context, host string, port int, tlsConfig *tls.Config, client *Client, onEvent func(PresenceEvent), onServer func(ServerEvent), onStatus func(ConnStatus, error)) error {
	conn, err := dialWS(ctx, host, port, tlsConfig, client, "")
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	keepAlive(conn)

	s.setConn(conn)
	defer s.clearConn(conn)

	onStatus(StatusConnected, nil)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("websocket closed: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		var env syncproto.WSEnvelope
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		switch env.Type {
		case syncproto.WSTypeFileUpdated:
			var m syncproto.WSFileUpdated
			if onServer != nil && json.Unmarshal(data, &m) == nil {
				onServer(ServerEvent{Type: env.Type, Path: m.Path, Revision: m.Revision, ModifiedBy: m.ModifiedBy, Deleted: m.Deleted})
			}
			continue
		case syncproto.WSTypeConflictCreated:
			var m syncproto.WSConflictCreated
			if onServer != nil && json.Unmarshal(data, &m) == nil {
				onServer(ServerEvent{Type: env.Type, Path: m.Path, ConflictID: m.ConflictID, Kind: m.Kind})
			}
			continue
		}
		if onEvent == nil || env.Type != syncproto.WSTypePresence {
			continue
		}
		var p syncproto.WSPresence
		if json.Unmarshal(data, &p) != nil {
			continue
		}
		onEvent(PresenceEvent{
			Username: p.Username, SessionID: p.SessionID, Path: p.Path,
			State: p.State, Dirty: p.Dirty, Action: p.Action,
		})
	}
}
