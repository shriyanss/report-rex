// Package gitexec wraps the system `git` executable via os/exec, mirroring
// internal/docker's shell-out pattern: no SDK dependency, small binary,
// CGO-free, and (per the sync design) meaningful because the server is
// required to verify a real `git --version` on startup.
package gitexec

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Runner executes git commands against a fixed working directory. It is an
// interface so callers (e.g. drift-detection logic) can substitute a fake
// in unit tests without invoking a real git binary.
type Runner interface {
	// Run executes `git <args...>` in the runner's working directory and
	// returns combined stdout. Non-zero exit is returned as an error
	// including stderr.
	Run(args ...string) (string, error)
	// RunWithEnv is like Run but with additional environment variables
	// appended to the subprocess environment (e.g. GIT_AUTHOR_NAME).
	RunWithEnv(env []string, args ...string) (string, error)
}

// execRunner is the real Runner backed by the system git binary.
type execRunner struct {
	dir string
}

// New returns a Runner that executes git commands in dir.
func New(dir string) Runner {
	return &execRunner{dir: dir}
}

func (r *execRunner) Run(args ...string) (string, error) {
	return r.RunWithEnv(nil, args...)
}

func (r *execRunner) RunWithEnv(env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	if env != nil {
		cmd.Env = append(cmd.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// git writes some diagnostic messages (e.g. "nothing to commit") to
		// stdout rather than stderr, so include both in the error.
		return stdout.String(), fmt.Errorf("git %v: %w: %s%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String(), nil
}

// Version returns the output of `git --version`, or an error if git is not
// on PATH or fails to execute.
func Version() (string, error) {
	out, err := exec.Command("git", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git --version failed: %w: %s", err, string(out))
	}
	return string(bytes.TrimSpace(out)), nil
}
