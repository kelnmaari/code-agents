//go:build windows

package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Run executes a command using cmd.exe on Windows with standard pipes.
func (r *RealRunner) Run(command string, env map[string]string) (string, error) {
	cmd := exec.Command("cmd", "/C", command)
	cmd.Dir = r.WorkDir

	// Build environment
	currentEnv := os.Environ()
	newEnv := make([]string, 0, len(currentEnv)+len(env))
	newEnv = append(newEnv, currentEnv...)
	for k, v := range env {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = newEnv

	// Capture output
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		return buf.String(), err
	}

	return buf.String(), nil
}
