package cmd

import (
	"fmt"

	"github.com/shriyanss/report-rex/cli/internal/docker"
	"github.com/shriyanss/report-rex/cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	ocrLang     string
	ocrImage    string
	ocrNoDocker bool
)

var ocrCmd = &cobra.Command{
	Use:   "ocr <image>",
	Short: "Extract the text from a screenshot (code, HTTP request/response) as a Markdown code block",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := docker.OCR(ocrImage, args[0], ocrNoDocker)
		if err != nil {
			return err
		}
		fmt.Print(docker.FormatOCR(raw, ocrLang))
		return nil
	},
}

func init() {
	ocrCmd.Flags().StringVar(&ocrLang, "lang", "text", "language tag for the code fence (e.g. http, python)")
	ocrCmd.Flags().StringVar(&ocrImage, "image", "report-rex:"+version.Version, "Docker image tag to build/run")
	ocrCmd.Flags().BoolVar(&ocrNoDocker, "no-docker", false, "use a local tesseract install instead of Docker")
	rootCmd.AddCommand(ocrCmd)
}
