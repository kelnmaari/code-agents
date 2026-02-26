package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runner"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

// runWorker runs a worker goroutine that pulls tasks from the queue and executes them.
func (o *Orchestrator) runWorker(ctx context.Context, workerID string) {
	for {
		t := o.queue.Pull(ctx)
		if t == nil {
			return // context cancelled
		}

		// Initialise MaxRetries from config if the task has no override.
		if t.MaxRetries == 0 {
			t.MaxRetries = o.cfg.Loop.MaxRetries
		}

		logging.Console.Printf("[worker-%s] ▶ task: %s", workerID[:6], t.Title)
		logging.File.Printf("[worker-%s] picked up task: %s - %s (retry %d/%d)",
			workerID[:6], t.ID[:6], t.Title, t.RetryCount, t.MaxRetries)

		// Determine if this should be a subplanner
		if t.IsSubplan && t.Depth < o.cfg.Loop.MaxDepth {
			if err := o.runSubplanner(ctx, t); err != nil {
				logging.File.Printf("[worker-%s] subplanner error for task %s: %v", workerID[:6], t.ID[:6], err)
				o.retryOrFail(t, err.Error())
			}
			continue
		}

		// Execute as regular worker task with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicMsg := fmt.Sprintf("panic: %v", r)
					logging.File.Printf("[worker-%s] %s", workerID[:6], panicMsg)
					logging.Console.Printf("[worker-%s] ✗ recovered from panic", workerID[:6])
					o.retryOrFail(t, panicMsg)
				}
			}()

			if err := o.executeWorkerTask(ctx, t); err != nil {
				logging.Console.Printf("[worker-%s] ✗ error: %v", workerID[:6], err)
				logging.File.Printf("[worker-%s] task %s error: %v", workerID[:6], t.ID[:6], err)
				o.retryOrFail(t, err.Error())
			}
		}()
	}
}

// retryOrFail checks whether a failed task has retries remaining.
// If so, it waits for the configured retry delay and re-queues the task as pending.
// Otherwise it permanently marks the task as failed.
func (o *Orchestrator) retryOrFail(t *task.Task, reason string) {
	maxRetries := t.MaxRetries
	if maxRetries == 0 {
		maxRetries = o.cfg.Loop.MaxRetries
	}

	if t.RetryCount < maxRetries {
		nextAttempt := t.RetryCount + 1
		logging.Console.Printf("[orchestrator] scheduling retry %d/%d for task %s (delay: %s)",
			nextAttempt, maxRetries, t.ID[:6], o.cfg.Loop.RetryDelay)
		logging.File.Printf("[orchestrator] retry %d/%d for task %s — reason: %s",
			nextAttempt, maxRetries, t.ID[:6], reason)
		time.Sleep(o.cfg.Loop.RetryDelayDuration)
		o.queue.RetryTask(t.ID)
	} else {
		logging.Console.Printf("[orchestrator] task %s permanently failed after %d retries",
			t.ID[:6], t.RetryCount)
		logging.File.Printf("[orchestrator] task %s permanently failed after %d retries: %s",
			t.ID[:6], t.RetryCount, reason)
		o.queue.Fail(t.ID, reason)
	}
}

// executeWorkerTask creates a worker agent and executes a single task.
func (o *Orchestrator) executeWorkerTask(ctx context.Context, t *task.Task) error {
	agentID, _ := gonanoid.New()

	// Build worker tool registry
	workerTools := tool.NewRegistry()

	// File tools
	workerTools.Register(tool.NewReadFile(o.cfg.Tools.WorkDir))
	workerTools.Register(tool.NewWriteFile(o.cfg.Tools.WorkDir))
	workerTools.Register(tool.NewEditFile(o.cfg.Tools.WorkDir))
	workerTools.Register(tool.NewReplaceLines(o.cfg.Tools.WorkDir))
	workerTools.Register(tool.NewListDir(o.cfg.Tools.WorkDir))

	// Shell tool
	r := runner.NewRealRunner(o.cfg.Tools.WorkDir)
	workerTools.Register(tool.NewShellExec(r, o.cfg.Tools.AllowedShell))

	// Git tools (if enabled)
	if o.cfg.Tools.GitEnabled {
		workerTools.Register(tool.NewGitStatus(r))
		workerTools.Register(tool.NewGitDiff(r))
		workerTools.Register(tool.NewGitCommit(r))
	}

	// Task completion tool
	workerTools.Register(tool.NewCompleteTask(o.queue, agentID))

	// Create worker agent
	worker := agent.NewWithConfig(
		agentID,
		agent.RoleWorker,
		o.client,
		o.cfg.Agents.Worker.Model,
		o.cfg.Agents.Worker.SystemPrompt,
		workerTools,
		o.cfg.Agents.Worker.MaxHistoryMessages,
	)

	// Inject task description
	taskMsg := formatTaskMessage(t)
	worker.AddUserMessage(taskMsg)

	// Worker execution loop: Step until complete or error
	for i := 0; i < o.cfg.Loop.MaxSteps; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result := worker.Step(ctx)
		o.reportUsage(result.Usage)
		if result.Error != nil {
			return fmt.Errorf("worker step %d: %w", i+1, result.Error)
		}

		// Check if complete_task was called (task is now completed in queue)
		if o.queue.IsTaskCompleted(t.ID) {
			logging.Console.Printf("[worker-%s] ✓ task completed: %s", agentID[:6], t.Title)
			logging.File.Printf("[worker-%s] task %s completed via handoff", agentID[:6], t.ID[:6])
			return nil
		}

		// If agent stopped without completing, give it one more chance
		if result.Stop {
			if o.queue.IsTaskCompleted(t.ID) {
				logging.Console.Printf("[worker-%s] ✓ task completed: %s", agentID[:6], t.Title)
				logging.File.Printf("[worker-%s] task %s completed via handoff", agentID[:6], t.ID[:6])
				return nil
			}
			logging.File.Printf("[worker-%s] task %s: agent stopped without complete_task, prompting to finalize", agentID[:6], t.ID[:6])
			worker.AddUserMessage("You MUST call the 'complete_task' tool to finish this task. Provide a brief summary of what you did. Do NOT just explain in text — you MUST use the tool now to finalize your work.")
			finalResult := worker.Step(ctx)
			if finalResult.Error != nil {
				return fmt.Errorf("worker final step: %w", finalResult.Error)
			}
			if o.queue.IsTaskCompleted(t.ID) {
				logging.Console.Printf("[worker-%s] ✓ task completed: %s", agentID[:6], t.Title)
				logging.File.Printf("[worker-%s] task %s completed via handoff (after prompt)", agentID[:6], t.ID[:6])
				return nil
			}
			return fmt.Errorf("worker did not submit handoff for task %s", t.ID)
		}
	}

	return fmt.Errorf("worker max steps reached for task %s", t.ID)
}

func formatTaskMessage(t *task.Task) string {
	var sb strings.Builder
	sb.WriteString("=== TASK ===\n")
	sb.WriteString(fmt.Sprintf("Task ID: %s\n", t.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n", t.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", t.Description))
	if t.Scope != "" {
		sb.WriteString(fmt.Sprintf("Scope: %s\n", t.Scope))
	}
	if len(t.Constraints) > 0 {
		sb.WriteString("Constraints:\n")
		for _, c := range t.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}
	sb.WriteString("===\n")
	return sb.String()
}
