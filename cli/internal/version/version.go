// Package version holds the report-rex CLI's release version.
package version

// Version is overridden at build time via:
//
//	-ldflags "-X github.com/shriyanss/report-rex/cli/internal/version.Version=$(cat version.txt)"
var Version = "dev"
