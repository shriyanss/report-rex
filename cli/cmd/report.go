package cmd

import "github.com/spf13/cobra"

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Build or scaffold a report",
}

func init() {
	rootCmd.AddCommand(reportCmd)
}
