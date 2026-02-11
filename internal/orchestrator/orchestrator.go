package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
)

// Orchestrator coordinates the planner, workers, and subplanners.
type Orchestrator struct {
	cfg    *config.Config
	client llm.Completer
	queue  *task.Queue
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

	// Phase 1 + 2: Plan and approve (may loop on amend)
	if err := o.planAndApprove(ctx, prompt); err != nil {
		return err
	}

	// Phase 3: Execute
	return o.executeWorkers(ctx)
}

// planAndApprove runs the planner, displays the plan, and prompts for approval.
// On amend, clears the queue, re-runs planner with amended prompt, and re-prompts.
func (o *Orchestrator) planAndApprove(ctx context.Context, prompt string) error {
	currentPrompt := prompt

	for {
		// Run planner
		if err := o.runPlanner(ctx, currentPrompt); err != nil {
			return fmt.Errorf("planner error: %w", err)
		}

		// Display the plan
		o.displayPlan()

		// Prompt for approval
		choice, amendment := o.promptApproval()

		switch choice {
		case "yes":
			logging.Console.Println("[orchestrator] plan approved by user")
			logging.File.Println("[orchestrator] plan approved by user")
			return nil
		case "no":
			logging.Console.Println("[orchestrator] plan rejected by user")
			logging.File.Println("[orchestrator] plan rejected by user")
			return fmt.Errorf("plan rejected by user")
		case "amend":
			logging.Console.Printf("[orchestrator] plan amended: %s", amendment)
			logging.File.Printf("[orchestrator] plan amended: %s", amendment)
			o.queue.Clear()
			currentPrompt = prompt + "\n\nADDITIONAL INSTRUCTIONS FROM USER:\n" + amendment
			continue
		}
	}
}

// executeWorkers launches the worker pool and waits for all tasks to complete.
func (o *Orchestrator) executeWorkers(ctx context.Context) error {
	var wg sync.WaitGroup

	for i := 0; i < o.cfg.Loop.MaxWorkers; i++ {
		wg.Add(1)
		workerID, _ := gonanoid.New()
		go func(id string) {
			defer wg.Done()
			o.runWorker(ctx, id)
		}(workerID)
	}

	// Wait for all tasks to complete
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return fmt.Errorf("global timeout reached: %w", ctx.Err())
		case <-time.After(1 * time.Second):
			if o.queue.AllDone() {
				wg.Wait()
				return nil
			}
		}
	}
}

// displayPlan prints the current plan (all tasks) to the user.
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
	sb.WriteString(fmt.Sprintf("Tasks: %d pending, %d completed, %d failed\n",
		o.queue.PendingCount(), o.queue.CompletedCount(), o.queue.FailedCount()))

	// Show failed tasks
	tasks := o.queue.TasksByParent(parentID)
	for _, t := range tasks {
		if t.Status == task.StatusFailed {
			sb.WriteString(fmt.Sprintf("Failed: %q (reason: %s)\n", t.Title, t.FailReason))
		}
	}

	// Show recent handoffs
	handoffs := o.queue.HandoffsFor(parentID)
	if len(handoffs) > 0 {
		sb.WriteString("\nRecent handoffs:\n")
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
