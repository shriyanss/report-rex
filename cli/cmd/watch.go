package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/shriyanss/report-rex/cli/internal/buildscript"
	"github.com/shriyanss/report-rex/cli/internal/docker"
	"github.com/shriyanss/report-rex/cli/internal/version"
	"github.com/shriyanss/report-rex/cli/internal/watcher"
	"github.com/spf13/cobra"
)

var watchFlags buildscript.BuildFlags

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch a report directory and rebuild on change",
	RunE: func(cmd *cobra.Command, args []string) error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		stop := make(chan struct{})
		go func() {
			<-sigCh
			close(stop)
		}()

		build := func() error {
			return docker.RunReportBuild(watchFlags)
		}
		return watcher.Watch(watchFlags.Dir, build, stop)
	},
}

func init() {
	f := watchCmd.Flags()
	f.StringVar(&watchFlags.Dir, "dir", ".", "report directory to watch")
	f.StringVar(&watchFlags.Template, "template", "", "template override (defaults to index.md's template)")
	f.StringVar(&watchFlags.Output, "output", "report.pdf", "output PDF path")
	f.BoolVar(&watchFlags.Final, "final", false, "produce the final report, no DRAFT watermark (default: draft, renamed to *-draft.pdf)")
	f.BoolVar(&watchFlags.Docx, "docx", false, "also generate a .docx clone alongside the PDF")
	f.BoolVar(&watchFlags.Force, "force", false, "skip QA-gate validation")
	f.BoolVar(&watchFlags.NoDocker, "no-docker", false, "invoke the local report-rex Python CLI instead of Docker")
	f.StringVar(&watchFlags.Image, "image", "report-rex:"+version.Version, "Docker image tag to build/run")
	f.BoolVar(&watchFlags.NoTerminalBS, "no-terminal-bs", false, "suppress the build progress indicator")
	f.StringVar(&watchFlags.Password, "password", "", "encrypt the output PDF with this password (requires qpdf; --no-docker needs it on PATH)")

	rootCmd.AddCommand(watchCmd)
}
