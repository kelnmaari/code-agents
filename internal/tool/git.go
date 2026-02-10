package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runner"
)

// --- GitStatusTool ---

type GitStatusTool struct {
	runner runner.AgentRunner
}

func NewGitStatus(r runner.AgentRunner) *GitStatusTool {
	return &GitStatusTool{runner: r}
}

func (t *GitStatusTool) Name() string        { return "git_status" }
func (t *GitStatusTool) Description() string { return "Run git status --porcelain" }
func (t *GitStatusTool) Parameters() interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *GitStatusTool) Execute(_ context.Context, _ string) (string, error) {
	output, err := t.runner.Run("git status --porcelain", nil)
	if err != nil {
		return fmt.Sprintf("Error: %s\n%s", err, output), nil
	}
	return output, nil
}

// --- GitDiffTool ---

type GitDiffTool struct {
	runner runner.AgentRunner
}

func NewGitDiff(r runner.AgentRunner) *GitDiffTool {
	return &GitDiffTool{runner: r}
}

func (t *GitDiffTool) Name() string        { return "git_diff" }
func (t *GitDiffTool) Description() string { return "Run git diff to show changes" }
func (t *GitDiffTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"staged": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, show staged changes (--cached). Default: false",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Optional path filter",
			},
		},
	}
}

func (t *GitDiffTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Staged bool   `json:"staged"`
		Path   string `json:"path"`
	}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return fmt.Sprintf("Error parsing arguments: %s", err), nil
		}
	}

	cmd := "git diff"
	if params.Staged {
		cmd += " --cached"
	}
	if params.Path != "" {
		cmd += " -- " + params.Path
	}

	output, err := t.runner.Run(cmd, nil)
	if err != nil {
		return fmt.Sprintf("Error: %s\n%s", err, output), nil
	}
	return output, nil
}

// --- GitCommitTool ---

type GitCommitTool struct {
	runner runner.AgentRunner
}

func NewGitCommit(r runner.AgentRunner) *GitCommitTool {
	return &GitCommitTool{runner: r}
}

func (t *GitCommitTool) Name() string { return "git_commit" }
func (t *GitCommitTool) Description() string {
	return "Stage files and create a git commit. Requires a commit message string."
}
func (t *GitCommitTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The commit message text, e.g. 'fix: resolve null pointer in handler'. This is a single string, NOT an array.",
			},
			"files": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional list of file paths to stage, e.g. [\"src/main.go\", \"README.md\"]. If empty or omitted, all modified files are staged (git add -A).",
			},
		},
		"required": []string{"message"},
	}
}

func (t *GitCommitTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Message string   `json:"message"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	if params.Message == "" {
		return "Error: commit message is required and cannot be empty", nil
	}

	// Stage files — quote each path individually
	var addCmd string
	if len(params.Files) > 0 {
		quoted := make([]string, len(params.Files))
		for i, f := range params.Files {
			quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(f, "'", "'\"'\"'"))
		}
		addCmd = "git add " + strings.Join(quoted, " ")
	} else {
		addCmd = "git add -A"
	}

	output, err := t.runner.Run(addCmd, nil)
	if err != nil {
		return fmt.Sprintf("Error staging: %s\n%s", err, output), nil
	}

	// Commit (escape message for shell)
	escapedMsg := strings.ReplaceAll(params.Message, "'", "'\"'\"'")
	commitCmd := fmt.Sprintf("git commit -m '%s'", escapedMsg)

	commitOutput, err := t.runner.Run(commitCmd, nil)
	if err != nil {
		return fmt.Sprintf("Error committing: %s\n%s", err, commitOutput), nil
	}

	return output + commitOutput, nil
}
