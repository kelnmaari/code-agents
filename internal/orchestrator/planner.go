package orchestrator

import (
	"context"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runner"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

const terminationSignal = "CODEAGENTS_DONE"

// initPlanner initializes a fresh planner agent.
func (o *Orchestrator) initPlanner() *agent.Agent {
	plannerID, _ := gonanoid.New()

	// Planner gets create_task + reconnaissance tools (list_dir, read_file) + verification (shell_exec)
	plannerTools := tool.NewRegistry()
	plannerTools.Register(tool.NewCreateTask(o.queue, plannerID, 0))
	plannerTools.Register(tool.NewListDir(o.cfg.Tools.WorkDir))
	plannerTools.Register(tool.NewReadFile(o.cfg.Tools.WorkDir))

	r := runner.NewRealRunner(o.cfg.Tools.WorkDir)
	plannerTools.Register(tool.NewShellExec(r, o.cfg.Tools.AllowedShell))

	return agent.New(
		plannerID,
		agent.RolePlanner,
		o.client,
		o.cfg.Agents.Planner.Model,
		o.cfg.Agents.Planner.SystemPrompt,
		plannerTools,
	)
}

// runPlanner runs the planner agent loop for one reassessment cycle.
// Returns (done, error). done=true means the mission is complete.
func (o *Orchestrator) runPlanner(ctx context.Context, step int) (bool, error) {
	o.ensurePlanner()

	logging.Console.Printf("[planner] reassessing (step %d/%d)", step, o.cfg.Loop.MaxSteps)
	logging.File.Printf("[planner] reassessing (step %d/%d)", step, o.cfg.Loop.MaxSteps)

	// Inject status update before each step. Prune old ones to keep history clean.
	status := o.buildStatusMessage(o.planner.ID())
	o.planner.PruneMessagesByPrefix("=== STATUS UPDATE ===")
	o.planner.AddUserMessage(status)

	result := o.planner.Step(ctx)
	o.reportUsage(result.Usage)
	if result.Error != nil {
		return false, fmt.Errorf("planner step %d: %w", step, result.Error)
	}

	logging.File.Printf("[planner] output: %s (tools: %d)", truncateLog(result.Output, 300), result.ToolCallsCount)

	// Check for termination signal
	if strings.Contains(result.Output, terminationSignal) {
		if o.queue.AllDone() {
			logging.Console.Println("[planner] ✓ CODEAGENTS_DONE")
			logging.File.Println("[planner] received CODEAGENTS_DONE")
			return true, nil
		}

		// Reject premature "Done" signal
		logging.File.Println("[planner] premature CODEAGENTS_DONE, nudging")
		logging.Console.Println("[planner] ! premature CODEAGENTS_DONE rejected (tasks still in progress)")
		o.planner.AddUserMessage(fmt.Sprintf(
			"You said '%s', but the mission is NOT complete. There are still tasks in progress or pending. "+
				"You MUST wait for all tasks to be 'completed' or 'failed', review their handoffs, and verify the final result via shell_exec before finishing. "+
				"Current Status: %d pending, %d assigned/busy.",
			terminationSignal, o.queue.PendingCount(), o.queue.AssignedCount()))
		return false, nil
	}

	// YIELD LOGIC:
	// We always yield to workers after a planner step unless it's done.
	// This ensures workers can pick up potentially new tasks.
	if result.ToolCallsCount == 0 {
		logging.File.Println("[planner] no tools called, yielding to workers")
	} else {
		logging.File.Printf("[planner] %d tools called, yielding for execution", result.ToolCallsCount)
	}

	return false, nil
}

func truncateLog(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
