package cmd

import (
	"fmt"

	"github.com/shriyanss/report-rex/cli/internal/docker"
	"github.com/shriyanss/report-rex/cli/internal/version"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage the report-rex Docker image",
}

var imageBuildTag string

var imageBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the report-rex Docker image from embedded source",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := docker.Preflight(); err != nil {
			return err
		}
		if err := docker.BuildImage(imageBuildTag); err != nil {
			return err
		}
		fmt.Printf("Built image %q\n", imageBuildTag)
		return nil
	},
}

func init() {
	imageBuildCmd.Flags().StringVar(&imageBuildTag, "tag", "report-rex:"+version.Version, "tag to apply to the built image")
	imageCmd.AddCommand(imageBuildCmd)
	rootCmd.AddCommand(imageCmd)
}
