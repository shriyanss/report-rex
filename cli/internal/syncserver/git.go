package syncserver

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/gitexec"
	"github.com/shriyanss/report-rex/cli/internal/handoff"
	"github.com/shriyanss/report-rex/cli/internal/synccommon"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

// driftDeleteMinCount and driftDeleteMaxFraction bound how much
// DriftCheckAndRepair is willing to delete from the workspace in one pass.
// The DB manifest is normally authoritative, but if it ever comes back
// emptier than reality (an operational --db/--workspace mismatch between
// restarts, or any future regression upstream of this function) nothing
// else stops the delete loop from wiping a legitimately-synced workspace in
// a single commit. A small number of stray files is expected and safe to
// clean up (e.g. manual tampering); a majority of the tree disappearing
// from the manifest is not "drift", it's a manifest-read anomaly, so it's
// refused -- and the server doesn't start, rather than serve that emptier
// manifest to clients that would then delete their own copies.
const (
	driftDeleteMinCount    = 5
	driftDeleteMaxFraction = 0.5
)

// rexSvrEmails are the repo-local identities NewGitMaterializer has stamped
// on workspaces it initialized (the first is from before the binary rename),
// which is how it tells its own repository apart from somebody else's.
var rexSvrEmails = map[string]bool{"server@rex-svr.local": true, "server@report-rex.local": true}

// CheckWorkspaceLayout refuses a --db, --cert or --key path inside
// --workspace. rex-svr owns that directory outright: the startup drift check
// deletes anything there the DB doesn't list -- which on a first start was
// the live SQLite database itself (issue #62) -- and every commit runs
// `git add -A` over it, which would put the TLS private key in git history.
func CheckWorkspaceLayout(workspace, db, cert, key string) error {
	ws := resolvePath(workspace)
	for _, f := range []struct{ flag, path string }{{"--db", db}, {"--cert", cert}, {"--key", key}} {
		rel, err := filepath.Rel(ws, resolvePath(f.path))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s %s is inside --workspace %s; rex-svr owns that directory and would delete or "+
				"commit it -- keep --workspace a dedicated directory (default ./rex-svr-git-workspace)", f.flag, f.path, workspace)
		}
	}
	return nil
}

// resolvePath makes p absolute with symlinks resolved as far as it exists, so
// a not-yet-created file or a symlinked directory still compares correctly.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	if parent := filepath.Dir(abs); parent != abs {
		return filepath.Join(resolvePath(parent), filepath.Base(abs))
	}
	return abs
}

// massDeleteMinCount, massDeleteMaxFraction and massDeleteWindow bound how
// much of the workspace one user may delete through per-file DELETEs in a
// short window (issue #46). The sync client enforces the same ratio, but an
// old or buggy client pushing a moved-away workspace's "deletes" one by one
// must not wipe the report; deliberate mass deletes go through the
// admin-password-gated POST /api/v1/admin/delete.
const (
	massDeleteMinCount    = 5
	massDeleteMaxFraction = 0.9
	massDeleteWindow      = 10 * time.Minute
)

// GitMaterializer projects accepted canonical DB changes into a
// server-owned git working copy, coalescing them into debounced commits.
// The DB is always authoritative: GitMaterializer never causes a DB write,
// only reads from it to materialize files onto disk and into git history.
type GitMaterializer struct {
	workspace     string
	runner        gitexec.Runner
	db            *sql.DB
	store         *Store
	flushInterval time.Duration

	mu      sync.Mutex
	trigger chan struct{}
	stop    chan struct{}
	done    chan struct{}

	healthy atomic.Bool
}

// NewGitMaterializer opens or initializes the git workspace at workspace
// and returns a ready-to-Start materializer. It does not itself verify the
// git executable is present -- callers should call gitexec.Version() at
// process startup, per the issue's mandated startup order.
func NewGitMaterializer(workspace string, db *sql.DB, store *Store) (*GitMaterializer, error) {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}
	runner := gitexec.New(workspace)

	if _, err := os.Stat(filepath.Join(workspace, ".git")); os.IsNotExist(err) {
		// rex-svr owns this directory outright -- drift repair deletes
		// whatever the DB doesn't list -- so never adopt one that already
		// holds somebody's files (issue #62).
		entries, err := os.ReadDir(workspace)
		if err != nil {
			return nil, fmt.Errorf("read workspace dir: %w", err)
		}
		for _, e := range entries {
			if !synccommon.IsExcluded(e.Name()) {
				return nil, fmt.Errorf("%s is not empty and is not a rex-svr workspace; rex-svr deletes anything "+
					"there its database doesn't list, so point --workspace at a new or empty directory", workspace)
			}
		}
		if _, err := runner.Run("init"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
		// Repo-local identity -- must not depend on global git config
		// being pre-configured on the host.
		if _, err := runner.Run("config", "user.name", "rex-svr"); err != nil {
			return nil, fmt.Errorf("git config user.name: %w", err)
		}
		if _, err := runner.Run("config", "user.email", "server@rex-svr.local"); err != nil {
			return nil, fmt.Errorf("git config user.email: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("stat workspace .git: %w", err)
	} else if email, _ := runner.Run("config", "--local", "user.email"); !rexSvrEmails[strings.TrimSpace(email)] {
		return nil, fmt.Errorf("%s is a git repository rex-svr did not create; rex-svr would delete and commit over "+
			"its files, so point --workspace at a new or empty directory", workspace)
	}

	m := &GitMaterializer{
		workspace:     workspace,
		runner:        runner,
		db:            db,
		store:         store,
		flushInterval: 2 * time.Second,
		trigger:       make(chan struct{}, 1),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	m.healthy.Store(true)
	return m, nil
}

// Enqueue durably records a pending materialization and signals the
// background flush loop. Enqueue itself never touches git -- the actual
// materialization happens in the debounced Flush, so a burst of writes in
// one debounce window becomes one commit.
func (m *GitMaterializer) Enqueue(path, op string, revision int64, username string) {
	if _, err := m.db.Exec(`
		INSERT INTO pending_materializations (path, revision, op, created_at)
		VALUES (?, ?, ?, ?)`, path, revision, op, now()); err != nil {
		log.Printf("syncserver: failed to record pending materialization for %s: %v", path, err)
		m.healthy.Store(false)
		return
	}
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

func (m *GitMaterializer) Healthy() bool { return m.healthy.Load() }

func (m *GitMaterializer) PendingCount() int {
	var n int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM pending_materializations`).Scan(&n)
	return n
}

// Start runs the debounced flush loop until Stop is called.
func (m *GitMaterializer) Start() {
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(m.flushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-m.trigger:
				// Debounce: wait for the flush interval of quiet before
				// materializing, so several rapid writes land in one commit --
				// but never longer than 5x the interval, or a team that never
				// stops typing never gets a commit at all.
				timer := time.NewTimer(m.flushInterval)
				maxWait := time.NewTimer(5 * m.flushInterval)
				drained := false
				for !drained {
					select {
					case <-m.trigger:
						timer.Reset(m.flushInterval)
					case <-timer.C:
						drained = true
					case <-maxWait.C:
						timer.Stop()
						drained = true
					case <-m.stop:
						timer.Stop()
						maxWait.Stop()
						return
					}
				}
				maxWait.Stop()
				m.Flush(context.Background())
			case <-ticker.C:
				// Periodic safety-net flush in case a trigger was ever
				// dropped, and to retry after a prior git failure.
				if m.PendingCount() > 0 {
					m.Flush(context.Background())
				}
			}
		}
	}()
}

func (m *GitMaterializer) Stop() {
	close(m.stop)
	<-m.done
}

type pendingRow struct {
	id       int64
	path     string
	revision int64
	op       string
}

// pendingChange describes one materialized path for commit-message
// generation and pending-queue clearing.
type pendingChange struct {
	path, op, username string
	// revision is the canonical revision this change materialized, used to
	// tell a handoff's creation apart from an update in the commit message.
	revision int64
	// handoffID is set only for paths in the handoff namespace, so handoff
	// history stays greppable by stable id.
	handoffID string
}

// Flush materializes every currently-queued pending_materializations row
// into the git workspace as a single commit. Safe to call concurrently
// with itself (serialized via mu) and safe to retry after a prior failure
// -- rows are only removed from pending_materializations once the commit
// actually succeeds.
func (m *GitMaterializer) Flush(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flush(ctx, "")
}

// Commit flushes everything pending right now, as one commit whose subject
// line is subject instead of the generated "sync: ..." one. Returns the
// resulting HEAD sha.
func (m *GitMaterializer) Commit(ctx context.Context, subject string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flush(ctx, subject)
	sha, _ := m.runner.Run("rev-parse", "HEAD")
	return strings.TrimSpace(sha)
}

// CommitFor returns "<sha>\t<subject>" of the oldest commit that introduced
// content at path, or "" if no commit ever held exactly that content.
func (m *GitMaterializer) CommitFor(path string, content []byte) string {
	blob := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(content))), content...))
	out, err := m.runner.Run("log", "--reverse", "--format=%H%x09%s", "--find-object="+hex.EncodeToString(blob[:]), "--", path)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	return first
}

// Log returns the n most recent commits as "<short sha> <date> <subject>".
func (m *GitMaterializer) Log(n int) []string {
	out, err := m.runner.Run("log", "-n", strconv.Itoa(n), "--date=format:%Y-%m-%d %H:%M", "--format=%h %ad %s")
	if err != nil || strings.TrimSpace(out) == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSpace(out), "\n")
}

func (m *GitMaterializer) flush(ctx context.Context, subject string) {

	rows, err := m.loadPending(ctx)
	if err != nil {
		log.Printf("syncserver: failed to load pending materializations: %v", err)
		m.healthy.Store(false)
		return
	}
	if len(rows) == 0 {
		return
	}

	// Dedup: keep only the latest row per path (a path may have been
	// written multiple times within one debounce window/outage).
	latest := map[string]pendingRow{}
	for _, r := range rows {
		if cur, ok := latest[r.path]; !ok || r.revision >= cur.revision {
			latest[r.path] = r
		}
	}

	var changes []pendingChange
	var appliedIDs []int64
	anyStaged := false

	for _, r := range latest {
		username := "unknown"
		if rec, err := m.store.Get(ctx, r.path); err == nil {
			username = rec.ModifiedBy
		}
		if err := m.applyOne(ctx, r); err != nil {
			log.Printf("syncserver: materialize %s failed: %v", r.path, err)
			m.healthy.Store(false)
			continue
		}
		changes = append(changes, pendingChange{
			path: r.path, op: r.op, username: username,
			revision: r.revision, handoffID: m.handoffID(ctx, r.path),
		})
		anyStaged = true
	}
	// Collect the IDs of every row for a path we successfully applied
	// (dedup means multiple stale rows for the same path all clear together).
	appliedPaths := map[string]bool{}
	for _, c := range changes {
		appliedPaths[c.path] = true
	}
	for _, r := range rows {
		if appliedPaths[r.path] {
			appliedIDs = append(appliedIDs, r.id)
		}
	}

	if !anyStaged {
		return
	}

	if _, err := m.runner.Run("add", "-A"); err != nil {
		log.Printf("syncserver: git add -A failed: %v", err)
		m.healthy.Store(false)
		return
	}

	msg := commitMessage(changes)
	if subject != "" {
		msg = subject + msg[strings.Index(msg, "\n"):]
	}
	if _, err := m.runner.Run("commit", "-m", msg); err != nil {
		// Nothing to commit is not a failure (e.g. content matched what's
		// already on disk); anything else is.
		if !strings.Contains(err.Error(), "nothing to commit") {
			log.Printf("syncserver: git commit failed: %v", err)
			m.healthy.Store(false)
			return
		}
	}

	sha, _ := m.runner.Run("rev-parse", "HEAD")
	sha = strings.TrimSpace(sha)

	if err := m.clearPending(ctx, appliedIDs, sha, changes); err != nil {
		log.Printf("syncserver: failed to clear pending materializations after commit: %v", err)
		// The commit landed but we couldn't clear the queue; next Flush
		// will just re-materialize identical content (idempotent) and
		// re-commit (a no-op "nothing to commit"), so no data is lost.
		return
	}
	m.healthy.Store(true)
}

func (m *GitMaterializer) loadPending(ctx context.Context) ([]pendingRow, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, path, revision, op FROM pending_materializations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.id, &r.path, &r.revision, &r.op); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// applyOne performs only the filesystem-level side effect for one pending
// row. It deliberately does NOT call `git add` per path: staging a path
// that was never tracked and no longer exists (e.g. write-then-delete
// within one debounce window) makes git's pathspec matching fail. Callers
// stage everything at once via a single `git add -A` over the whole
// working tree after all filesystem changes are applied.
func (m *GitMaterializer) applyOne(ctx context.Context, r pendingRow) error {
	full := filepath.Join(m.workspace, filepath.FromSlash(r.path))
	if r.op == "delete" {
		if err := os.RemoveAll(full); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", r.path, err)
		}
		return nil
	}

	rec, err := m.store.Get(ctx, r.path)
	if err != nil {
		return fmt.Errorf("load current content for %s: %w", r.path, err)
	}
	if rec.Deleted {
		// The file was deleted again after this write was queued; the
		// later delete's own pending row (if present) will handle it.
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", r.path, err)
	}
	if err := os.WriteFile(full, rec.Content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", r.path, err)
	}
	return nil
}

func (m *GitMaterializer) clearPending(ctx context.Context, ids []int64, sha string, changes []pendingChange) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM pending_materializations WHERE id = ?`, id); err != nil {
			return err
		}
	}

	type changeJSON struct {
		Path     string `json:"path"`
		Op       string `json:"op"`
		Username string `json:"username"`
	}
	jsonChanges := make([]changeJSON, 0, len(changes))
	for _, c := range changes {
		jsonChanges = append(jsonChanges, changeJSON{Path: c.path, Op: c.op, Username: c.username})
	}
	pathsJSON, err := json.Marshal(jsonChanges)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO commit_batches (started_at, committed_at, git_commit_sha, paths_json, status)
		VALUES (?, ?, ?, ?, 'committed')`, now(), now(), sha, string(pathsJSON)); err != nil {
		return err
	}

	return tx.Commit()
}

// handoffID resolves a handoff path's stable id from its latest content
// revision. A tombstoned handoff (i.e. one that was just resolved) has no
// current content, so this reads the last revision that had some -- which is
// exactly the resolved copy retained for history.
func (m *GitMaterializer) handoffID(ctx context.Context, path string) string {
	if !handoff.IsPath(path) {
		return ""
	}
	content, _, err := m.store.LatestContentRevision(ctx, path)
	if err != nil {
		return ""
	}
	return handoff.ParseMeta(content).ID
}

// changeVerb describes one materialized change for the commit body. Handoffs
// get their own vocabulary so `git log` reads as a handoff history.
func changeVerb(c pendingChange) string {
	if !handoff.IsPath(c.path) {
		if c.op == "delete" {
			return "deleted"
		}
		return "modified"
	}
	switch {
	case c.op == "delete":
		return "handoff resolve"
	case c.revision <= 1:
		return "handoff create"
	default:
		return "handoff update"
	}
}

func commitMessage(changes []pendingChange) string {
	users := map[string]bool{}
	for _, c := range changes {
		users[c.username] = true
	}
	var names []string
	for u := range users {
		names = append(names, u)
	}
	sort.Strings(names)

	allHandoffs := true
	for _, c := range changes {
		if !handoff.IsPath(c.path) {
			allHandoffs = false
			break
		}
	}

	var b strings.Builder
	subject := "sync"
	if allHandoffs {
		subject = "handoff"
	}
	fmt.Fprintf(&b, "%s: %d file(s) by %s\n\n", subject, len(changes), strings.Join(names, ", "))
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	for _, c := range changes {
		if c.handoffID != "" {
			fmt.Fprintf(&b, "- %s %s [%s] (%s)\n", changeVerb(c), c.path, c.handoffID, c.username)
			continue
		}
		fmt.Fprintf(&b, "- %s %s (%s)\n", changeVerb(c), c.path, c.username)
	}
	return b.String()
}

// DriftCheckAndRepair compares the git workspace's on-disk state against
// the canonical DB and re-materializes any mismatch, since the DB is
// always authoritative. Returns the number of paths repaired. If any
// repair was made, the result is committed as a single "startup drift
// repair" commit.
func (m *GitMaterializer) DriftCheckAndRepair(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dbFiles, err := m.store.Manifest(ctx)
	if err != nil {
		return 0, fmt.Errorf("load manifest: %w", err)
	}
	dbHashes := map[string]string{}
	for _, f := range dbFiles {
		if !f.Deleted {
			dbHashes[f.Path] = f.Hash
		}
	}

	onDisk, err := m.walkWorkspace()
	if err != nil {
		return 0, fmt.Errorf("walk workspace: %w", err)
	}

	var deleteCandidates []string
	for path := range onDisk {
		if _, wanted := dbHashes[path]; !wanted {
			deleteCandidates = append(deleteCandidates, path)
		}
	}
	sort.Strings(deleteCandidates)

	// Checked before touching anything: a majority of the workspace missing
	// from the DB means the wrong or a fresh --db, and serving that empty
	// manifest makes every client delete its copy too (issue #62).
	if len(deleteCandidates) >= driftDeleteMinCount && len(onDisk) > 0 &&
		float64(len(deleteCandidates))/float64(len(onDisk)) > driftDeleteMaxFraction {
		sample := deleteCandidates
		if len(sample) > 10 {
			sample = sample[:10]
		}
		return 0, fmt.Errorf("%d of the %d files in workspace %s are not in the database (sample: %v) -- this looks "+
			"like the wrong or a fresh --db, not drift, so nothing was changed. Point --db at this workspace's "+
			"database; to deliberately start over, move the workspace aside",
			len(deleteCandidates), len(onDisk), m.workspace, sample)
	}

	repaired := 0
	var changes []pendingChange

	for path, wantHash := range dbHashes {
		gotHash, exists := onDisk[path]
		if exists && gotHash == wantHash {
			continue
		}
		rec, err := m.store.Get(ctx, path)
		if err != nil {
			return repaired, fmt.Errorf("load %s for repair: %w", path, err)
		}
		full := filepath.Join(m.workspace, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return repaired, err
		}
		if err := os.WriteFile(full, rec.Content, 0o644); err != nil {
			return repaired, err
		}
		repaired++
		changes = append(changes, pendingChange{path: path, op: "write", username: "repair"})
	}

	for _, path := range deleteCandidates {
		full := filepath.Join(m.workspace, filepath.FromSlash(path))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return repaired, err
		}
		repaired++
		changes = append(changes, pendingChange{path: path, op: "delete", username: "repair"})
	}

	if repaired > 0 {
		if _, err := m.runner.Run("add", "-A"); err != nil {
			return repaired, fmt.Errorf("stage drift repair: %w", err)
		}
		msg := driftCommitMessage(repaired, changes)
		if _, err := m.runner.Run("commit", "-m", msg); err != nil {
			if !strings.Contains(err.Error(), "nothing to commit") {
				return repaired, fmt.Errorf("commit drift repair: %w", err)
			}
		}
	}

	return repaired, nil
}

// driftCommitMessage lists every path a drift repair actually touched, so a
// repair commit is self-diagnosing from `git log` alone instead of just a
// bare count.
func driftCommitMessage(repaired int, changes []pendingChange) string {
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	var b strings.Builder
	fmt.Fprintf(&b, "rex-svr: startup drift repair (%d path(s))\n\n", repaired)
	for _, c := range changes {
		verb := "wrote"
		if c.op == "delete" {
			verb = "removed"
		}
		fmt.Fprintf(&b, "- %s %s\n", verb, c.path)
	}
	return b.String()
}

// walkWorkspace returns path -> sha256 for every tracked file on disk
// (excluding .git and anything matching the shared exclude list).
func (m *GitMaterializer) walkWorkspace() (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(m.workspace, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(m.workspace, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if d.IsDir() {
			if slashRel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if synccommon.IsExcluded(slashRel) {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[slashRel] = synccommon.HashBytes(content)
		return nil
	})
	return out, err
}

// Blame runs `git blame` over path's materialized copy. Commits are authored
// as rex-svr, so each line's user comes from the commit body's
// "- <verb> <path> (<user>)" entry for that path. Lines not committed yet
// (still in the debounce window) are attributed to the file's last modifier.
func (m *GitMaterializer) Blame(ctx context.Context, path string) ([]syncproto.BlameLine, error) {
	m.mu.Lock()
	out, err := m.runner.Run("blame", "--line-porcelain", "--", path)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	users := map[string]string{}
	lines := []syncproto.BlameLine{}
	var cur syncproto.BlameLine
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "\t"):
			cur.Line = len(lines) + 1
			cur.Text = l[1:]
			lines = append(lines, cur)
			cur = syncproto.BlameLine{}
		case strings.HasPrefix(l, "author-time "):
			if sec, err := strconv.ParseInt(l[len("author-time "):], 10, 64); err == nil {
				cur.Time = time.Unix(sec, 0).UTC().Format(time.RFC3339)
			}
		case strings.HasPrefix(l, "summary "):
			cur.Summary = l[len("summary "):]
		case len(l) > 40 && l[40] == ' ' && isHex(l[:40]):
			sha := l[:40]
			if strings.Trim(sha, "0") == "" {
				cur.Summary = "Not committed yet"
				if rec, err := m.store.Get(ctx, path); err == nil {
					cur.User = rec.ModifiedBy
				}
				continue
			}
			cur.Commit = sha
			if _, ok := users[sha]; !ok {
				body, _ := m.runner.Run("show", "-s", "--format=%B", sha)
				users[sha] = commitUser(body, path)
			}
			cur.User = users[sha]
		}
	}
	return lines, nil
}

// commitUser finds who changed path in a commit body written by
// commitMessage ("- modified path (alice)"); drift repairs carry no user.
func commitUser(body, path string) string {
	for _, l := range strings.Split(body, "\n") {
		if !strings.HasPrefix(l, "- ") || !strings.Contains(l+" ", " "+path+" ") {
			continue
		}
		if i := strings.LastIndex(l, " ("); i >= 0 && strings.HasSuffix(l, ")") {
			return l[i+2 : len(l)-1]
		}
		return "repair"
	}
	return "rex-svr"
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
