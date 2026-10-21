package cmd

import (
	"fmt"

	"github.com/shriyanss/report-rex/cli/internal/ingest"
	"github.com/spf13/cobra"
)

var ingestDir string

var ingestCmd = &cobra.Command{
	Use:   "ingest <tool> [tool-args...]",
	Short: "Import scan tool output into the report's appendices",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "nmap":
			return ingest.RunNmap(ingestDir, args[1:])
		case "naabu":
			return ingest.RunNaabu(ingestDir, args[1:])
		case "nikto":
			return ingest.RunNikto(ingestDir, args[1:])
		case "bloodhound":
			return ingest.RunBloodHound(ingestDir, args[1:])
		default:
			return fmt.Errorf("unsupported ingest tool %q (supported: nmap, naabu, nikto, bloodhound)", args[0])
		}
	},
}

var ingestExistingCmd = &cobra.Command{
	Use:   "existing <tool> [tool-args...]",
	Short: "Import a scan tool's output you already have, without re-running the tool",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "nmap":
			return ingest.RunNmapExisting(ingestDir, args[1:])
		case "naabu":
			return ingest.RunNaabuExisting(ingestDir, args[1:])
		default:
			return fmt.Errorf("unsupported ingest-existing tool %q (supported: naabu, nmap)", args[0])
		}
	},
}

func init() {
	// Shared with ingestExistingCmd (a persistent flag so the subcommand inherits it).
	ingestCmd.PersistentFlags().StringVar(&ingestDir, "dir", ".", "report directory")
	// Tool args (e.g. nmap's own -sV) look like flags but aren't ours — stop parsing
	// rex's own flags once the tool name is reached, so `rex ingest --dir d nmap -sV
	// host` passes "-sV" and "host" through untouched instead of erroring as an
	// unknown rex flag.
	ingestCmd.Flags().SetInterspersed(false)
	ingestExistingCmd.Flags().SetInterspersed(false)
	ingestCmd.AddCommand(ingestExistingCmd)
	rootCmd.AddCommand(ingestCmd)
}
