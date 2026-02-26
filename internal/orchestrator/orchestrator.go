package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/agent"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

// RunResult holds the outcome metrics of a completed orchestrator run.
type RunResult struct {
	PlannerIterations int
	FailedTasks       int
	CompletedTasks    int
	Usage             llm.Usage
}

// Orchestrator coordinates the planner, workers, and subplanners.
type Orchestrator struct {
	cfg             *config.Config
	client          llm.Completer
	queue           *task.Queue
	planner         *agent.Agent         // Persistent planner instance
	plannerTaskTool *tool.CreateTaskTool // Reference for counter reset

	usageMu      sync.Mutex
	usage        llm.Usage
	plannerSteps int
}

// New creates an orchestrator from the given configuration.
func New(cfg *config.Config) *Orchestrator {
	client := llm.NewClient(cfg.Provider.BaseURL, cfg.Provider.APIKey)
	return &Orchestrator{
		cfg:    cfg,
		client: client,
		queue:  task.NewQueue(),
	}
}

// NewWithClient creates an orchestrator with a custom LLM client (for testing).
func NewWithClient(cfg *config.Config, client llm.Completer) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		client: client,
		queue:  task.NewQueue(),
	}
}

// Run is the main entry point. It runs in 3 phases:
// 1. PLAN   — run planner to create tasks
// 2. APPROVE — display plan, prompt user for YES/NO/Amend
// 3. EXECUTE — launch workers and wait for completion
func (o *Orchestrator) Run(ctx context.Context, prompt string) error {
	ctx, cancel := context.WithTimeout(ctx, o.cfg.Loop.TimeoutDuration)
	defer cancel()

	// Initial prompt injection (only once)
	o.ensurePlanner()
	o.planner.AddUserMessage(prompt)

	step := 0
	for {
		step++
		if step > o.cfg.Loop.MaxSteps {
			return fmt.Errorf("max planner steps (%d) reached without CODEAGENTS_DONE", o.cfg.Loop.MaxSteps)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		o.plannerSteps = step
		logging.Console.Printf("[orchestrator] phase: Plan & Approve (Turn %d/%d)", step, o.cfg.Loop.MaxSteps)
		// Phase 1 + 2: Plan and approve
		done, err := o.planAndApprove(ctx, step)
		if err != nil {
			return err
		}
		if done {
			// Check if any tasks failed before finishing
			if failed := o.queue.FailedCount(); failed > 0 {
				return fmt.Errorf("mission finished with %d failed task(s)", failed)
			}
			return nil
		}

		// Phase 3: Execute (only if there are pending tasks)
		pending := o.queue.PendingCount()
		if pending > 0 {
			logging.Console.Printf("[orchestrator] phase: Execution (%d tasks)", pending)
			if err := o.executeWorkers(ctx); err != nil {
				return err
			}
			logging.Console.Println("[orchestrator] phase: Workers finished")
		} else {
			// If no tasks pending but not done, we might be in a weird state.
			// However, usually we just wait a bit and let the planner reassess.
			time.Sleep(o.cfg.Loop.StepDelayDuration)
		}
	}
}

// ensurePlanner initializes the persistent planner if not already present.
func (o *Orchestrator) ensurePlanner() {
	if o.planner != nil {
		return
	}
	o.planner = o.initPlanner()
}

// planAndApprove runs the planner, displays the plan, and prompts for approval.
func (o *Orchestrator) planAndApprove(ctx context.Context, step int) (bool, error) {
	// Run planner. This returns bool indicating mission completion (CODEAGENTS_DONE)
	done, err := o.runPlanner(ctx, step)
	if err != nil {
		return false, fmt.Errorf("planner error: %w", err)
	}

	if done {
		return true, nil
	}

	// Check if there are NEW tasks that require approval
	newTasks := o.queue.UnapprovedPendingCount()

	// If no unapproved pending tasks, we yield to workers (no approval needed)
	if newTasks == 0 {
		return false, nil
	}

	// Auto-approve if configured
	if o.cfg.Loop.AutoApprove {
		logging.Console.Printf("[orchestrator] auto-approving %d new tasks", newTasks)
		o.queue.ApproveTasks()
		return false, nil
	}

	// Display and prompt
	o.displayPlan()
	choice, amendment := o.promptApproval()

	switch choice {
	case "yes":
		logging.Console.Println("[orchestrator] plan approved")
		o.queue.ApproveTasks()
		return false, nil // Proceed to execution
	case "no":
		return false, fmt.Errorf("plan rejected by user")
	case "amend":
		logging.Console.Printf("[orchestrator] plan amended: %s", amendment)
		o.queue.Clear() // This is bit aggressive but follows existing logic
		o.planner.AddUserMessage("ADDITIONAL INSTRUCTIONS FROM USER:\n" + amendment)
		return o.planAndApprove(ctx, step) // Re-plan
	default:
		// This case should ideally not be reached due to promptApproval's loop,
		// but as a safeguard, treat it as a rejection or an internal error.
		return false, fmt.Errorf("unexpected approval choice: %s", choice)
	}
}

// executeWorkers launches the worker pool and waits for all tasks to complete.
func (o *Orchestrator) executeWorkers(ctx context.Context) error {
	var wg sync.WaitGroup
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	// Reset batch state for this run
	o.queue.OpenBatch()

	for i := 0; i < o.cfg.Loop.MaxWorkers; i++ {
		wg.Add(1)
		workerID, _ := gonanoid.New()
		go func(id string) {
			defer wg.Done()
			o.runWorker(workerCtx, id)
		}(workerID)
	}

	// Wait for all tasks to complete
	for {
		select {
		case <-ctx.Done():
			cancelWorkers()
			wg.Wait()
			return fmt.Errorf("global timeout reached: %w", ctx.Err())
		case <-time.After(1 * time.Second):
			executable := o.queue.ExecutablePendingCount()
			assigned := o.queue.AssignedCount()

			if executable == 0 && assigned == 0 {
				logging.File.Printf("[orchestrator] no executable tasks left, yielding to planner (total pending: %d)", o.queue.PendingCount())
				logging.Console.Printf("[orchestrator] Yielding to planner (waiting for re-approval or final review)")
				// Signal batch closure (Drain) instead of killing workers immediately
				o.queue.CloseBatch()
				wg.Wait()
				return nil
			}

			// Periodic verbose status
			logging.File.Printf("[orchestrator] waiting for tasks (executable: %d, busy: %d, total pending: %d)", executable, assigned, o.queue.PendingCount())
		}
	}
}

// Results returns the outcome metrics after Run() completes.
func (o *Orchestrator) Results() RunResult {
	o.usageMu.Lock()
	usage := o.usage
	o.usageMu.Unlock()
	return RunResult{
		PlannerIterations: o.plannerSteps,
		FailedTasks:       o.queue.FailedCount(),
		CompletedTasks:    o.queue.CompletedCount(),
		Usage:             usage,
	}
}

func (o *Orchestrator) reportUsage(u llm.Usage) {
	o.usageMu.Lock()
	o.usage.PromptTokens += u.PromptTokens
	o.usage.CompletionTokens += u.CompletionTokens
	o.usage.TotalTokens += u.TotalTokens
	usage := o.usage
	o.usageMu.Unlock()

	logging.Console.Printf("[usage] tokens: %d (prompt: %d, completion: %d)", usage.TotalTokens, usage.PromptTokens, usage.CompletionTokens)
}

// displayPlan prints the current task queue to the console.
func (o *Orchestrator) displayPlan() {
	tasks := o.queue.ListAll()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║                    EXECUTION PLAN                       ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")

	if len(tasks) == 0 {
		fmt.Println("║  No tasks created.                                      ║")
	} else {
		for i, t := range tasks {
			fmt.Printf("║  %d. %-52s ║\n", i+1, truncatePlan(t.Title, 52))
			if t.Description != "" {
				// Wrap description
				desc := truncatePlan(t.Description, 100)
				fmt.Printf("║     %s\n", desc)
			}
			if len(t.Constraints) > 0 {
				fmt.Printf("║     Constraints: %s\n", strings.Join(t.Constraints, ", "))
			}
			if t.Scope != "" {
				fmt.Printf("║     Scope: %s\n", t.Scope)
			}
			if i < len(tasks)-1 {
				fmt.Println("║  ──────────────────────────────────────────────────────  ║")
			}
		}
	}

	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Total tasks: %-42d ║\n", len(tasks))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// promptApproval asks the user to approve, reject, or amend the plan.
// Returns ("yes", ""), ("no", ""), or ("amend", "<user text>").
func (o *Orchestrator) promptApproval() (string, string) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("Choose an action:")
		fmt.Println("  [Y]es    — approve and execute the plan")
		fmt.Println("  [N]o     — reject and abort")
		fmt.Println("  [A]mend  — add instructions and re-plan")
		fmt.Print("\n> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch {
		case input == "y" || input == "yes":
			return "yes", ""
		case input == "n" || input == "no":
			return "no", ""
		case input == "a" || input == "amend":
			fmt.Println("\nEnter additional instructions (press Enter twice to finish):")
			var lines []string
			for {
				line, _ := reader.ReadString('\n')
				trimmed := strings.TrimSpace(line)
				if trimmed == "" && len(lines) > 0 {
					break
				}
				if trimmed != "" {
					lines = append(lines, trimmed)
				}
			}
			amendment := strings.Join(lines, "\n")
			if amendment == "" {
				fmt.Println("No amendment text provided. Try again.")
				continue
			}
			return "amend", amendment
		default:
			fmt.Println("Invalid input. Please enter Y, N, or A.")
		}
	}
}

func truncatePlan(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// buildStatusMessage creates a status update for the planner.
func (o *Orchestrator) buildStatusMessage(parentID string) string {
	var sb strings.Builder
	sb.WriteString("=== STATUS UPDATE ===\n")
	sb.WriteString(fmt.Sprintf("Environment: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("Global: %d pending, %d assigned/busy, %d completed, %d failed\n",
		o.queue.PendingCount(), len(o.queue.ListAll())-o.queue.PendingCount()-o.queue.CompletedCount()-o.queue.FailedCount(),
		o.queue.CompletedCount(), o.queue.FailedCount()))

	tasks := o.queue.TasksByParent(parentID)
	if len(tasks) > 0 {
		sb.WriteString("\nYour Tasks:\n")
		for _, t := range tasks {
			sb.WriteString(fmt.Sprintf("- [%s] %s (ID: %s)\n", t.Status, t.Title, t.ID))
			if t.Status == task.StatusFailed {
				sb.WriteString(fmt.Sprintf("  Error: %s\n", t.FailReason))
			}
		}
	}

	// Show recent handoffs
	handoffs := o.queue.HandoffsFor(parentID)
	if len(handoffs) > 0 {
		sb.WriteString("\nRecent handoffs from completed tasks:\n")
		for _, h := range handoffs {
			sb.WriteString("---\n")
			sb.WriteString(fmt.Sprintf("Task: %s\n", h.TaskID))
			sb.WriteString(fmt.Sprintf("Summary: %s\n", h.Summary))
			if len(h.Findings) > 0 {
				sb.WriteString(fmt.Sprintf("Findings: %s\n", strings.Join(h.Findings, "; ")))
			}
			if len(h.Concerns) > 0 {
				sb.WriteString(fmt.Sprintf("Concerns: %s\n", strings.Join(h.Concerns, "; ")))
			}
			if len(h.FilesChanged) > 0 {
				sb.WriteString(fmt.Sprintf("Files changed: %s\n", strings.Join(h.FilesChanged, ", ")))
			}
		}
	}

	sb.WriteString("===\n")
	return sb.String()
}
