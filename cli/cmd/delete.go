package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

var deleteFlagsVal syncFlags

var deleteCmd = &cobra.Command{
	Use:   "delete <path>...",
	Short: "Delete files from the synced report for everyone (needs rex-svr's admin password)",
	Long: `Delete workspace-relative paths from a rex-svr in one step, for everyone.

` + "`rex sync`" + ` refuses to push a local delete of nearly the whole workspace
(at least 5 files and over 90% of it): that's far likelier a moved or emptied
directory than intent. This is the deliberate way to do it. Connected clients
remove the files on their next sync; the server keeps every file's history
and records one git commit.

The admin password is read from $REX_ADMIN_PASSWORD, or prompted for.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDelete,
}

func init() {
	f := deleteCmd.Flags()
	f.StringVar(&deleteFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&deleteFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&deleteFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&deleteFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&deleteFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&deleteFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&deleteFlagsVal.JSON, "json", false, "print the result as JSON")

	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, deleteFlagsVal)
	if err != nil {
		return err
	}
	admin, err := resolveAdminPassword()
	if err != nil {
		return err
	}
	res, err := client.BulkDelete(ctx, syncproto.BulkDeleteRequest{Paths: args, AdminPassword: admin})
	if err != nil {
		return err
	}
	if deleteFlagsVal.JSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	fmt.Printf("Deleted %d file(s)", len(res.Deleted))
	if res.Commit != "" {
		fmt.Printf(" (commit %s)", res.Commit)
	}
	fmt.Println()
	for _, p := range res.Skipped {
		fmt.Printf("  skipped  %s (missing, already deleted, excluded from sync, or a handoff)\n", p)
	}
	return nil
}
