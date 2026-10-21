package syncserver

import "net/http"

// handleWS upgrades to a WebSocket connection and registers the client
// with the server's WSHub, if one is configured as the notifier. If the
// server was constructed with NoopNotifier (e.g. in tests that don't need
// live notifications), WS upgrades are rejected -- clients fall back to
// polling GET /manifest.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	hub, ok := s.notifier.(*WSHub)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not_implemented", "websocket notifications not available on this server")
		return
	}
	// An optional ?prefix= narrows this connection's subscription; clients
	// that want everything (the default, and what `rex sync` uses) omit it.
	hub.handle(w, r, usernameFromContext(r.Context()), r.URL.Query().Get("prefix"))
}
