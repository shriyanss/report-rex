package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

type syncFlags struct {
	Host     string
	Port     int
	Username string
	Password string
	Dir      string
	CACert   string
	Insecure bool
	JSON     bool
}

var syncFlagsVal syncFlags

// syncOutput is where `rex sync` reports what it's doing: the
// human-readable log by default, or one JSON object per line (--json) for
// editor integrations, which previously had to scrape the text.
type syncOutput struct {
	json bool
	mu   sync.Mutex
}

// syncLine is one --json output line. Type is one of ready, status (State:
// connected|offline|reconnecting), event (Kind: pushed|pulled|deleted|
// conflict|merged|skipped), error (Scope: reconcile|watch|handoffs), or
// stopped.
type syncLine struct {
	Type         string `json:"type"`
	Time         string `json:"time"`
	State        string `json:"state,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Path         string `json:"path,omitempty"`
	Detail       string `json:"detail,omitempty"`
	ConflictID   int64  `json:"conflict_id,omitempty"`
	ConflictKind string `json:"conflict_kind,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Message      string `json:"message,omitempty"`
	Server       string `json:"server,omitempty"`
	User         string `json:"user,omitempty"`
	Dir          string `json:"dir,omitempty"`
}

// syncOut is package-level so reconcileWorkspace's helpers (also driven
// directly by tests) share the running command's output mode.
var syncOut = &syncOutput{}

func (o *syncOutput) emit(l syncLine) {
	l.Time = time.Now().UTC().Format(time.RFC3339Nano)
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = json.NewEncoder(os.Stdout).Encode(l)
}

func (o *syncOutput) ready(f syncFlags) {
	if o.json {
		o.emit(syncLine{Type: "ready", Server: fmt.Sprintf("%s:%d", f.Host, f.Port), User: f.Username, Dir: f.Dir})
		return
	}
	fmt.Printf("Report Rex Sync\n\nServer:    %s:%d\nUser:      %s\nWorkspace: %s\n\nConnected.\n\n",
		f.Host, f.Port, f.Username, f.Dir)
}

func (o *syncOutput) status(state string, text string, err error) {
	if o.json {
		l := syncLine{Type: "status", State: state}
		if err != nil {
			l.Message = err.Error()
		}
		o.emit(l)
		return
	}
	fmt.Println(text)
}

func (o *syncOutput) event(ev syncclient.Event) {
	if o.json {
		o.emit(syncLine{Type: "event", Kind: string(ev.Kind), Path: ev.Path, Detail: ev.Detail,
			ConflictID: ev.ConflictID, ConflictKind: ev.ConflictKind})
		return
	}
	ts := time.Now().Format("15:04:05")
	if ev.Detail != "" {
		fmt.Printf("[%s] %-9s %s (%s)\n", ts, ev.Kind, ev.Path, ev.Detail)
	} else {
		fmt.Printf("[%s] %-9s %s\n", ts, ev.Kind, ev.Path)
	}
	if ev.Kind == syncclient.EventConflict {
		fmt.Printf("           run `rex conflicts` to view, `rex resolve %s --id %d` to fix\n", ev.Path, ev.ConflictID)
	}
}

func (o *syncOutput) errorf(scope string, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if o.json {
		o.emit(syncLine{Type: "error", Scope: scope, Message: msg})
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", scope, msg)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Continuously synchronize a report workspace with a rex-svr",
	RunE:  runSync,
}

func init() {
	f := syncCmd.Flags()
	f.StringVar(&syncFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&syncFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&syncFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&syncFlagsVal.Password, "password", "", "shared server password (prompted interactively if omitted)")
	f.StringVar(&syncFlagsVal.Dir, "dir", ".", "workspace directory to synchronize")
	f.StringVar(&syncFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&syncFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&syncFlagsVal.JSON, "json", false, "emit newline-delimited JSON events instead of the human-readable log, and read stdin: {\"op\":\"reconcile\"} syncs immediately, EOF stops sync (for editor integrations)")

	rootCmd.AddCommand(syncCmd)
}

func resolveSyncPassword(flags syncFlags) (string, error) {
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

func runSync(cmd *cobra.Command, args []string) error {
	flags := syncFlagsVal
	if flags.Host == "" || flags.Username == "" {
		return fmt.Errorf("--host and --username are required")
	}
	password, err := resolveSyncPassword(flags)
	if err != nil {
		return err
	}
	tlsConfig, err := buildTLSConfig(flags)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := syncclient.NewClient(flags.Host, flags.Port, tlsConfig)
	if _, err := client.Login(ctx, flags.Username, password); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	local, err := syncclient.OpenLocalStore(flags.Dir)
	if err != nil {
		return err
	}
	defer local.Close()

	engine := &syncclient.Engine{
		Dir: flags.Dir, Client: client, Local: local, Username: flags.Username,
	}

	syncOut.json = flags.JSON
	syncOut.ready(flags)

	if err := reconcileWorkspace(ctx, flags, client, local, engine); err != nil {
		return err
	}

	if !flags.JSON {
		fmt.Println("\nWatching workspace...")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	trigger := make(chan struct{}, 1)

	stopFSWatch := make(chan struct{})
	go func() {
		if err := syncclient.WatchWorkspace(flags.Dir, trigger, stopFSWatch); err != nil {
			syncOut.errorf("watch", "filesystem watch error: %v", err)
		}
	}()
	defer close(stopFSWatch)

	// In --json mode the editor drives "sync now" over stdin. EOF (the
	// editor went away) stops the process rather than leaving it orphaned.
	stdinClosed := make(chan struct{})
	if flags.JSON {
		go func() {
			defer close(stdinClosed)
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				var in struct {
					Op string `json:"op"`
				}
				if json.Unmarshal(sc.Bytes(), &in) == nil && in.Op == "reconcile" {
					select {
					case trigger <- struct{}{}:
					default:
					}
				}
			}
		}()
	}

	// Live change notifications are a low-latency hint, not the source of
	// truth: if the connection drops, ListenWithReconnect retries with
	// backoff in the background while the periodic ticker below keeps
	// reconciling on a schedule regardless -- the user can keep editing
	// locally the entire time the server is unreachable. Every successful
	// (re)connect fires an immediate reconcile, which is what actually
	// pushes/pulls whatever changed while disconnected.
	var wasDisconnected bool
	onStatus := func(status syncclient.ConnStatus, err error) {
		switch status {
		case syncclient.StatusConnected:
			if wasDisconnected || flags.JSON {
				syncOut.status("connected", "Reconnected. Reconciling workspace...", nil)
				wasDisconnected = false
			}
		case syncclient.StatusDisconnected:
			if !wasDisconnected {
				syncOut.status("offline", fmt.Sprintf("Connection lost (%v). Working offline...", err), err)
				wasDisconnected = true
			}
		case syncclient.StatusReconnecting:
			syncOut.status("reconnecting", "Reconnecting...", nil)
		}
	}
	onPresence := func(ev syncclient.PresenceEvent) {
		// Our own activity isn't news, and --json consumers (editors) run
		// `rex presence` for this already -- printing every teammate's 5s
		// heartbeat here is also what used to fill an unread stdout pipe.
		if ev.Username == flags.Username || flags.JSON {
			return
		}
		verb := "is viewing"
		switch {
		case ev.Action == "idle":
			verb = "stopped editing"
		case ev.State == syncproto.PresenceEditing:
			verb = "is editing"
		}
		fmt.Printf("           %s %s %s\n", ev.Username, verb, ev.Path)
	}
	go syncclient.ListenWithReconnect(watchCtx, flags.Host, flags.Port, tlsConfig, client, trigger, onStatus, onPresence)

	// Safety-net ticker: catches anything a dropped fsnotify event or a
	// still-reconnecting websocket might have missed.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	stop := func() error {
		syncOut.status("stopped", "\nStopping sync.", nil)
		return nil
	}
	for {
		select {
		case <-sigCh:
			return stop()
		case <-stdinClosed:
			return stop()
		case <-trigger:
			if err := reconcileWorkspace(ctx, flags, client, local, engine); err != nil {
				syncOut.errorf("reconcile", "reconcile error: %v", err)
			}
		case <-ticker.C:
			if err := reconcileWorkspace(ctx, flags, client, local, engine); err != nil {
				syncOut.errorf("reconcile", "reconcile error: %v", err)
			}
		}
	}
}

// reconcileWorkspace runs one whole-workspace reconcile pass, with the
// handoff namespace narrowed to the handoffs this user is actually part of.
//
// Everything else in the workspace is shared by the whole team, but a handoff
// is addressed to two specific people: `rex sync` materializes the ones given
// to you and the ones you gave someone else, and leaves handoffs between two
// other people alone. Use `rex handoffs sync --user <name>` or `--all` to opt
// into someone else's queue.
func reconcileWorkspace(
	ctx context.Context,
	flags syncFlags,
	client *syncclient.Client,
	local *syncclient.LocalStore,
	engine *syncclient.Engine,
) error {
	// A handoff problem must never stop the rest of the workspace from
	// syncing, so a failure here degrades to "leave handoffs alone this pass"
	// rather than aborting.
	v, err := loadHandoffView(ctx, flags, client, handoffScope{})
	if err != nil {
		syncOut.errorf("handoffs", "skipping this pass: %v", err)
		engine.PathFilter = func(p string) bool { return !handoff.InNamespace(p) }
		return reconcileAndPrint(ctx, engine)
	}

	// Deleting an assigned handoff resolves it; the canonical copy stays on
	// the server. Without this the reconcile below would push a plain
	// deletion instead, losing the resolution metadata.
	if _, err := resolveVanishedHandoffs(ctx, flags, client, local, handoffScope{}, v); err != nil {
		syncOut.errorf("handoffs", "%v", err)
	}

	engine.PathFilter = func(p string) bool {
		if !handoff.InNamespace(p) {
			return true
		}
		return v.inScope(p) && !v.resolved[p]
	}
	if err := reconcileAndPrint(ctx, engine); err != nil {
		return err
	}
	// Retire local copies of handoffs that were resolved elsewhere. Quiet
	// here: reconcileAndPrint already owns this command's event log.
	if _, err := retireResolvedHandoffs(flags, local, v, false); err != nil {
		return err
	}
	if v.held > 0 {
		reportInvalid(v.invalid)
		fmt.Fprintf(os.Stderr, "%d handoff(s) have inconsistent metadata and were not synchronized\n", v.held)
	}
	return nil
}

func reconcileAndPrint(ctx context.Context, engine *syncclient.Engine) error {
	events, err := engine.Reconcile(ctx)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.Kind == "" {
			continue
		}
		syncOut.event(ev)
	}
	return nil
}
