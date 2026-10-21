package syncserver

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

const (
	wsWriteTimeout = 10 * time.Second
	wsPingInterval = 30 * time.Second
	wsPongTimeout  = 3*wsPingInterval + 5*time.Second // ~3 missed pings
)

var upgrader = websocket.Upgrader{
	// The sync server is intended for a trusted CPTC LAN, not arbitrary
	// browser origins; skip origin checking rather than mis-configuring it.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHub is the real Notifier implementation: it fans out file_updated and
// conflict_created events to every connected /api/v1/ws client, and (Phase
// 9) advisory edit-presence events driven by client heartbeats.
type WSHub struct {
	mu       sync.Mutex
	clients  map[*wsConn]bool
	presence *PresenceTracker
	stopOnce sync.Once
	stop     chan struct{}
}

func NewWSHub() *WSHub {
	h := &WSHub{
		clients:  map[*wsConn]bool{},
		presence: NewPresenceTracker(),
		stop:     make(chan struct{}),
	}
	go h.sweepPresenceLoop()
	return h
}

// Close stops the hub's background presence-sweep loop. Safe to call more
// than once.
func (h *WSHub) Close() {
	h.stopOnce.Do(func() { close(h.stop) })
}

func (h *WSHub) sweepPresenceLoop() {
	ticker := time.NewTicker(PresenceHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-ticker.C:
			for _, exp := range h.presence.Sweep() {
				h.broadcastPresence(exp, "idle")
			}
		}
	}
}

type wsConn struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
	// prefix optionally narrows this connection's subscription to one path
	// prefix, so a client that only materializes part of the tree (e.g.
	// `rex handoffs sync --user alice`) isn't woken by events it would
	// ignore. Empty means "everything", which is what `rex sync` requests.
	prefix string
}

// wants reports whether this connection subscribes to events about path.
// Events with no path (if any are ever added) go to everyone.
func (c *wsConn) wants(path string) bool {
	return c.prefix == "" || path == "" || strings.HasPrefix(path, c.prefix)
}

func (h *WSHub) register(c *wsConn) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func (h *WSHub) unregister(c *wsConn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

func (h *WSHub) broadcast(payload []byte, path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if !c.wants(path) {
			continue
		}
		select {
		case c.send <- payload:
		default:
			// Slow/stuck client: drop the message rather than blocking the
			// broadcaster. The client will still see current state on its
			// next GET /manifest (WS is a low-latency hint, not the
			// source of truth).
			log.Printf("syncserver: dropping WS message for a slow client")
		}
	}
}

func (h *WSHub) NotifyFileUpdated(path string, revision int64, sha256, modifiedBy string, deleted bool) {
	msg := syncproto.WSFileUpdated{
		Type: syncproto.WSTypeFileUpdated, Path: path, Revision: revision,
		SHA256: sha256, ModifiedBy: modifiedBy, Deleted: deleted,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcast(b, path)
}

func (h *WSHub) NotifyConflictCreated(conflictID int64, path string, kind ConflictKind) {
	msg := syncproto.WSConflictCreated{
		Type: syncproto.WSTypeConflictCreated, ConflictID: conflictID, Path: path, Kind: syncproto.ConflictKind(kind),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcast(b, path)
}

func (h *WSHub) broadcastPresence(entry FilePresence, action string) {
	msg := syncproto.WSPresence{
		Type: syncproto.WSTypePresence, Username: entry.Username, SessionID: entry.SessionID,
		Path: entry.Path, State: entry.State, Dirty: entry.Dirty, Action: action,
		ExpiresAt: entry.ExpiresAt.UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcast(b, entry.Path)
}

func (h *WSHub) handle(w http.ResponseWriter, r *http.Request, username, prefix string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsConn{conn: conn, send: make(chan []byte, 32), username: username, prefix: prefix}
	h.register(c)

	go h.writePump(c)
	h.readPump(c) // blocks until the connection closes
}

func (h *WSHub) readPump(c *wsConn) {
	defer func() {
		h.unregister(c)
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		return nil
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		h.handleClientMessage(c, data)
	}
}

// handleClientMessage handles the advisory presence messages the server
// reads from clients. Anything else (malformed JSON, unknown type) is
// silently ignored -- presence is never allowed to affect correctness, so
// a bad message must never be treated as a protocol error.
func (h *WSHub) handleClientMessage(c *wsConn, data []byte) {
	var env syncproto.WSEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case syncproto.WSTypePresenceUpdate:
		var u syncproto.WSPresenceUpdate
		if err := json.Unmarshal(data, &u); err != nil || u.Path == "" || u.SessionID == "" {
			return
		}
		h.presence.Update(c.username, u.SessionID, u.Path, u.State, u.Dirty)
		h.broadcastPresence(FilePresence{
			Username: c.username, SessionID: u.SessionID, Path: u.Path,
			State: u.State, Dirty: u.Dirty, ExpiresAt: time.Now().Add(PresenceTTL),
		}, "active")
	case syncproto.WSTypePresenceRelease:
		var rel syncproto.WSPresenceRelease
		if err := json.Unmarshal(data, &rel); err != nil || rel.Path == "" || rel.SessionID == "" {
			return
		}
		if h.presence.Release(c.username, rel.SessionID, rel.Path) {
			h.broadcastPresence(FilePresence{
				Username: c.username, SessionID: rel.SessionID, Path: rel.Path,
			}, "idle")
		}
	}
}

func (h *WSHub) writePump(c *wsConn) {
	ticker := time.NewTicker(wsPingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
