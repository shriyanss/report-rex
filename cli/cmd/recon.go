package cmd

import (
	"fmt"

	"github.com/shriyanss/report-rex/cli/internal/ingest"
	"github.com/spf13/cobra"
)

var (
	reconDir   string
	reconHosts []string
	reconCIDRs []string
)

var reconCmd = &cobra.Command{
	Use:   "recon",
	Short: "Run a full recon sweep (nmap, nikto, nuclei) and fold results into the report",
	Long: `Orchestrate a recon sweep against one or more targets and ingest everything
into the report's appendices and Timeline:

  1. a two-phase nmap scan — a fast all-port discovery pass, then
     service/version detection + default scripts against only the open ports;
  2. nikto against each discovered web app;
  3. nuclei against the discovered web apps.

nmap, nikto, and nuclei must all be installed (recon fails fast if any is
missing). Results accumulate in artifacts/*.json and the generated appendices,
and every tool run is stamped into the report's Timeline.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targets := append(append([]string{}, reconHosts...), reconCIDRs...)
		if len(targets) == 0 {
			return fmt.Errorf("pass at least one --host or --cidr")
		}
		return ingest.RunRecon(reconDir, targets)
	},
}

func init() {
	reconCmd.Flags().StringVar(&reconDir, "dir", ".", "report directory")
	reconCmd.Flags().StringSliceVar(&reconHosts, "host", nil, "target host (repeatable, or comma-separated)")
	reconCmd.Flags().StringSliceVar(&reconCIDRs, "cidr", nil, "target CIDR range (repeatable, or comma-separated)")
	rootCmd.AddCommand(reconCmd)
}
