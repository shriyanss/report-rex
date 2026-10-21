// Package cmd wires up the report-rex CLI's Cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/shriyanss/report-rex/cli/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "rex",
	Short:   "Standalone CLI for building report-rex pentest reports",
	Version: version.Version,
}

// Execute runs the root command, exiting the process on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
