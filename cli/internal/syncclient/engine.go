package syncclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// EventKind categorizes one line of the client's live event log.
type EventKind string

const (
	EventPushed   EventKind = "pushed"
	EventPulled   EventKind = "pulled"
	EventUpdated  EventKind = "updated"
	EventDeleted  EventKind = "deleted"
	EventConflict EventKind = "conflict"
	EventMerged   EventKind = "merged"
	EventSkipped  EventKind = "skipped"
)

// Event is one reconciliation outcome, reported to the CLI's event log.
// ConflictID/ConflictKind are set for conflict and merged events.
type Event struct {
	Kind         EventKind
	Path         string
	Detail       string
	ConflictID   int64
	ConflictKind string
}

// A reconcile pass refuses to push local deletes when they'd remove at least
// massDeleteMinCount files and more than massDeleteMaxFraction of the live
// synced files (mirrors syncserver's drift-repair guard).
const (
	massDeleteMinCount    = 5
	massDeleteMaxFraction = 0.9
)

// Engine drives reconciliation between the local workspace and the sync
// server: initial sync (Phase 3) and reconnection reconciliation (Phase 8)
// both use this same algorithm, since both reduce to "diff local, last-
// synced, and remote state, then push/pull/conflict per path."
type Engine struct {
	Dir      string
	Client   *Client
	Local    *LocalStore
	Username string

	// PathFilter optionally restricts this engine to a subset of the
	// workspace. Paths it rejects are left entirely alone: not pushed, not
	// pulled, not deleted. `rex sync` leaves it nil to reconcile everything;
	// `rex handoffs sync` sets it so a client materializes only the handoff
	// scope it subscribed to -- and so a resolved handoff is never a
	// candidate path again, which is what stops resurrection.
	PathFilter func(path string) bool

	// SelfPushed optionally records (path -> revision/hash) this engine
	// itself just pushed, so a caller with a live filesystem watcher can
	// recognize and suppress the resulting self-echo (wired in Phase 4).
	SelfPushed func(path string, revision int64, hash string)

	// lastConflictHash remembers, per path, the local content hash that
	// most recently produced a server-side conflict. Reconcile runs
	// repeatedly (on every fs event, WS notification, and the fallback
	// ticker) for the lifetime of a long-running `sync` process; without
	// this, an unresolved conflict would be re-flagged -- and a brand new
	// server-side conflict record created -- on every single pass, since
	// nothing about local/remote changes until the user runs `resolve`.
	// The cache self-heals: once local content actually changes (directly,
	// or indirectly via `resolve` overwriting the file), the hash no
	// longer matches and the path is retried normally.
	lastConflictHash map[string]string
}

// Reconcile performs one full pass: scan local files, fetch the remote
// manifest, classify every known path against last-synced state, and
// push/pull/flag-conflict accordingly. It returns the events applied, in
// the order they were processed, plus a list of paths that ended up in
// conflict (never silently resolved).
func (e *Engine) Reconcile(ctx context.Context) ([]Event, error) {
	localFiles, err := ScanWorkspace(e.Dir)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	remoteFiles, err := e.Client.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	remoteByPath := map[string]struct {
		revision int64
		hash     string
		deleted  bool
	}{}
	for _, f := range remoteFiles {
		remoteByPath[f.Path] = struct {
			revision int64
			hash     string
			deleted  bool
		}{f.Revision, f.SHA256, f.Deleted}
	}
	syncState, err := e.Local.All()
	if err != nil {
		return nil, fmt.Errorf("load local sync state: %w", err)
	}

	paths := map[string]bool{}
	for p := range localFiles {
		paths[p] = true
	}
	for p := range remoteByPath {
		paths[p] = true
	}
	for p := range syncState {
		paths[p] = true
	}

	inScope := func(path string) bool {
		// A path synced before it joined the exclude list (e.g. an old
		// .vscode/settings.json) still sits in the server manifest; the
		// server now refuses to serve it, so leave it alone locally.
		return (e.PathFilter == nil || e.PathFilter(path)) && !synccommon.IsExcluded(path)
	}

	// Mass-delete guard (issue #46): a local delete of nearly every synced
	// file is far likelier a moved/emptied workspace than intent, and
	// pushing it wipes the report for everyone. Refuse those deletes; a
	// real mass delete goes through `rex delete` with the admin password.
	var live, deletes int
	for p, rf := range remoteByPath {
		if rf.deleted || !inScope(p) {
			continue
		}
		live++
		if _, ok := localFiles[p]; !ok && syncState[p].LastSyncedLocalHash != "" {
			deletes++
		}
	}
	blockDeletes := deletes >= massDeleteMinCount && float64(deletes)/float64(live) > massDeleteMaxFraction

	var events []Event
	if blockDeletes {
		events = append(events, Event{Kind: EventSkipped, Detail: fmt.Sprintf(
			"refusing to delete %d/%d synced files (%.0f%%) -- if the workspace was moved, move it back; "+
				"to really delete them, use \"Report Rex: Delete for Everyone\" or `rex delete` with the admin password",
			deletes, live, 100*float64(deletes)/float64(live))})
	}
	for path := range paths {
		if !inScope(path) {
			continue
		}
		if _, ok := localFiles[path]; blockDeletes && !ok && syncState[path].LastSyncedLocalHash != "" {
			continue
		}
		ev, applyErr := e.reconcileOne(ctx, path, localFiles, remoteByPath, syncState)
		if applyErr != nil {
			events = append(events, Event{Kind: EventSkipped, Path: path, Detail: applyErr.Error()})
			continue
		}
		if ev != nil {
			events = append(events, *ev)
		}
	}
	return events, nil
}

func (e *Engine) reconcileOne(
	ctx context.Context,
	path string,
	localFiles map[string]LocalFile,
	remoteByPath map[string]struct {
		revision int64
		hash     string
		deleted  bool
	},
	syncState map[string]SyncStateRow,
) (*Event, error) {
	lf, hasLocal := localFiles[path]
	rf, hasRemote := remoteByPath[path]
	remoteExists := hasRemote && !rf.deleted
	st, hasLastSync := syncState[path]

	// Every real delete leaves a tombstone, so a path the server has no
	// record of at all means the server lost its state (a fresh or different
	// --db), not that someone deleted it. Treat it as never synced: local
	// content gets pushed back instead of wiped (issue #62), and a stale base
	// can't turn the path's next appearance into a false conflict.
	if !hasRemote && hasLastSync {
		hasLastSync = false
		if !hasLocal {
			_ = e.Local.Forget(path)
		}
	}

	local := synccommon.PathSnapshot{Exists: hasLocal, Hash: lf.Hash}
	remote := synccommon.PathSnapshot{Exists: remoteExists, Hash: rf.hash}
	var lastSynced synccommon.PathSnapshot
	if hasLastSync {
		lastSynced = synccommon.PathSnapshot{
			Exists: st.LastSyncedLocalHash != "",
			Hash:   st.LastSyncedLocalHash,
		}
	}

	state := synccommon.Classify(hasLastSync, lastSynced, local, remote)

	switch state {
	case synccommon.StateUnchanged:
		// Make sure sync_state reflects reality even if nothing moved:
		// the first time we see an already-identical path, and whenever
		// local and remote converged on their own (a `rex resolve`, or two
		// people making the same edit) -- otherwise the stale base makes
		// this client's next edit a false conflict.
		if hasLocal && remoteExists && (!hasLastSync || st.LastKnownRemoteRevision == nil ||
			*st.LastKnownRemoteRevision != rf.revision || st.LastSyncedLocalHash != lf.Hash) {
			_ = e.Local.Upsert(path, rf.revision, rf.hash, lf.Hash, now())
		}
		return nil, nil

	case synccommon.StateRemoteOnlyChanged:
		// The scan is a snapshot; if the file changed since, a pull would
		// silently overwrite (or delete) that newer edit. Skip this path --
		// the change already triggered another pass, which will see it.
		if !localUnchanged(e.Dir, path, lf, hasLocal) {
			return nil, nil
		}
		return e.pull(ctx, path, rf, remoteExists)

	case synccommon.StateLocalOnlyChanged, synccommon.StateBothChanged, synccommon.StateInitialUnknownBase:
		// If this exact local content already produced a conflict on a
		// previous pass, don't retry -- nothing has changed since, so the
		// server would just reject it identically and create a duplicate
		// conflict record. Wait for local content to change or the user
		// to run `resolve`. Reconcile runs repeatedly for the lifetime of
		// a long-running `sync` process (every fs event, WS notification,
		// and fallback tick), so without this an unresolved conflict
		// would spawn a fresh server-side conflict record every pass.
		// (Check presence explicitly: a deleted file's hash is "", which is
		// also what a missing map key returns -- comparing values alone
		// silently suppressed every local delete once anything had been
		// pushed.)
		if prev, ok := e.lastConflictHash[path]; ok && prev == local.Hash {
			return nil, nil
		}
		// Attempt the push/delete. For Both-changed/initial-unknown-base,
		// the server holds the authoritative revision counter and will
		// reject this as a 409 conflict, recording it -- we never guess a
		// winner client-side.
		var base *int64
		if hasLastSync {
			base = st.LastKnownRemoteRevision
		}
		ev, err := e.push(ctx, path, lf, hasLocal, base)
		if err != nil {
			return ev, err
		}
		if e.lastConflictHash == nil {
			e.lastConflictHash = map[string]string{}
		}
		if ev != nil && ev.Kind == EventConflict {
			e.lastConflictHash[path] = local.Hash
		} else {
			delete(e.lastConflictHash, path)
		}
		return ev, nil
	}
	return nil, nil
}

func (e *Engine) pull(ctx context.Context, path string, rf struct {
	revision int64
	hash     string
	deleted  bool
}, remoteExists bool) (*Event, error) {
	if !remoteExists {
		if err := RemoveFile(e.Dir, path); err != nil {
			return nil, err
		}
		_ = e.Local.Forget(path)
		return &Event{Kind: EventDeleted, Path: path, Detail: "removed locally (deleted on server)"}, nil
	}

	content, revision, sha, ok, err := e.Client.GetFile(ctx, path)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Raced with a delete between manifest fetch and content fetch;
		// treat as deleted.
		if err := RemoveFile(e.Dir, path); err != nil {
			return nil, err
		}
		_ = e.Local.Forget(path)
		return &Event{Kind: EventDeleted, Path: path}, nil
	}
	if err := WriteFile(e.Dir, path, content); err != nil {
		return nil, err
	}
	if err := e.Local.Upsert(path, revision, sha, synccommon.HashBytes(content), now()); err != nil {
		return nil, err
	}
	_ = rf
	return &Event{Kind: EventPulled, Path: path, Detail: fmt.Sprintf("rev %d", revision)}, nil
}

func (e *Engine) push(ctx context.Context, path string, lf LocalFile, hasLocal bool, base *int64) (*Event, error) {
	if !hasLocal {
		if base == nil {
			// Nothing local, no known base, nothing to delete remotely
			// (this happens if the path was only ever a tombstone).
			return nil, nil
		}
		revision, err := e.Client.DeleteFile(ctx, path, *base)
		if err != nil {
			var ce *ErrConflict
			if errors.As(err, &ce) {
				return conflictEvent(path, ce), nil
			}
			return nil, err
		}
		if err := e.Local.Upsert(path, revision, "", "", now()); err != nil {
			return nil, err
		}
		return &Event{Kind: EventPushed, Path: path, Detail: "deleted"}, nil
	}

	content, err := ReadFile(e.Dir, path)
	if err != nil {
		return nil, err
	}
	revision, sha, err := e.Client.PutFile(ctx, path, content, base)
	if err != nil {
		var ce *ErrConflict
		if errors.As(err, &ce) {
			if ev := e.autoMerge(ctx, path, ce, content); ev != nil {
				return ev, nil
			}
			return conflictEvent(path, ce), nil
		}
		return nil, err
	}
	if e.SelfPushed != nil {
		e.SelfPushed(path, revision, sha)
	}
	// Record the hash of what was actually uploaded, not the scan-time
	// hash: the file may have changed in between, and a stale
	// last-synced hash turns the next remote-only change into a false
	// both-changed conflict.
	if err := e.Local.Upsert(path, revision, sha, synccommon.HashBytes(content), now()); err != nil {
		return nil, err
	}
	return &Event{Kind: EventPushed, Path: path, Detail: fmt.Sprintf("rev %d", revision)}, nil
}

func conflictEvent(path string, ce *ErrConflict) *Event {
	return &Event{Kind: EventConflict, Path: path, Detail: ce.Error(),
		ConflictID: ce.Detail.ConflictID, ConflictKind: string(ce.Detail.Kind)}
}

// autoMergeAttempts bounds how often autoMerge re-fetches and re-merges when
// the file keeps moving on the server underneath it.
const autoMergeAttempts = 3

// autoMerge tries to settle a just-recorded upload conflict without the
// user: if a line-based three-way merge of base/local/remote has no
// overlapping hunks, it's submitted as the conflict's resolution. It
// returns nil -- leaving the conflict open for a human -- whenever it can't
// merge safely: no known base, a delete/edit conflict, binary content, the
// handoff namespace (whose metadata the server validates on upload), or
// genuinely overlapping edits. A file still carrying `rex resolve --merge`
// markers is mid hand-merge and is left alone too.
func (e *Engine) autoMerge(ctx context.Context, path string, ce *ErrConflict, local []byte) *Event {
	if ce.Detail.Kind == syncproto.ConflictKindDeleteEdit || handoff.InNamespace(path) ||
		bytes.IndexByte(local, 0) >= 0 || HasConflictMarkers(local) {
		return nil
	}
	id := ce.Detail.ConflictID
	for attempt := 0; attempt < autoMergeAttempts; attempt++ {
		detail, err := e.Client.GetConflict(ctx, id, false, true, true)
		if err != nil || !detail.HasBase || detail.RemoteContentRevision == 0 || bytes.IndexByte(detail.RemoteContent, 0) >= 0 {
			return nil
		}
		merged, clean := ThreeWayMerge(detail.BaseContent, local, detail.RemoteContent)
		if !clean {
			return nil
		}
		rev := detail.RemoteContentRevision
		resp, err := e.Client.ResolveConflict(ctx, id, syncproto.ResolveRequest{
			Resolution: syncproto.ResolutionMerge, MergedContent: merged, ExpectedRemoteRevision: &rev,
		})
		if err != nil || resp.NewRevision == nil {
			continue // most likely stale_remote: someone pushed meanwhile; re-merge on top
		}
		// Write the merge back only if nobody touched the file while we
		// were merging. Otherwise leave sync_state alone: the next pass
		// sees both sides changed, conflicts again, and merges the newer
		// local content against the same base -- nothing is lost.
		if cur, err := ReadFile(e.Dir, path); err == nil && bytes.Equal(cur, local) {
			if err := WriteFile(e.Dir, path, merged); err == nil {
				h := synccommon.HashBytes(merged)
				_ = e.Local.Upsert(path, *resp.NewRevision, h, h, now())
			}
		}
		return &Event{Kind: EventMerged, Path: path, ConflictID: id, ConflictKind: string(ce.Detail.Kind),
			Detail: fmt.Sprintf("rev %d, auto-merged with remote changes", *resp.NewRevision)}
	}
	return nil
}

// localUnchanged reports whether path on disk still matches the scan
// snapshot lf/hasLocal.
func localUnchanged(dir, path string, lf LocalFile, hasLocal bool) bool {
	content, err := ReadFile(dir, path)
	if err != nil {
		return !hasLocal && os.IsNotExist(err)
	}
	return hasLocal && synccommon.HashBytes(content) == lf.Hash
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
