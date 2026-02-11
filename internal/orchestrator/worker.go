package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
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

		log.Printf("[worker-%s] picked up task: %s - %s", workerID[:6], t.ID[:6], t.Title)

		// Determine if this should be a subplanner
		if t.IsSubplan && t.Depth < o.cfg.Loop.MaxDepth {
			if err := o.runSubplanner(ctx, t); err != nil {
				log.Printf("[worker-%s] subplanner error for task %s: %v", workerID[:6], t.ID[:6], err)
				o.queue.Fail(t.ID, err.Error())
			}
			continue
		}

		// Execute as regular worker task with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[worker-%s] panic: %v", workerID[:6], r)
					o.queue.Fail(t.ID, fmt.Sprintf("panic: %v", r))
				}
			}()

			if err := o.executeWorkerTask(ctx, t); err != nil {
				log.Printf("[worker-%s] task %s error: %v", workerID[:6], t.ID[:6], err)
				o.queue.Fail(t.ID, err.Error())
			}
		}()
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
	worker := agent.New(
		agentID,
		agent.RoleWorker,
		o.client,
		o.cfg.Agents.Worker.Model,
		o.cfg.Agents.Worker.SystemPrompt,
		workerTools,
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
		if result.Error != nil {
			return fmt.Errorf("worker step %d: %w", i+1, result.Error)
		}

		// Check if complete_task was called (task is now completed in queue)
		if o.queue.IsTaskCompleted(t.ID) {
			log.Printf("[worker-%s] task %s completed via handoff", agentID[:6], t.ID[:6])
			return nil
		}

		// If agent stopped without completing, give it one more chance
		if result.Stop {
			if o.queue.IsTaskCompleted(t.ID) {
				log.Printf("[worker-%s] task %s completed via handoff", agentID[:6], t.ID[:6])
				return nil
			}
			log.Printf("[worker-%s] task %s: agent stopped without complete_task, prompting to finalize", agentID[:6], t.ID[:6])
			worker.AddUserMessage("You have finished your work but did not call complete_task. You MUST call complete_task NOW with a summary of your findings and changes. Do not write any text — just call the tool.")
			finalResult := worker.Step(ctx)
			if finalResult.Error != nil {
				return fmt.Errorf("worker final step: %w", finalResult.Error)
			}
			if o.queue.IsTaskCompleted(t.ID) {
				log.Printf("[worker-%s] task %s completed via handoff (after prompt)", agentID[:6], t.ID[:6])
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
