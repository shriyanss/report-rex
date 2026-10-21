package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

var (
	conflictsFlagsVal syncFlags
	conflictsID       int64
	conflictsSide     string
)

var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "List open synchronization conflicts on a rex-svr",
	RunE:  runConflicts,
}

func init() {
	f := conflictsCmd.Flags()
	f.StringVar(&conflictsFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&conflictsFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&conflictsFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&conflictsFlagsVal.Password, "password", "", "shared server password (prompted interactively if omitted)")
	f.StringVar(&conflictsFlagsVal.Dir, "dir", ".", "workspace directory")
	f.StringVar(&conflictsFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&conflictsFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&conflictsFlagsVal.JSON, "json", false, "print {\"conflicts\": [...]} as JSON instead of a table")
	f.Int64Var(&conflictsID, "id", 0, "with --side: the conflict to print")
	f.StringVar(&conflictsSide, "side", "", "print one side of conflict --id as raw bytes: local (the rejected upload), remote (the server's current copy), or base")

	rootCmd.AddCommand(conflictsCmd)
}

// buildTLSConfig resolves flags.CACert/Insecure into the TLS config used to
// connect to rex-svr, shared by every command built on syncFlags.
func buildTLSConfig(flags syncFlags) (*tls.Config, error) {
	return syncclient.BuildTLSConfig(flags.CACert, flags.Insecure)
}

func dialSyncClient(ctx context.Context, flags syncFlags) (*syncclient.Client, error) {
	if flags.Host == "" || flags.Username == "" {
		return nil, fmt.Errorf("--host and --username are required")
	}
	password, err := resolveSyncPassword(flags)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := buildTLSConfig(flags)
	if err != nil {
		return nil, err
	}
	client := syncclient.NewClient(flags.Host, flags.Port, tlsConfig)
	if _, err := client.Login(ctx, flags.Username, password); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	return client, nil
}

func runConflicts(cmd *cobra.Command, args []string) error {
	if (conflictsSide == "") != (conflictsID == 0) {
		return fmt.Errorf("--side and --id go together")
	}
	ctx := context.Background()
	client, err := dialSyncClient(ctx, conflictsFlagsVal)
	if err != nil {
		return err
	}
	if conflictsSide != "" {
		return printConflictSide(ctx, client, conflictsID, conflictsSide)
	}

	conflicts, err := client.ListConflicts(ctx)
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}
	if conflictsFlagsVal.JSON {
		if conflicts == nil {
			conflicts = []syncproto.ConflictSummary{}
		}
		return json.NewEncoder(os.Stdout).Encode(syncproto.ConflictsResponse{Conflicts: conflicts})
	}
	if len(conflicts) == 0 {
		fmt.Println("No open conflicts.")
		return nil
	}

	fmt.Printf("%-6s %-40s %-22s %-14s %-8s %s\n", "ID", "PATH", "KIND", "BY", "REV", "CREATED")
	for _, c := range conflicts {
		fmt.Printf("%-6d %-40s %-22s %-14s %-8d %s\n", c.ID, c.Path, c.Kind, c.LocalUsername, c.RemoteRevision, c.CreatedAt)
	}
	fmt.Println("\nRun `rex resolve <path> --id <ID> --remote|--local|--merge|--keep-both` to resolve one.")
	return nil
}

// printConflictSide writes one side of a conflict to stdout verbatim, e.g.
// for an editor to diff the server's copy against the local file.
func printConflictSide(ctx context.Context, client *syncclient.Client, id int64, side string) error {
	d, err := client.GetConflict(ctx, id, side == "local", side == "remote", side == "base")
	if err != nil {
		return fmt.Errorf("fetch conflict %d: %w", id, err)
	}
	var content []byte
	switch side {
	case "local":
		content = d.LocalContent
	case "remote":
		content = d.RemoteContent
	case "base":
		if !d.HasBase {
			return fmt.Errorf("conflict %d has no common base (%s)", id, d.Kind)
		}
		content = d.BaseContent
	default:
		return fmt.Errorf("--side must be local, remote, or base")
	}
	_, err = os.Stdout.Write(content)
	return err
}
