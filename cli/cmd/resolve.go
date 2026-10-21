package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

var (
	resolveFlagsVal   syncFlags
	resolveUseRemote  bool
	resolveUseLocal   bool
	resolveMerge      bool
	resolveKeepBoth   bool
	resolveSuffix     string
	resolveConflictID int64
)

var resolveCmd = &cobra.Command{
	Use:   "resolve <path>",
	Short: "Resolve an open synchronization conflict for a path",
	Args:  cobra.ExactArgs(1),
	RunE:  runResolve,
}

func init() {
	f := resolveCmd.Flags()
	f.StringVar(&resolveFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&resolveFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&resolveFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&resolveFlagsVal.Password, "password", "", "shared server password (prompted interactively if omitted)")
	f.StringVar(&resolveFlagsVal.Dir, "dir", ".", "workspace directory")
	f.StringVar(&resolveFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&resolveFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")

	f.BoolVar(&resolveUseRemote, "remote", false, "keep the server's version; overwrite the local copy")
	f.BoolVar(&resolveUseLocal, "local", false, "make the local version canonical on the server")
	f.BoolVar(&resolveMerge, "merge", false, "attempt an automatic three-way merge (text files only)")
	f.BoolVar(&resolveKeepBoth, "keep-both", false, "keep both versions as separate files (binary conflicts)")
	f.StringVar(&resolveSuffix, "suffix", "", "suffix for --keep-both's derived filename (default: your username)")
	f.Int64Var(&resolveConflictID, "id", 0, "disambiguate which conflict to resolve if a path has more than one open")

	rootCmd.AddCommand(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	path := args[0]

	selected := 0
	for _, b := range []bool{resolveUseRemote, resolveUseLocal, resolveMerge, resolveKeepBoth} {
		if b {
			selected++
		}
	}
	if selected != 1 {
		return fmt.Errorf("exactly one of --remote, --local, --merge, --keep-both is required")
	}

	ctx := context.Background()
	client, err := dialSyncClient(ctx, resolveFlagsVal)
	if err != nil {
		return err
	}

	conflict, err := findConflict(ctx, client, path, resolveConflictID, resolveFlagsVal.Username)
	if err != nil {
		return err
	}

	req := syncproto.ResolveRequest{KeepBothSuffix: resolveSuffix}
	switch {
	case resolveUseRemote:
		// For a delete/edit conflict, "remote" means the deletion wins.
		if conflict.Kind == syncproto.ConflictKindDeleteEdit {
			req.Resolution = syncproto.ResolutionKeepDeletion
		} else {
			req.Resolution = syncproto.ResolutionUseRemote
		}
	case resolveUseLocal:
		// For a delete/edit conflict, "local" means the edit is restored,
		// overriding the deletion.
		if conflict.Kind == syncproto.ConflictKindDeleteEdit {
			req.Resolution = syncproto.ResolutionRestoreLocal
		} else {
			req.Resolution = syncproto.ResolutionUseLocal
		}
	case resolveKeepBoth:
		req.Resolution = syncproto.ResolutionKeepBoth
	case resolveMerge:
		if conflict.Kind != syncproto.ConflictKindDivergentEdit && conflict.Kind != syncproto.ConflictKindInitialUnknown {
			return fmt.Errorf("--merge is only valid for text-content conflicts, not %s", conflict.Kind)
		}
		return runMergeResolve(ctx, client, resolveFlagsVal, conflict)
	}

	resp, err := client.ResolveConflict(ctx, conflict.ID, req)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	if err := applyResolutionLocally(resolveFlagsVal.Dir, client, req.Resolution, resp); err != nil {
		return err
	}

	fmt.Printf("Resolved %s (%s)\n", path, req.Resolution)
	return nil
}

// findConflict looks up the open conflict for path. The server keeps at
// most one open conflict per (path, user), so when several teammates each
// have one on the same path, the caller's own is the natural pick; id
// disambiguates anything else.
func findConflict(ctx context.Context, client *syncclient.Client, path string, id int64, username string) (*syncproto.ConflictSummary, error) {
	conflicts, err := client.ListConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list conflicts: %w", err)
	}
	var matches []syncproto.ConflictSummary
	for _, c := range conflicts {
		if c.Path == path && (id == 0 || c.ID == id) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no open conflict found for %s", path)
	}
	if len(matches) > 1 && id == 0 {
		var mine []syncproto.ConflictSummary
		for _, c := range matches {
			if c.LocalUsername == username {
				mine = append(mine, c)
			}
		}
		if len(mine) == 1 {
			matches = mine
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("%d open conflicts for %s; disambiguate with --id (run `rex conflicts`)", len(matches), path)
	}
	return &matches[0], nil
}

// applyResolutionLocally performs the client-side filesystem effect of a
// resolution: use_remote overwrites the local file from the server (the
// server makes no state change for this case, so the client must fetch
// the canonical content itself); the other resolutions already changed
// server state, so the client just re-pulls to pick up the new revision.
func applyResolutionLocally(dir string, client *syncclient.Client, resolution syncproto.ResolutionKind, resp *syncproto.ResolveResponse) error {
	ctx := context.Background()
	content, _, _, ok, err := client.GetFile(ctx, resp.Path)
	if err != nil {
		return fmt.Errorf("fetch resolved content: %w", err)
	}
	if !ok {
		return syncclient.RemoveFile(dir, resp.Path)
	}
	if err := syncclient.WriteFile(dir, resp.Path, content); err != nil {
		return err
	}
	if resp.NewPath != "" {
		keepBothContent, _, _, ok, err := client.GetFile(ctx, resp.NewPath)
		if err == nil && ok {
			_ = syncclient.WriteFile(dir, resp.NewPath, keepBothContent)
		}
	}
	return nil
}

var _ = base64.StdEncoding // merged_content travels as []byte, JSON-encodes as base64 automatically

// runMergeResolve settles a text-content conflict by merging. First run:
// three-way merge BASE/LOCAL/REMOTE -- a clean merge is submitted
// immediately; overlapping changes are written into the file as git-style
// markers and the attempt is recorded locally. Second run (after the user
// has edited the markers out): the file on disk is submitted as the
// resolution, pinned to the remote revision the markers were built from.
func runMergeResolve(ctx context.Context, client *syncclient.Client, flags syncFlags, conflict *syncproto.ConflictSummary) error {
	detail, err := client.GetConflict(ctx, conflict.ID, true, true, true)
	if err != nil {
		return fmt.Errorf("fetch conflict detail: %w", err)
	}
	local, err := syncclient.OpenLocalStore(flags.Dir)
	if err != nil {
		return err
	}
	defer local.Close()

	localOnDisk, err := syncclient.ReadFile(flags.Dir, conflict.Path)
	if err != nil {
		return fmt.Errorf("read local file %s: %w", conflict.Path, err)
	}

	if syncclient.HasConflictMarkers(localOnDisk) {
		return fmt.Errorf("%s still has <<<<<<< LOCAL / ======= / >>>>>>> REMOTE markers; edit them out, save, and re-run `rex resolve %s --id %d --merge`",
			conflict.Path, conflict.Path, conflict.ID)
	}
	if builtOn, ok := local.PendingMerge(conflict.ID); ok {
		fmt.Println("Submitting your hand-resolved content.")
		detail.RemoteContentRevision = builtOn
		err := submitMerge(ctx, client, detail, flags.Dir, localOnDisk)
		if err != nil && strings.Contains(err.Error(), "stale_remote") {
			_ = local.ClearPendingMerge(conflict.ID)
			return fmt.Errorf("%s changed on the server while you were merging; re-run `rex resolve %s --id %d --merge` to merge your version with the new one",
				conflict.Path, conflict.Path, conflict.ID)
		}
		if err == nil {
			_ = local.ClearPendingMerge(conflict.ID)
		}
		return err
	}

	base := detail.BaseContent
	if !detail.HasBase {
		base = nil
	}

	merged, clean := syncclient.ThreeWayMerge(base, localOnDisk, detail.RemoteContent)
	if clean {
		fmt.Println("Automatic merge succeeded.")
		return submitMerge(ctx, client, detail, flags.Dir, merged)
	}

	if err := syncclient.WriteFile(flags.Dir, conflict.Path, merged); err != nil {
		return fmt.Errorf("write conflict markers: %w", err)
	}
	if err := local.MarkPendingMerge(conflict.ID, conflict.Path, string(conflict.Kind), detail.RemoteContentRevision); err != nil {
		return err
	}
	fmt.Printf("Automatic merge failed: %s has overlapping changes.\n", conflict.Path)
	fmt.Println("Edit the file to resolve the <<<<<<< LOCAL / ======= / >>>>>>> REMOTE markers, then re-run:")
	fmt.Printf("  rex resolve %s --id %d --merge\n", conflict.Path, conflict.ID)
	return nil
}

// submitMerge submits content as detail's merge resolution, pinned to the
// remote revision the merge was built on, so a teammate's push in the
// meantime is refused (re-run to merge on top of it) rather than
// overwritten.
func submitMerge(ctx context.Context, client *syncclient.Client, detail *syncproto.ConflictDetail, dir string, content []byte) error {
	req := syncproto.ResolveRequest{Resolution: syncproto.ResolutionMerge, MergedContent: content}
	if detail.RemoteContentRevision != 0 {
		req.ExpectedRemoteRevision = &detail.RemoteContentRevision
	}
	path := detail.Path
	resp, err := client.ResolveConflict(ctx, detail.ID, req)
	if err != nil {
		return fmt.Errorf("submit merge: %w", err)
	}
	if err := syncclient.WriteFile(dir, resp.Path, content); err != nil {
		return err
	}
	fmt.Printf("Resolved %s (merge)\n", path)
	return nil
}
