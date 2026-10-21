package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/syncproto"
)

var (
	statusFlagsVal syncFlags
	statusRecent   int
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Test the connection to a rex-svr and summarize the workspace: health, login, recent changes, open conflicts",
	RunE:  runStatus,
}

func init() {
	f := statusCmd.Flags()
	f.StringVar(&statusFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&statusFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&statusFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&statusFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&statusFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&statusFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&statusFlagsVal.JSON, "json", false, "print the report as JSON")
	f.IntVar(&statusRecent, "recent", 20, "how many of the most recently changed files to list")

	rootCmd.AddCommand(statusCmd)
}

// statusReport is `rex status --json`'s output. On failure Error is set and
// Hint says which layer failed: config (missing flags/password), tls
// (certificate trust), unreachable (network/server down), or auth (login
// rejected) -- so an editor can say what to fix, not just "failed".
type statusReport struct {
	Server        string                      `json:"server"`
	User          string                      `json:"user"`
	Reachable     bool                        `json:"reachable"`
	LatencyMS     int64                       `json:"latency_ms"`
	Health        *syncproto.HealthResponse   `json:"health,omitempty"`
	Authenticated bool                        `json:"authenticated"`
	Files         int                         `json:"files"`
	Recent        []syncproto.FileEntry       `json:"recent"`
	Conflicts     []syncproto.ConflictSummary `json:"conflicts"`
	Error         string                      `json:"error,omitempty"`
	Hint          string                      `json:"hint,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	flags := statusFlagsVal
	rep := collectStatus(context.Background(), flags, statusRecent)
	if flags.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(rep)
	} else {
		printStatus(rep)
	}
	if rep.Error != "" {
		os.Exit(1) // already reported; skip cobra's usage dump
	}
	return nil
}

func collectStatus(ctx context.Context, flags syncFlags, recent int) statusReport {
	rep := statusReport{
		Server: fmt.Sprintf("%s:%d", flags.Host, flags.Port), User: flags.Username,
		Recent: []syncproto.FileEntry{}, Conflicts: []syncproto.ConflictSummary{},
	}
	fail := func(hint string, err error) statusReport {
		rep.Hint, rep.Error = hint, err.Error()
		return rep
	}
	if flags.Host == "" || flags.Username == "" {
		return fail("config", fmt.Errorf("--host and --username are required"))
	}
	tlsConfig, err := buildTLSConfig(flags)
	if err != nil {
		return fail("tls", err)
	}
	client := syncclient.NewClient(flags.Host, flags.Port, tlsConfig)

	hctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	start := time.Now()
	health, err := client.Healthz(hctx)
	if err != nil {
		return fail(classifyDialError(err), err)
	}
	rep.Reachable, rep.Health, rep.LatencyMS = true, health, time.Since(start).Milliseconds()

	password, err := resolveSyncPassword(flags)
	if err != nil {
		return fail("config", err)
	}
	if _, err := client.Login(ctx, flags.Username, password); err != nil {
		return fail("auth", err)
	}
	rep.Authenticated = true

	files, err := client.Manifest(ctx)
	if err != nil {
		return fail("server", fmt.Errorf("fetch manifest: %w", err))
	}
	for _, f := range files {
		if !f.Deleted {
			rep.Files++
		}
	}
	// RFC3339Nano UTC timestamps sort correctly as strings.
	sort.Slice(files, func(i, j int) bool { return files[i].UpdatedAt > files[j].UpdatedAt })
	if len(files) > recent {
		files = files[:recent]
	}
	rep.Recent = append(rep.Recent, files...)

	conflicts, err := client.ListConflicts(ctx)
	if err != nil {
		return fail("server", fmt.Errorf("list conflicts: %w", err))
	}
	rep.Conflicts = append(rep.Conflicts, conflicts...)
	return rep
}

func classifyDialError(err error) string {
	msg := err.Error()
	for _, s := range []string{"certificate", "x509", "tls:"} {
		if strings.Contains(msg, s) {
			return "tls"
		}
	}
	return "unreachable"
}

func printStatus(r statusReport) {
	fmt.Printf("Server:  %s\nUser:    %s\n", r.Server, r.User)
	if r.Reachable {
		fmt.Printf("Health:  %s (git_ok=%v db_ok=%v pending=%d, %dms", r.Health.Status, r.Health.GitOK, r.Health.DBOK,
			r.Health.PendingMaterializations, r.LatencyMS)
		if r.Health.Version != "" {
			fmt.Printf(", rex-svr %s", r.Health.Version)
		}
		fmt.Println(")")
	}
	if r.Error != "" {
		fmt.Printf("\nFAILED (%s): %s\n", r.Hint, r.Error)
		return
	}
	fmt.Printf("Login:   ok\nFiles:   %d\nOpen conflicts: %d\n", r.Files, len(r.Conflicts))
	if len(r.Recent) > 0 {
		fmt.Println("\nRecent changes:")
		for _, f := range r.Recent {
			what := fmt.Sprintf("rev %d", f.Revision)
			if f.Deleted {
				what = "deleted"
			}
			fmt.Printf("  %-24s %-14s %-9s %s\n", f.UpdatedAt, f.ModifiedBy, what, f.Path)
		}
	}
}
