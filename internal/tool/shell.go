package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runner"
)

// ShellExecTool executes shell commands via the runner.
type ShellExecTool struct {
	runner       runner.AgentRunner
	allowedShell []string
}

// NewShellExec creates a shell execution tool with an optional command whitelist.
func NewShellExec(r runner.AgentRunner, allowedShell []string) *ShellExecTool {
	return &ShellExecTool{runner: r, allowedShell: allowedShell}
}

func (t *ShellExecTool) Name() string        { return "shell_exec" }
func (t *ShellExecTool) Description() string  { return "Execute a shell command and return stdout+stderr output" }
func (t *ShellExecTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellExecTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	if len(t.allowedShell) > 0 && !isAllowed(params.Command, t.allowedShell) {
		return fmt.Sprintf("Error: command not allowed. Allowed commands: %s", strings.Join(t.allowedShell, ", ")), nil
	}

	output, err := t.runner.Run(params.Command, nil)
	if err != nil {
		// Return output + error as text so LLM can see it
		return fmt.Sprintf("Exit error: %s\n%s", err, output), nil
	}

	return output, nil
}

// isAllowed checks if the first word of the command is in the whitelist.
func isAllowed(command string, allowed []string) bool {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	cmd := filepath.Base(parts[0])
	for _, a := range allowed {
		if cmd == a {
			return true
		}
	}
	return false
}
