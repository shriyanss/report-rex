package syncserver

import (
	"context"
	"database/sql"
	"log"
	"net/http"
)

// Materializer receives notifications of accepted canonical changes so it
// can project them into the server's git workspace (Phase 2). Enqueue must
// not block the caller for long; it queues work for a background debounced
// flush.
type Materializer interface {
	Enqueue(path, op string, revision int64, username string)
	// Healthy reports whether git materialization is currently working
	// (used by GET /healthz).
	Healthy() bool
	// PendingCount returns the number of not-yet-materialized changes.
	PendingCount() int
}

// noopMaterializer is used until Phase 2 wires in the real git-backed
// implementation, and in unit tests that don't need materialization.
type noopMaterializer struct{}

func (noopMaterializer) Enqueue(string, string, int64, string) {}
func (noopMaterializer) Healthy() bool                         { return true }
func (noopMaterializer) PendingCount() int                     { return 0 }

// NoopMaterializer is the default Materializer: it accepts and discards
// enqueue calls. Exported so cmd/server can wire it in before Phase 2's
// git materializer exists, and so other packages' tests can use it.
var NoopMaterializer Materializer = noopMaterializer{}

// Notifier broadcasts change events to connected WebSocket clients
// (Phase 4). Implementations must not block the caller.
type Notifier interface {
	NotifyFileUpdated(path string, revision int64, sha256, modifiedBy string, deleted bool)
	NotifyConflictCreated(conflictID int64, path string, kind ConflictKind)
}

type noopNotifier struct{}

func (noopNotifier) NotifyFileUpdated(string, int64, string, string, bool) {}
func (noopNotifier) NotifyConflictCreated(int64, string, ConflictKind)     {}

// NoopNotifier discards all notifications. Used until Phase 4 wires in the
// real WebSocket-backed implementation.
var NoopNotifier Notifier = noopNotifier{}

// Config holds the server's runtime configuration, resolved from CLI flags
// by cmd/server before constructing a Server.
type Config struct {
	Addr         string // e.g. ":9443"
	PasswordHash string // bcrypt hash of the shared v1 password
	// AdminPasswordHash gates reverting the workspace to an earlier change;
	// empty disables reverts entirely.
	AdminPasswordHash string
	AllowedTextExt    map[string]bool
	CertFile          string // TLS certificate (PEM), see EnsureSelfSignedCert
	KeyFile           string // TLS private key (PEM), see EnsureSelfSignedCert
}

// Server wires together the DB-backed store, auth, materializer, and
// notifier behind an http.Handler.
type Server struct {
	cfg          Config
	db           *sql.DB
	store        *Store
	auth         *Authenticator
	materializer Materializer
	notifier     Notifier
	mux          *http.ServeMux
}

func NewServer(cfg Config, db *sql.DB, materializer Materializer, notifier Notifier) *Server {
	if materializer == nil {
		materializer = NoopMaterializer
	}
	if notifier == nil {
		notifier = NoopNotifier
	}
	s := &Server{
		cfg:          cfg,
		db:           db,
		store:        NewStore(db),
		auth:         NewAuthenticator(db),
		materializer: materializer,
		notifier:     notifier,
		mux:          http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

// Store exposes the underlying Store for wiring (e.g. the Materializer's
// drift-repair pass needs read access) without leaking the *sql.DB.
func (s *Server) Store() *Store { return s.store }

// ListenAndServe serves HTTPS/WSS only -- all client-server communication
// must be encrypted, so there is no plaintext fallback.
func (s *Server) ListenAndServe() error {
	log.Printf("rex-svr listening on %s (TLS)", s.cfg.Addr)
	return http.ListenAndServeTLS(s.cfg.Addr, s.cfg.CertFile, s.cfg.KeyFile, s.mux)
}

// HealthStatus is a small helper for GET /healthz and startup logging.
func (s *Server) HealthStatus(ctx context.Context) (dbOK, gitOK bool, pending int) {
	dbOK = s.db.PingContext(ctx) == nil
	gitOK = s.materializer.Healthy()
	pending = s.materializer.PendingCount()
	return
}
