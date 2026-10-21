// Command rex-svr is the standalone sync server binary: it owns
// the canonical SQLite-backed file store and (from Phase 2 onward) the
// server-side Git history projection. It is distributed separately from
// the rex client binary.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/gitexec"
	"github.com/shriyanss/report-rex/cli/internal/syncserver"
	"github.com/shriyanss/report-rex/cli/internal/version"
)

var (
	flagPort         int
	flagPassword     string
	flagPasswordFile string
	flagAdminPass    string
	flagAdminFile    string
	flagDB           string
	flagWorkspace    string
	flagLogLevel     string
	flagCert         string
	flagKey          string
)

var rootCmd = &cobra.Command{
	Use:     "rex-svr",
	Short:   "Standalone sync server for report-rex collaborative report workspaces",
	Version: version.Version,
	RunE:    runServer,
}

func init() {
	f := rootCmd.Flags()
	f.IntVar(&flagPort, "port", 9443, "TCP port to listen on")
	f.StringVar(&flagPassword, "password", "", "shared password clients authenticate with (mutually exclusive with --password-file)")
	f.StringVar(&flagPasswordFile, "password-file", "", "file containing the shared password (recommended over --password to avoid shell-history/process-list leakage)")
	f.StringVar(&flagAdminPass, "admin-password", "", "admin password required to revert the workspace to an earlier change or bulk-delete files (both are disabled if unset; mutually exclusive with --admin-password-file)")
	f.StringVar(&flagAdminFile, "admin-password-file", "", "file containing the admin password (recommended over --admin-password to avoid shell-history/process-list leakage)")
	f.StringVar(&flagDB, "db", "./rex-svr.db", "path to the server's SQLite database file")
	f.StringVar(&flagWorkspace, "workspace", "./rex-svr-git-workspace", "path to the server-owned git working copy (must never be a directory a client syncs into)")
	f.StringVar(&flagLogLevel, "log-level", "info", "log level (info, debug)")
	f.StringVar(&flagCert, "cert", "./rex-svr.crt", "TLS certificate file (auto-generated as a self-signed cert if it and --key don't already exist)")
	f.StringVar(&flagKey, "key", "./rex-svr.key", "TLS private key file (auto-generated alongside --cert if missing)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// readSecret resolves a --<name>/--<name>-file flag pair; "" if neither is set.
func readSecret(name, value, file string) (string, error) {
	if value != "" && file != "" {
		return "", fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name)
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --%s-file: %w", name, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return value, nil
}

func resolvePassword() (string, error) {
	pw, err := readSecret("password", flagPassword, flagPasswordFile)
	if err == nil && pw == "" {
		err = fmt.Errorf("one of --password or --password-file is required")
	}
	return pw, err
}

func runServer(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	password, err := resolvePassword()
	if err != nil {
		return err
	}

	// 1. Locate/validate the git executable. The server must never accept
	// sync traffic without a working git materialization path.
	gitVersion, err := gitexec.Version()
	if err != nil {
		return fmt.Errorf("rex-svr requires git to be installed and on PATH: %w", err)
	}
	fmt.Fprintf(os.Stdout, "git: %s\n", gitVersion)

	// Before anything is created on disk: the server's own state must never
	// live inside the workspace it owns (issue #62, `--workspace .`).
	if err := syncserver.CheckWorkspaceLayout(flagWorkspace, flagDB, flagCert, flagKey); err != nil {
		return err
	}

	// 2. Open/create the canonical database and run migrations.
	db, err := syncserver.OpenDB(flagDB)
	if err != nil {
		return fmt.Errorf("open database %s: %w", flagDB, err)
	}
	defer db.Close()

	passwordHash, err := syncserver.HashPassword(password)
	if err != nil {
		return err
	}

	adminPass, err := readSecret("admin-password", flagAdminPass, flagAdminFile)
	if err != nil {
		return err
	}
	var adminHash string
	if adminPass != "" {
		if adminHash, err = syncserver.HashPassword(adminPass); err != nil {
			return err
		}
	}

	// 4. Open-or-init the server-owned git workspace (repo-local identity,
	// never global git config).
	store := syncserver.NewStore(db)
	materializer, err := syncserver.NewGitMaterializer(flagWorkspace, db, store)
	if err != nil {
		return fmt.Errorf("initialize git workspace %s: %w", flagWorkspace, err)
	}

	// 5-6. Drift check: the DB is always authoritative, so any mismatch
	// between canonical state and the git working tree is repaired from
	// the DB, never the other way around.
	repaired, err := materializer.DriftCheckAndRepair(ctx)
	if err != nil {
		return fmt.Errorf("startup drift check/repair: %w", err)
	}
	if repaired > 0 {
		fmt.Fprintf(os.Stdout, "startup drift repair: %d path(s) re-materialized from database\n", repaired)
	}

	// 7. Drain any materializations that were accepted into the DB but
	// never committed to git before a prior crash/restart.
	materializer.Start()

	notifier := syncserver.NewWSHub()

	fingerprint, err := syncserver.EnsureSelfSignedCert(flagCert, flagKey)
	if err != nil {
		return fmt.Errorf("prepare TLS certificate: %w", err)
	}
	fmt.Fprintf(os.Stdout, "TLS certificate: %s (sha256 fingerprint %s)\n", flagCert, fingerprint)
	fmt.Fprintf(os.Stdout, "Share %s with clients via `--ca-cert %s` to trust this server.\n", flagCert, flagCert)

	cfg := syncserver.Config{
		Addr:              fmt.Sprintf(":%d", flagPort),
		PasswordHash:      passwordHash,
		AdminPasswordHash: adminHash,
		CertFile:          flagCert,
		KeyFile:           flagKey,
	}
	srv := syncserver.NewServer(cfg, db, materializer, notifier)

	dbOK, gitOK, pending := srv.HealthStatus(ctx)
	fmt.Fprintf(os.Stdout, "rex-svr %s ready: db_ok=%v git_ok=%v pending_materializations=%d workspace=%s\n",
		version.Version, dbOK, gitOK, pending, flagWorkspace)

	return srv.ListenAndServe()
}
