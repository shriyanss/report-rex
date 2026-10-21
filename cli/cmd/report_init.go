package cmd

import (
	"fmt"
	"strings"

	"github.com/shriyanss/report-rex/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

var (
	reportInitDir    string
	reportInitType   string
	reportInitKind   string
	reportInitTitle  string
	reportInitClient string
	reportInitDate   string
	reportInitForce  bool
)

var reportInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a new report from an example template",
	RunE: func(cmd *cobra.Command, args []string) error {
		var missing []string
		if reportInitTitle == "" {
			missing = append(missing, "--title")
		}
		if reportInitClient == "" {
			missing = append(missing, "--client")
		}
		if reportInitDate == "" {
			missing = append(missing, "--date")
		}
		if len(missing) > 0 && !reportInitForce {
			return fmt.Errorf("missing %s (or pass --force to scaffold with placeholder values)", strings.Join(missing, ", "))
		}

		meta := scaffold.Meta{Title: reportInitTitle, Client: reportInitClient, Date: reportInitDate}
		if err := scaffold.InitReport(reportInitDir, reportInitType, reportInitKind, meta); err != nil {
			return err
		}
		fmt.Printf("Initialized %s report in %s\n", reportInitType, reportInitDir)
		if len(missing) > 0 {
			fmt.Printf("Warning: using placeholder values for %s — replace before building a real report\n", strings.Join(missing, ", "))
		}
		return nil
	},
}

func init() {
	f := reportInitCmd.Flags()
	f.StringVar(&reportInitDir, "dir", ".", "target directory")
	f.StringVar(&reportInitType, "type", scaffold.TypeFullReport, "report type: full-report or quick-comm")
	f.StringVar(&reportInitKind, "kind", "", "quick-comm kind: critical-finding or progress-update (default progress-update)")
	f.StringVar(&reportInitTitle, "title", "", "engagement title (required unless --force)")
	f.StringVar(&reportInitClient, "client", "", "client name (required unless --force)")
	f.StringVar(&reportInitDate, "date", "", "engagement date, YYYY-MM-DD (required unless --force)")
	f.BoolVar(&reportInitForce, "force", false, "scaffold with placeholder title/client/date instead of requiring them")

	reportCmd.AddCommand(reportInitCmd)
}
