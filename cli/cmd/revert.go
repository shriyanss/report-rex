package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

var (
	revertFlagsVal syncFlags
	revertPath     string
	revertRevision int64
	revertPlan     string
	revertBefore   bool
)

var revertCmd = &cobra.Command{
	Use:   "revert",
	Short: "Revert the whole synced report to how it stood at one change (needs rex-svr's admin password)",
	Long: `Revert the whole synced report on a rex-svr to how it stood right after
--path reached --revision -- or, with --before, right before it, undoing that
change and everything after it.

Without --plan this only previews: it lists every file that would change, the
server git commit that recorded the target change, and the recent git log, and
prints a plan token. Re-run with --plan <token> to apply exactly that plan; if
anything changed on the server in between, the revert is refused.

A revert never rewrites history: each file gets a new revision holding its old
content, and the server's git history gains one "Revert to ..." commit.

The admin password is read from $REX_ADMIN_PASSWORD, or prompted for.`,
	RunE: runRevert,
}

func init() {
	f := revertCmd.Flags()
	f.StringVar(&revertFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&revertFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&revertFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&revertFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&revertFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&revertFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&revertFlagsVal.JSON, "json", false, "print the preview/result as JSON")
	f.StringVar(&revertPath, "path", "", "the change's file path (required)")
	f.Int64Var(&revertRevision, "revision", 0, "the change's revision (required)")
	f.BoolVar(&revertBefore, "before", false, "revert to right before the change instead of right after it")
	f.StringVar(&revertPlan, "plan", "", "apply the previewed plan with this token (omit to preview)")

	rootCmd.AddCommand(revertCmd)
}

func resolveAdminPassword() (string, error) {
	if pw := os.Getenv("REX_ADMIN_PASSWORD"); pw != "" {
		return pw, nil
	}
	fmt.Fprint(os.Stderr, "Admin password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read admin password: %w", err)
	}
	return string(pw), nil
}

func runRevert(cmd *cobra.Command, args []string) error {
	if revertPath == "" || revertRevision <= 0 {
		return fmt.Errorf("--path and --revision are required")
	}
	ctx := context.Background()
	client, err := dialSyncClient(ctx, revertFlagsVal)
	if err != nil {
		return err
	}
	admin, err := resolveAdminPassword()
	if err != nil {
		return err
	}
	req := syncproto.RevertRequest{Path: revertPath, Revision: revertRevision, Before: revertBefore, AdminPassword: admin, Plan: revertPlan}

	if revertPlan == "" {
		p, err := client.RevertPreview(ctx, req)
		if err != nil {
			return err
		}
		if revertFlagsVal.JSON {
			return json.NewEncoder(os.Stdout).Encode(p)
		}
		to := "to"
		if p.Before {
			to = "to before"
		}
		fmt.Printf("Revert %s %s rev %d\n", to, p.Path, p.Revision)
		if p.Commit != "" {
			fmt.Printf("That change's commit: %s\n", p.Commit)
		}
		if len(p.Ops) == 0 {
			fmt.Println("\nNothing to change: the report already matches that point.")
			return nil
		}
		fmt.Printf("\n%d file(s) would change:\n", len(p.Ops))
		for _, op := range p.Ops {
			if op.Action == "delete" {
				fmt.Printf("  delete   %s (now rev %d)\n", op.Path, op.FromRevision)
			} else {
				fmt.Printf("  restore  %s (rev %d -> content of rev %d)\n", op.Path, op.FromRevision, op.TargetRevision)
			}
		}
		fmt.Println("\nRecent server git log:")
		for _, l := range p.GitLog {
			fmt.Println("  " + l)
		}
		fmt.Printf("\nApply with: --plan %s\n", p.Plan)
		return nil
	}

	res, err := client.Revert(ctx, req)
	if err != nil {
		return err
	}
	if revertFlagsVal.JSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	to := "to"
	if revertBefore {
		to = "to before"
	}
	fmt.Printf("Reverted %d file(s) %s %s rev %d", len(res.Ops), to, revertPath, revertRevision)
	if res.Commit != "" {
		fmt.Printf(" (commit %s)", res.Commit)
	}
	fmt.Println()
	return nil
}
