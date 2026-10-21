package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var blameFlagsVal syncFlags

var blameCmd = &cobra.Command{
	Use:   "blame <path>",
	Short: "Show who last changed each line of a synced file, from rex-svr's git history",
	Long: `Show, for each line of a workspace-relative path on a rex-svr, which user
last changed it, when, and in which server git commit. Lines saved in the last
few seconds may show as "Not committed yet" until the server commits them.`,
	Args: cobra.ExactArgs(1),
	RunE: runBlame,
}

func init() {
	f := blameCmd.Flags()
	f.StringVar(&blameFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&blameFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&blameFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&blameFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&blameFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&blameFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")
	f.BoolVar(&blameFlagsVal.JSON, "json", false, "print the result as JSON")

	rootCmd.AddCommand(blameCmd)
}

func runBlame(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, blameFlagsVal)
	if err != nil {
		return err
	}
	res, err := client.Blame(ctx, args[0])
	if err != nil {
		return err
	}
	if blameFlagsVal.JSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	for _, l := range res.Lines {
		sha := "        "
		if len(l.Commit) >= 8 {
			sha = l.Commit[:8]
		}
		date := l.Time
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Printf("%s %-12.12s %-10s %4d| %s\n", sha, l.User, date, l.Line, l.Text)
	}
	return nil
}
