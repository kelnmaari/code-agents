//go:build !windows

package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// Run executes a shell command in a pseudo-terminal on Unix systems.
// Streams output to os.Stdout and returns the full captured output.
func (r *RealRunner) Run(command string, env map[string]string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = r.WorkDir

	// Build environment
	currentEnv := os.Environ()
	newEnv := make([]string, 0, len(currentEnv)+len(env))
	newEnv = append(newEnv, currentEnv...)
	for k, v := range env {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = newEnv

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start pty: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Capture output while streaming
	var buf bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &buf)
	_, _ = io.Copy(mw, ptmx)

	err = cmd.Wait()
	if err != nil {
		return buf.String(), err
	}

	return buf.String(), nil
}
