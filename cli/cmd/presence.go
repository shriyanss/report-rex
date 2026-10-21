package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// presenceIn is one newline-delimited JSON command read from stdin, sent by
// an editor integration (e.g. the Neovim plugin in cli/editors/nvim).
type presenceIn struct {
	Op    string `json:"op"` // "update" | "release"
	File  string `json:"file"`
	State string `json:"state,omitempty"` // "viewing" | "editing"
	Dirty bool   `json:"dirty,omitempty"`
}

// presenceOut is one newline-delimited JSON event written to stdout.
type presenceOut struct {
	Type      string `json:"type"` // "presence" | "status" | "remote_change" | "conflict_created"
	Username  string `json:"username,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	File      string `json:"file,omitempty"`
	State     string `json:"state,omitempty"`
	Dirty     bool   `json:"dirty,omitempty"`
	Action    string `json:"action,omitempty"`
	Connected bool   `json:"connected,omitempty"`
	Error     string `json:"error,omitempty"`

	// remote_change (a server file_updated broadcast) / conflict_created.
	Revision   int64  `json:"revision,omitempty"`
	ModifiedBy string `json:"modified_by,omitempty"`
	Deleted    bool   `json:"deleted,omitempty"`
	ConflictID int64  `json:"conflict_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

var presenceFlagsVal syncFlags

var presenceCmd = &cobra.Command{
	Use:   "presence",
	Short: "Bridge stdin/stdout JSON presence commands to a rex-svr's advisory presence protocol, for editor integrations",
	RunE:  runPresence,
}

func init() {
	f := presenceCmd.Flags()
	f.StringVar(&presenceFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&presenceFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&presenceFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&presenceFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&presenceFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&presenceFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")

	rootCmd.AddCommand(presenceCmd)
}

func newSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is effectively unrecoverable, but presence is
		// advisory: fall back to a fixed-ish value rather than crashing an
		// editor integration over it.
		return "session-fallback"
	}
	return hex.EncodeToString(b)
}

func resolvePresencePassword(flags syncFlags) (string, error) {
	if flags.Password != "" {
		return flags.Password, nil
	}
	if pw := os.Getenv("REX_PASSWORD"); pw != "" {
		return pw, nil
	}
	fmt.Fprint(os.Stderr, "Server password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pw), nil
}

// runPresence logs in, dials the presence protocol, and bridges
// stdin/stdout JSON lines to it until stdin closes or the process is
// signaled. Any connection failure is reported on stdout as a status
// event and otherwise swallowed -- an editor integration driving this
// process must never be blocked from editing by a presence-layer failure.
func runPresence(cmd *cobra.Command, args []string) error {
	flags := presenceFlagsVal
	if flags.Host == "" || flags.Username == "" {
		return fmt.Errorf("--host and --username are required")
	}
	password, err := resolvePresencePassword(flags)
	if err != nil {
		return err
	}
	tlsConfig, err := buildTLSConfig(flags)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	client := syncclient.NewClient(flags.Host, flags.Port, tlsConfig)
	if _, err := client.Login(ctx, flags.Username, password); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	sessionID := newSessionID()
	out := json.NewEncoder(os.Stdout)
	var outMu sync.Mutex
	writeOut := func(ev presenceOut) {
		outMu.Lock()
		defer outMu.Unlock()
		_ = out.Encode(ev)
	}

	var trackedMu sync.Mutex
	tracked := map[string]presenceIn{} // last known state per file, for reconnect replay

	onEvent := func(ev syncclient.PresenceEvent) {
		writeOut(presenceOut{
			Type: "presence", Username: ev.Username, SessionID: ev.SessionID,
			File: ev.Path, State: string(ev.State), Dirty: ev.Dirty, Action: ev.Action,
		})
	}

	// Forward live server activity too: this connection stays up whether
	// or not the editor's `rex sync` is running, so it's what lets an
	// editor show "bob just changed index.md" or a new conflict right away.
	onServer := func(ev syncclient.ServerEvent) {
		switch ev.Type {
		case syncproto.WSTypeFileUpdated:
			writeOut(presenceOut{Type: "remote_change", File: ev.Path, Revision: ev.Revision,
				ModifiedBy: ev.ModifiedBy, Deleted: ev.Deleted})
		case syncproto.WSTypeConflictCreated:
			writeOut(presenceOut{Type: "conflict_created", File: ev.Path, ConflictID: ev.ConflictID, Kind: string(ev.Kind)})
		}
	}

	var sess *syncclient.PresenceSession
	onStatus := func(status syncclient.ConnStatus, connErr error) {
		switch status {
		case syncclient.StatusConnected:
			writeOut(presenceOut{Type: "status", Connected: true})
			// A fresh connection means the server has no leases for this
			// session yet -- replay everything we're currently tracking so
			// a reconnect never silently drops presence.
			trackedMu.Lock()
			for file, in := range tracked {
				sess.Update(file, sessionID, syncproto.PresenceState(in.State), in.Dirty)
			}
			trackedMu.Unlock()
		case syncclient.StatusDisconnected, syncclient.StatusReconnecting:
			msg := ""
			if connErr != nil {
				msg = connErr.Error()
			}
			writeOut(presenceOut{Type: "status", Connected: false, Error: msg})
		}
	}
	sess = syncclient.DialPresence(ctx, flags.Host, flags.Port, tlsConfig, client, onEvent, onServer, onStatus)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var in presenceIn
		if err := json.Unmarshal(scanner.Bytes(), &in); err != nil {
			continue // malformed input line: presence is advisory, never fatal
		}
		if in.File == "" {
			continue
		}
		switch in.Op {
		case "update":
			trackedMu.Lock()
			tracked[in.File] = in
			trackedMu.Unlock()
			sess.Update(in.File, sessionID, syncproto.PresenceState(in.State), in.Dirty)
		case "release":
			trackedMu.Lock()
			delete(tracked, in.File)
			trackedMu.Unlock()
			sess.Release(in.File, sessionID)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}
