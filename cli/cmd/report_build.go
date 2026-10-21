package cmd

import (
	"fmt"
	"os"

	"github.com/shriyanss/report-rex/cli/internal/buildscript"
	"github.com/shriyanss/report-rex/cli/internal/docker"
	"github.com/shriyanss/report-rex/cli/internal/version"
	"github.com/spf13/cobra"
)

var reportBuildFlags buildscript.BuildFlags

var reportBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a report from the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !reportBuildFlags.Force {
			_, result, err := runValidation(reportBuildFlags.Dir, false, nil)
			if err != nil {
				return err
			}
			if !result.OK() {
				fmt.Fprintf(os.Stderr, "\nBuild aborted: %d validation error(s). Fix them, or pass --force to build anyway.\n", len(result.Errors()))
				os.Exit(1)
			}
		}
		return docker.RunReportBuild(reportBuildFlags)
	},
}

func init() {
	f := reportBuildCmd.Flags()
	f.StringVar(&reportBuildFlags.Dir, "dir", ".", "report directory")
	f.StringVar(&reportBuildFlags.Template, "template", "", "template override (defaults to index.md's template)")
	f.StringVar(&reportBuildFlags.Output, "output", "report.pdf", "output PDF path")
	f.BoolVar(&reportBuildFlags.Final, "final", false, "produce the final report, no DRAFT watermark (default: draft, renamed to *-draft.pdf)")
	f.BoolVar(&reportBuildFlags.Docx, "docx", false, "also generate a .docx clone alongside the PDF")
	f.BoolVar(&reportBuildFlags.Force, "force", false, "skip QA-gate validation")
	f.BoolVar(&reportBuildFlags.NoDocker, "no-docker", false, "invoke the local report-rex Python CLI instead of Docker")
	f.StringVar(&reportBuildFlags.Image, "image", "report-rex:"+version.Version, "Docker image tag to build/run")
	f.BoolVar(&reportBuildFlags.NoTerminalBS, "no-terminal-bs", false, "suppress the build progress indicator")
	f.StringVar(&reportBuildFlags.Password, "password", "", "encrypt the output PDF with this password (requires qpdf; --no-docker needs it on PATH)")

	reportCmd.AddCommand(reportBuildCmd)
}
