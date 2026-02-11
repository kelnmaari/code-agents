package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

const terminationSignal = "CODEAGENTS_DONE"

// runPlanner runs the root planner agent loop.
func (o *Orchestrator) runPlanner(ctx context.Context, prompt string) error {
	plannerID, _ := gonanoid.New()

	// Planner gets create_task + reconnaissance tools (list_dir, read_file)
	plannerTools := tool.NewRegistry()
	plannerTools.Register(tool.NewCreateTask(o.queue, plannerID, 0))
	plannerTools.Register(tool.NewListDir(o.cfg.Tools.WorkDir))
	plannerTools.Register(tool.NewReadFile(o.cfg.Tools.WorkDir))

	planner := agent.New(
		plannerID,
		agent.RolePlanner,
		o.client,
		o.cfg.Agents.Planner.Model,
		o.cfg.Agents.Planner.SystemPrompt,
		plannerTools,
	)

	// Inject user prompt
	planner.AddUserMessage(prompt)

	for step := 0; step < o.cfg.Loop.MaxSteps; step++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logging.Console.Printf("[planner] step %d/%d", step+1, o.cfg.Loop.MaxSteps)
		logging.File.Printf("[planner] step %d/%d", step+1, o.cfg.Loop.MaxSteps)

		result := planner.Step(ctx)
		if result.Error != nil {
			return fmt.Errorf("planner step %d: %w", step+1, result.Error)
		}

		logging.File.Printf("[planner] output: %s (tools: %d)", truncateLog(result.Output, 300), result.ToolCallsCount)

		// Check for termination signal
		if strings.Contains(result.Output, terminationSignal) {
			logging.Console.Println("[planner] ✓ CODEAGENTS_DONE")
			logging.File.Println("[planner] received CODEAGENTS_DONE")
			return nil
		}

		// Sleep before next reassessment
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.cfg.Loop.StepDelayDuration):
		}

		// Inject status update
		status := o.buildStatusMessage(planner.ID())
		planner.AddUserMessage(status)
	}

	return fmt.Errorf("max steps (%d) reached without CODEAGENTS_DONE", o.cfg.Loop.MaxSteps)
}

func truncateLog(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
