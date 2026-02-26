package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

// runSubplanner runs a subplanner for a task marked as IsSubplan.
func (o *Orchestrator) runSubplanner(ctx context.Context, t *task.Task) error {
	subplannerID, _ := gonanoid.New()

	logging.File.Printf("[subplanner-%s] starting for task %s (depth %d)", subplannerID[:6], t.ID[:6], t.Depth)

	// Subplanner is an approved agent because its parent task was approved
	o.queue.RegisterApprovedAgent(subplannerID)

	// Subplanner gets create_task and submit_handoff
	subTools := tool.NewRegistry()
	subTools.Register(tool.NewCreateTask(o.queue, subplannerID, t.Depth))
	subTools.Register(tool.NewSubmitHandoff(o.queue, subplannerID))

	subplanner := agent.NewWithConfig(
		subplannerID,
		agent.RoleSubplanner,
		o.client,
		o.cfg.Agents.Subplanner.Model,
		o.cfg.Agents.Subplanner.SystemPrompt,
		subTools,
		o.cfg.Agents.Subplanner.MaxHistoryMessages,
	)

	// Inject task scope
	scopeMsg := formatSubplannerMessage(t)
	subplanner.AddUserMessage(scopeMsg)

	for step := 0; step < o.cfg.Loop.MaxSteps; step++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logging.File.Printf("[subplanner-%s] step %d/%d", subplannerID[:6], step+1, o.cfg.Loop.MaxSteps)

		result := subplanner.Step(ctx)
		o.reportUsage(result.Usage)
		if result.Error != nil {
			return fmt.Errorf("subplanner step %d: %w", step+1, result.Error)
		}

		// Check if submit_handoff was called (task completed)
		if o.queue.IsTaskCompleted(t.ID) {
			logging.File.Printf("[subplanner-%s] task %s completed via handoff", subplannerID[:6], t.ID[:6])
			return nil
		}

		// Wait for sub-tasks to complete
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.cfg.Loop.StepDelayDuration):
		}

		// Inject status of sub-tasks. Prune old ones.
		status := o.buildStatusMessage(subplannerID)
		subplanner.PruneMessagesByPrefix("=== STATUS UPDATE ===")
		subplanner.AddUserMessage(status)
	}

	return fmt.Errorf("subplanner max steps reached for task %s", t.ID)
}

func formatSubplannerMessage(t *task.Task) string {
	var sb strings.Builder
	sb.WriteString("=== SUBPLAN TASK ===\n")
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
	sb.WriteString(fmt.Sprintf("Depth: %d (max: use wisely)\n", t.Depth))
	sb.WriteString("\nBreak this into smaller tasks using create_task.\n")
	sb.WriteString("When all sub-tasks are complete, use submit_handoff to report upward.\n")
	sb.WriteString("===\n")
	return sb.String()
}
