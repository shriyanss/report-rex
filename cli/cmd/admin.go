package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var adminFlagsVal syncFlags

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "rex-svr admin helpers",
}

var adminVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check rex-svr's admin password without changing anything",
	Long: `Check an admin password against a rex-svr. Exits 0 if it's right; fails
if it's wrong or the server has no --admin-password (admin actions off).
The VS Code extension uses this to unlock its admin features.

The admin password is read from $REX_ADMIN_PASSWORD, or prompted for.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true, // a wrong password isn't a usage error
	RunE:         runAdminVerify,
}

func init() {
	f := adminVerifyCmd.Flags()
	f.StringVar(&adminFlagsVal.Host, "host", "", "sync server host (required)")
	f.IntVar(&adminFlagsVal.Port, "port", 9443, "sync server port")
	f.StringVar(&adminFlagsVal.Username, "username", "", "username to authenticate as (required)")
	f.StringVar(&adminFlagsVal.Password, "password", "", "shared server password (prompted interactively, or $REX_PASSWORD, if omitted)")
	f.StringVar(&adminFlagsVal.CACert, "ca-cert", "", "path to the server's self-signed certificate (printed by rex-svr on startup), to trust it")
	f.BoolVar(&adminFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")

	adminCmd.AddCommand(adminVerifyCmd)
	rootCmd.AddCommand(adminCmd)
}

func runAdminVerify(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := dialSyncClient(ctx, adminFlagsVal)
	if err != nil {
		return err
	}
	admin, err := resolveAdminPassword()
	if err != nil {
		return err
	}
	if err := client.VerifyAdmin(ctx, admin); err != nil {
		return err
	}
	fmt.Println("admin password OK")
	return nil
}
