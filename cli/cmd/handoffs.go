package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// handoffScope selects which handoffs a command operates on.
//
// The default is deliberately not "the handoffs/<me>/ directory": a handoff
// concerns two people, and you need the ones you *sent* materialized just as
// much as the ones you were given -- otherwise you couldn't follow up on your
// own request. Handoffs between two other people never appear.
type handoffScope struct {
	User string // one operator's queue, by assignee
	All  bool   // every handoff, overriding User
}

// resolve returns the assignee this scope filters on, or "" for no filter.
// The default scope has no single assignee (it spans both directions), so it
// resolves to the current user only for the queue-shaped questions -- use
// allows for the actual membership test.
func (s handoffScope) resolve(self string) string {
	if s.All {
		return ""
	}
	if s.User != "" {
		return s.User
	}
	return self
}

// allows reports whether a handoff belongs to this scope.
func (s handoffScope) allows(m handoff.Meta, self string) bool {
	switch {
	case s.All:
		return true
	case s.User != "":
		return m.To == s.User
	default:
		// Both directions: given to me, or given by me.
		return m.To == self || m.From == self
	}
}

// canResolve reports whether deleting a local handoff file in this scope
// should resolve it. Only the queue you subscribed to counts: dropping your
// own copy of a handoff you sent someone else must not finish their work.
func (s handoffScope) canResolve(m handoff.Meta, self string) bool {
	if s.All {
		return true
	}
	return m.To == s.resolve(self)
}

// wsPrefix is the server-side WebSocket subscription for this scope. A
// single-queue scope narrows to that directory; the default scope spans both
// directions, so it subscribes to the namespace and lets allows do the
// filtering -- events are metadata-only and just trigger an idempotent pass.
func (s handoffScope) wsPrefix(self string) string {
	if !s.All && s.User != "" {
		return handoff.ScopePrefix(s.User)
	}
	return handoff.ScopePrefix("")
}

// describe names the scope in prose, e.g. "your handoffs".
func (s handoffScope) describe(self string) string {
	switch {
	case s.All:
		return "every user's handoffs"
	case s.User != "" && s.User != self:
		return s.User + "'s handoffs"
	default:
		return "your handoffs"
	}
}

// queue names the scope as a place, so it reads correctly after
// "No handoffs ...".
func (s handoffScope) queue(self string) string {
	switch {
	case s.All:
		return "in any queue"
	case s.User != "" && s.User != self:
		return "in " + s.User + "'s queue"
	default:
		return "involving you"
	}
}

var (
	handoffsFlagsVal    syncFlags
	handoffsScope       handoffScope
	handoffsShowResolve bool
	handoffsWatch       bool
)

var handoffsCmd = &cobra.Command{
	Use:   "handoffs",
	Short: "Manage and synchronize work handoffs with a rex-svr",
	Long: `Manage work handoffs.

A handoff is a markdown file at handoffs/<assignee>/<title>.md whose
frontmatter records who it came from, who it's for, and whether it's still
open. The canonical copy lives on rex-svr, which versions every create,
update and resolve in git.

The directory a handoff lives in is part of its identity: handoffs/<user>/
must match the ` + "`to`" + ` metadata. Rex never guesses between the two.`,
}

var handoffsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List handoffs on the server (defaults to your own)",
	RunE:  runHandoffsList,
}

var handoffsViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "Show one handoff without synchronizing it locally",
	Args:  cobra.ExactArgs(1),
	RunE:  runHandoffsView,
}

var handoffsRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Resolve a handoff (the server keeps the canonical copy)",
	Args:  cobra.ExactArgs(1),
	RunE:  runHandoffsRemove,
}

var handoffsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize handoffs between rex-svr and the local workspace",
	RunE:  runHandoffsSync,
}

func init() {
	// The connection flags are shared by every subcommand, so they live on
	// the parent as persistent flags rather than being repeated four times.
	f := handoffsCmd.PersistentFlags()
	f.StringVar(&handoffsFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&handoffsFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&handoffsFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&handoffsFlagsVal.Password, "password", "", "shared server password (prompted interactively if omitted)")
	f.StringVar(&handoffsFlagsVal.Dir, "dir", ".", "workspace directory")
	f.StringVar(&handoffsFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&handoffsFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.StringVar(&handoffsScope.User, "user", "", "operate on another user's handoff queue")
	f.BoolVar(&handoffsScope.All, "all", false, "operate on every user's handoffs")

	handoffsListCmd.Flags().BoolVar(&handoffsShowResolve, "resolved", false, "include resolved handoffs")
	handoffsSyncCmd.Flags().BoolVar(&handoffsWatch, "watch", false, "keep running and synchronize changes as they happen")

	// A rejected handoff or an unreachable server is a runtime failure, not
	// a usage mistake -- printing the whole flag reference on top of the
	// validation message just buries it.
	for _, c := range []*cobra.Command{handoffsListCmd, handoffsViewCmd, handoffsRemoveCmd, handoffsSyncCmd} {
		c.SilenceUsage = true
	}

	handoffsCmd.AddCommand(handoffsListCmd, handoffsViewCmd, handoffsRemoveCmd, handoffsSyncCmd)
	rootCmd.AddCommand(handoffsCmd)
}

// reportInvalid prints every path/metadata disagreement the server knows
// about. These are never repaired automatically: an ambiguous assignee could
// deliver a handoff to the wrong person, so the operator has to decide.
func reportInvalid(invalid []syncproto.HandoffInvalid) {
	for _, iv := range invalid {
		fmt.Fprintf(os.Stderr, "\nerror: %s\n", iv.Error)
	}
}

func runHandoffsList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, handoffsFlagsVal)
	if err != nil {
		return err
	}

	entries, invalid, err := client.ListHandoffs(ctx, handoffsScope.User, handoffsScope.All, handoffsShowResolve)
	if err != nil {
		return fmt.Errorf("list handoffs: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	if len(entries) == 0 {
		fmt.Printf("No handoffs %s.\n", handoffsScope.queue(handoffsFlagsVal.Username))
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tFROM\tTO\tSTATUS\tTITLE")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.ID, e.From, e.To, e.Status, e.Title)
		}
		w.Flush()
	}

	if len(invalid) > 0 {
		reportInvalid(invalid)
		return fmt.Errorf("%d handoff(s) have inconsistent metadata and will not be synchronized", len(invalid))
	}
	return nil
}

func runHandoffsView(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, handoffsFlagsVal)
	if err != nil {
		return err
	}

	d, err := client.GetHandoff(ctx, args[0])
	if err != nil {
		return fmt.Errorf("view handoff %s: %w", args[0], err)
	}

	fmt.Printf("ID:      %s\nFrom:    %s\nTo:      %s\nStatus:  %s\n", d.ID, d.From, d.To, d.Status)
	if d.CreatedAt != "" {
		fmt.Printf("Created: %s\n", d.CreatedAt)
	}
	if d.ResolvedBy != "" {
		fmt.Printf("Resolved: %s by %s\n", d.ResolvedAt, d.ResolvedBy)
	}
	fmt.Printf("Path:    %s\n\n", d.Path)

	// Print the body only -- the frontmatter is already rendered above.
	if h, err := handoff.Parse(d.Path, d.Content); err == nil {
		fmt.Println(h.Body)
		return nil
	}
	fmt.Println(strings.TrimSpace(string(d.Content)))
	return nil
}

func runHandoffsRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, handoffsFlagsVal)
	if err != nil {
		return err
	}

	rr, err := client.ResolveHandoff(ctx, args[0])
	if err != nil {
		return fmt.Errorf("resolve handoff %s: %w", args[0], err)
	}
	fmt.Printf("Resolved %s [%s]\n", rr.Path, rr.ID)

	// Drop the local copy too, so the workspace matches the server without
	// waiting for the next sync pass. The canonical copy stays on rex-svr.
	local, err := syncclient.OpenLocalStore(handoffsFlagsVal.Dir)
	if err != nil {
		return err
	}
	defer local.Close()
	if err := syncclient.RemoveFile(handoffsFlagsVal.Dir, rr.Path); err != nil {
		return fmt.Errorf("remove local copy: %w", err)
	}
	_ = local.Forget(rr.Path)
	fmt.Println("The server keeps the canonical copy; `rex handoffs list --resolved` still shows it.")
	return nil
}

func runHandoffsSync(cmd *cobra.Command, args []string) error {
	flags := handoffsFlagsVal
	ctx := context.Background()
	client, err := dialSyncClient(ctx, flags)
	if err != nil {
		return err
	}
	tlsConfig, err := buildTLSConfig(flags)
	if err != nil {
		return err
	}

	local, err := syncclient.OpenLocalStore(flags.Dir)
	if err != nil {
		return err
	}
	defer local.Close()

	engine := &syncclient.Engine{
		Dir: flags.Dir, Client: client, Local: local, Username: flags.Username,
	}

	fmt.Printf("Synchronizing %s...\n\n", handoffsScope.describe(flags.Username))
	if err := handoffSyncPass(ctx, flags, client, local, engine, handoffsScope); err != nil {
		return err
	}

	if !handoffsWatch {
		return nil
	}

	fmt.Println("\nWatching for handoff changes...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	// Same shape as `rex sync`: a filesystem watcher, live server
	// notifications, and a periodic tick all feed one trigger channel, and
	// every trigger runs the same idempotent pass. The 400ms debounce inside
	// WatchWorkspace is also the first half of the atomic-save guard -- see
	// resolveVanishedHandoffs.
	trigger := make(chan struct{}, 1)

	stopFSWatch := make(chan struct{})
	go func() {
		if err := syncclient.WatchWorkspace(flags.Dir, trigger, stopFSWatch); err != nil {
			fmt.Fprintf(os.Stderr, "filesystem watch error: %v\n", err)
		}
	}()
	defer close(stopFSWatch)

	// The server only sends events for this subscription's prefix, so a
	// client watching one queue isn't woken by the rest of the workspace.
	go syncclient.ListenWithReconnectScoped(watchCtx, flags.Host, flags.Port, tlsConfig, client,
		handoffsScope.wsPrefix(flags.Username), trigger, nil, nil)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopping handoff sync.")
			return nil
		case <-trigger:
		case <-ticker.C:
		}
		if err := handoffSyncPass(ctx, flags, client, local, engine, handoffsScope); err != nil {
			fmt.Fprintf(os.Stderr, "handoff sync error: %v\n", err)
		}
	}
}

// handoffView is one snapshot of the handoff namespace: what's on disk, what
// the server holds, and which paths this scope is allowed to materialize.
// Both `rex handoffs sync` and `rex sync` reconcile against one of these.
type handoffView struct {
	// inScope reports whether a handoff path may be materialized here.
	inScope func(path string) bool
	// openIDs maps the path of every still-open handoff to its stable id.
	openIDs map[string]string
	// resolved holds the paths the server has retired. They are excluded
	// from reconciliation (rather than left to reconcile as a remote delete)
	// so that editing a handoff someone else just resolved retires the local
	// copy instead of raising a conflict on a tombstoned path.
	resolved map[string]bool
	// metas is the post-stamp metadata of every handoff file on disk.
	metas map[string]handoff.Meta
	// held counts handoffs excluded from this pass for inconsistent
	// metadata, local and server-side.
	held    int
	invalid []syncproto.HandoffInvalid
}

// loadHandoffView validates and stamps the local handoff files, asks the
// server what it holds, and works out which paths belong to scope.
func loadHandoffView(
	ctx context.Context,
	flags syncFlags,
	client *syncclient.Client,
	scope handoffScope,
) (*handoffView, error) {
	// Local first: stamping is what turns a hand-written `to:`-only file into
	// a handoff with a creator and an id, and the creator has to be known
	// before we can tell whether it's in scope.
	metas, bad, err := prepareLocalHandoffs(flags)
	if err != nil {
		return nil, err
	}

	// Ask for every queue and filter locally. Scope decides what a client
	// *materializes*; it can't decide what the client is allowed to know
	// about, because a handoff you created lives in someone else's directory
	// and you still need to track it.
	entries, invalid, err := client.ListHandoffs(ctx, "", true, true)
	if err != nil {
		return nil, fmt.Errorf("list handoffs: %w", err)
	}

	remote := map[string]handoff.Meta{}
	openIDs := map[string]string{}
	resolved := map[string]bool{}
	for _, e := range entries {
		remote[e.Path] = handoff.Meta{ID: e.ID, From: e.From, To: e.To, Status: e.Status}
		if e.Status == handoff.StatusResolved {
			resolved[e.Path] = true
			continue
		}
		openIDs[e.Path] = e.ID
	}

	v := &handoffView{
		openIDs: openIDs, resolved: resolved, metas: metas,
		held: len(bad) + len(invalid), invalid: invalid,
	}
	v.inScope = func(p string) bool {
		if !handoff.IsPath(p) || bad[p] {
			return false
		}
		// Prefer the server's metadata: it is authoritative about who
		// created a handoff. Fall back to the local file for one that hasn't
		// been pushed yet.
		if m, ok := remote[p]; ok {
			return scope.allows(m, flags.Username)
		}
		if m, ok := metas[p]; ok {
			return scope.allows(m, flags.Username)
		}
		return false
	}
	return v, nil
}

// meta returns the best-known metadata for a handoff path.
func (v *handoffView) meta(path string) handoff.Meta {
	if m, ok := v.metas[path]; ok {
		return m
	}
	return handoff.Meta{To: handoff.AssigneeFromPath(path)}
}

// handoffSyncPass performs one full reconciliation of the handoff scope. It
// is idempotent, so it is safe to run on every filesystem event, server
// notification and timer tick.
func handoffSyncPass(
	ctx context.Context,
	flags syncFlags,
	client *syncclient.Client,
	local *syncclient.LocalStore,
	engine *syncclient.Engine,
	scope handoffScope,
) error {
	v, err := loadHandoffView(ctx, flags, client, scope)
	if err != nil {
		return err
	}

	// A handoff the assignee deleted locally is a resolve, not a push of a
	// deletion -- the server keeps the canonical copy.
	synced, err := resolveVanishedHandoffs(ctx, flags, client, local, scope, v)
	if err != nil {
		return err
	}

	// Reconcile the scope. Excluding resolved paths is what stops a finished
	// handoff from being resurrected just because a canonical copy still
	// exists server-side.
	engine.PathFilter = func(p string) bool { return v.inScope(p) && !v.resolved[p] }
	events, err := engine.Reconcile(ctx)
	if err != nil {
		return err
	}
	for _, ev := range events {
		switch ev.Kind {
		case syncclient.EventPushed, syncclient.EventPulled, syncclient.EventUpdated:
			fmt.Printf("+ %s\n", ev.Path)
			synced++
		case syncclient.EventDeleted:
			fmt.Printf("- %s\n", ev.Path)
			synced++
		case syncclient.EventConflict:
			fmt.Printf("! %s (%s)\n", ev.Path, ev.Detail)
			fmt.Println("  run `rex conflicts` to view, `rex resolve` to fix")
		case syncclient.EventSkipped:
			fmt.Fprintf(os.Stderr, "  skipped %s: %s\n", ev.Path, ev.Detail)
		}
	}

	// Retire local copies of anything that finished elsewhere.
	retired, err := retireResolvedHandoffs(flags, local, v, true)
	if err != nil {
		return err
	}
	synced += retired

	switch {
	case synced > 0:
		fmt.Printf("\n%d handoff(s) synchronized\n", synced)
	case v.held > 0:
		// The error below says what happened; don't also claim all is well.
	case !handoffsWatch:
		// In watch mode this fires on every tick, which is just noise.
		fmt.Println("Up to date.")
	}

	if v.held > 0 {
		reportInvalid(v.invalid)
		return fmt.Errorf("%d handoff(s) have inconsistent metadata and were not synchronized", v.held)
	}
	return nil
}

// retireResolvedHandoffs deletes the local copy of every in-scope handoff the
// server has resolved, so a finished handoff leaves the workspace. The
// canonical copy stays on rex-svr.
func retireResolvedHandoffs(flags syncFlags, local *syncclient.LocalStore, v *handoffView, verbose bool) (int, error) {
	paths := make([]string, 0, len(v.resolved))
	for p := range v.resolved {
		if v.inScope(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	var retired int
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(flags.Dir, filepath.FromSlash(p))); err != nil {
			_ = local.Forget(p)
			continue
		}
		if err := syncclient.RemoveFile(flags.Dir, p); err != nil {
			return retired, fmt.Errorf("remove resolved handoff %s: %w", p, err)
		}
		_ = local.Forget(p)
		retired++
		if verbose {
			fmt.Printf("- %s (resolved)\n", p)
		}
	}
	return retired, nil
}

// prepareLocalHandoffs validates every handoff file in the workspace and
// stamps the identity metadata a hand-written one omits, so a user can create
// a handoff by writing nothing but `to:` and a body. It returns the
// post-stamp metadata per path, plus the paths that failed validation and
// must be excluded from synchronization.
//
// It walks only the handoffs/ subtree rather than reusing ScanWorkspace,
// which reads and hashes every file in the workspace -- this runs on every
// debounced filesystem event, so it must not re-hash a directory of
// screenshots to find a handful of markdown files.
func prepareLocalHandoffs(flags syncFlags) (metas map[string]handoff.Meta, bad map[string]bool, err error) {
	metas, bad = map[string]handoff.Meta{}, map[string]bool{}

	paths, err := walkHandoffDir(flags.Dir)
	if err != nil {
		return nil, nil, err
	}

	for _, p := range paths {
		if !handoff.IsPath(p) {
			// Something under handoffs/ that can't name an assignee. The
			// server would refuse it, so surface it here rather than letting
			// every pass fail with an opaque 422.
			bad[p] = true
			fmt.Fprintf(os.Stderr, "\nerror: %s is not a valid handoff path\n\nHandoffs must be %s/<assignee>/<title>.md\n", p, handoff.Dir)
			continue
		}
		content, err := syncclient.ReadFile(flags.Dir, p)
		if err != nil {
			return nil, nil, err
		}
		stamped, changed, err := handoff.Stamp(content, flags.Username)
		if err != nil {
			bad[p] = true
			fmt.Fprintf(os.Stderr, "\nerror: cannot read handoff metadata in %s: %v\n", p, err)
			continue
		}
		h, err := handoff.Parse(p, stamped)
		if err != nil {
			bad[p] = true
			fmt.Fprintf(os.Stderr, "\nerror: %s\n", err)
			continue
		}
		if changed {
			if err := syncclient.WriteFile(flags.Dir, p, stamped); err != nil {
				return nil, nil, fmt.Errorf("stamp handoff %s: %w", p, err)
			}
		}
		metas[p] = h.Meta
	}
	return metas, bad, nil
}

// walkHandoffDir lists every markdown file under the workspace's handoffs/
// directory, as normalized workspace-relative paths.
func walkHandoffDir(dir string) ([]string, error) {
	root := filepath.Join(dir, handoff.Dir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable entry must not block the rest of the sync.
			fmt.Fprintf(os.Stderr, "handoffs: skipping %s: %v\n", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		slash := filepath.ToSlash(rel)
		if synccommon.IsExcluded(slash) || !strings.HasSuffix(slash, ".md") {
			return nil
		}
		paths = append(paths, slash)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan handoffs: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// resolveVanishedHandoffs turns "the assignee deleted the file" into a
// resolve operation. The server must keep the canonical copy, so this never
// pushes a deletion.
//
// ponytail: absence is confirmed by the watcher's 400ms debounce plus a
// fresh stat here, which is enough to ride out an editor's
// write-temp/unlink/rename save cycle. Correlating inotify rename pairs
// would be stricter; do that only if a real editor slips through.
func resolveVanishedHandoffs(
	ctx context.Context,
	flags syncFlags,
	client *syncclient.Client,
	local *syncclient.LocalStore,
	scope handoffScope,
	v *handoffView,
) (int, error) {
	tracked, err := local.All()
	if err != nil {
		return 0, err
	}
	paths := make([]string, 0, len(tracked))
	for p := range tracked {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var resolved int
	for _, p := range paths {
		if !handoff.IsPath(p) {
			continue
		}
		if _, err := os.Stat(filepath.Join(flags.Dir, filepath.FromSlash(p))); err == nil {
			continue // still there: an atomic save, not a deletion
		}
		id, isOpen := v.openIDs[p]
		if !isOpen {
			// Already resolved (or never reached the server): nothing to do
			// beyond dropping the stale tracking row.
			_ = local.Forget(p)
			continue
		}
		if !scope.canResolve(v.meta(p), flags.Username) {
			// Dropping your own copy of a handoff you sent someone else must
			// not finish their work for them.
			_ = local.Forget(p)
			continue
		}
		if _, err := client.ResolveHandoff(ctx, id); err != nil {
			return resolved, fmt.Errorf("resolve handoff %s: %w", id, err)
		}
		_ = local.Forget(p)
		delete(v.openIDs, p)
		v.resolved[p] = true
		resolved++
		fmt.Printf("resolved %s [%s] (deleted locally)\n", p, id)
	}
	return resolved, nil
}
