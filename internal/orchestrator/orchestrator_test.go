package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
)

func testConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Provider: config.ProviderConfig{
			BaseURL: "http://localhost:8080/v1",
			APIKey:  "test",
		},
		Agents: config.AgentsConfig{
			Planner: config.AgentConfig{
				Model:        config.ModelConfig{Model: "test", Temperature: 0.3, MaxTokens: 4096},
				SystemPrompt: "You are a planner.",
			},
			Subplanner: config.AgentConfig{
				Model:        config.ModelConfig{Model: "test", Temperature: 0.3, MaxTokens: 4096},
				SystemPrompt: "You are a subplanner.",
			},
			Worker: config.AgentConfig{
				Model:        config.ModelConfig{Model: "test", Temperature: 0.2, MaxTokens: 8192},
				SystemPrompt: "You are a worker.",
			},
		},
		Loop: config.LoopConfig{
			MaxDepth:          3,
			MaxWorkers:        2,
			MaxSteps:          10,
			Timeout:           "5m",
			StepDelay:         "1s",
			TimeoutDuration:   5 * time.Minute,
			StepDelayDuration: 1 * time.Second,
		},
		Tools: config.ToolsConfig{
			WorkDir:    ".",
			GitEnabled: false,
		},
		Input: config.InputConfig{
			Prompt: "Test prompt",
		},
	}
}

func TestBuildStatusMessage_Empty(t *testing.T) {
	cfg := testConfig()
	orch := NewWithClient(cfg, nil)

	msg := orch.buildStatusMessage("parent-1")
	require.Contains(t, msg, "STATUS UPDATE")
	require.Contains(t, msg, "0 pending")
	require.Contains(t, msg, "0 completed")
	require.Contains(t, msg, "0 failed")
}

func TestBuildStatusMessage_WithTasks(t *testing.T) {
	cfg := testConfig()
	orch := NewWithClient(cfg, nil)

	// Push and complete a task (pre-approved so Pull doesn't block forever)
	orch.queue.Push(&task.Task{
		ID:        "task-1",
		ParentID:  "parent-1",
		Title:     "First task",
		CreatedAt: time.Now(),
		Approved:  true,
	})

	// Pull to assign (use a cancellable context to avoid goroutine leak)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pulled := make(chan *task.Task, 1)
	go func() {
		pulled <- orch.queue.Pull(ctx)
	}()
	// Give queue time to respond
	time.Sleep(10 * time.Millisecond)

	msg := orch.buildStatusMessage("parent-1")
	require.Contains(t, msg, "STATUS UPDATE")
}

func TestBuildStatusMessage_WithFailedTasks(t *testing.T) {
	cfg := testConfig()
	orch := NewWithClient(cfg, nil)

	orch.queue.Push(&task.Task{
		ID:        "task-f1",
		ParentID:  "parent-1",
		Title:     "Failing task",
		CreatedAt: time.Now(),
	})

	orch.queue.Fail("task-f1", "something went wrong")

	msg := orch.buildStatusMessage("parent-1")
	require.Contains(t, msg, "[failed]")
	require.Contains(t, msg, "something went wrong")
	require.Contains(t, msg, "1 failed")
}

func TestBuildStatusMessage_WithHandoffs(t *testing.T) {
	cfg := testConfig()
	orch := NewWithClient(cfg, nil)

	orch.queue.Push(&task.Task{
		ID:        "task-h1",
		ParentID:  "parent-1",
		Title:     "Done task",
		CreatedAt: time.Now(),
	})

	orch.queue.Complete("task-h1", &task.Handoff{
		TaskID:       "task-h1",
		AgentID:      "worker-1",
		Summary:      "Implemented feature X",
		Findings:     []string{"Found a bug"},
		Concerns:     []string{"Performance may degrade"},
		FilesChanged: []string{"main.go", "config.go"},
	})

	msg := orch.buildStatusMessage("parent-1")
	require.Contains(t, msg, "1 completed")
	require.Contains(t, msg, "Recent handoffs")
	require.Contains(t, msg, "Implemented feature X")
	require.Contains(t, msg, "main.go")
}

func TestFormatTaskMessage(t *testing.T) {
	tk := &task.Task{
		ID:          "task-123",
		Title:       "Write tests",
		Description: "Write unit tests for the config package",
		Scope:       "internal/config/",
		Constraints: []string{"Use testify", "No mocks"},
	}

	msg := formatTaskMessage(tk)
	require.Contains(t, msg, "TASK")
	require.Contains(t, msg, "task-123")
	require.Contains(t, msg, "Write tests")
	require.Contains(t, msg, "Write unit tests for the config package")
	require.Contains(t, msg, "internal/config/")
	require.Contains(t, msg, "Use testify")
	require.Contains(t, msg, "No mocks")
}

func TestFormatSubplannerMessage(t *testing.T) {
	tk := &task.Task{
		ID:          "task-456",
		Title:       "Refactor auth",
		Description: "Refactor the authentication module",
		Scope:       "internal/auth/",
		Depth:       2,
	}

	msg := formatSubplannerMessage(tk)
	require.Contains(t, msg, "SUBPLAN TASK")
	require.Contains(t, msg, "task-456")
	require.Contains(t, msg, "Depth: 2")
	require.Contains(t, msg, "create_task")
	require.Contains(t, msg, "submit_handoff")
}

func TestTruncateLog(t *testing.T) {
	require.Equal(t, "hello", truncateLog("hello", 100))
	require.Equal(t, "hello world", truncateLog("hello\nworld", 100))

	long := strings.Repeat("a", 200)
	result := truncateLog(long, 50)
	require.Len(t, result, 53) // 50 + "..."
	require.True(t, strings.HasSuffix(result, "..."))
}
