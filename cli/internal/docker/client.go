// Package docker shells out to the `docker` CLI binary the same way build.sh does —
// no Docker SDK dependency, keeping the report-rex binary small and CGO-free.
package docker

import (
	"fmt"
	"os"
	"os/exec"
)

// Preflight verifies a working `docker` CLI and daemon are available, returning a
// clear, actionable error if not.
func Preflight() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH: install Docker Desktop or the Docker Engine CLI")
	}
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker daemon not reachable: is Docker running? (%w)", err)
	}
	return nil
}

// run executes a docker subcommand, streaming stdout/stderr to the current process.
func run(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
