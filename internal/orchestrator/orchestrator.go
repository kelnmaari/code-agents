package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
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

// Run is the main entry point. It creates the planner and worker pool,
// then waits for completion, timeout, or max_steps.
func (o *Orchestrator) Run(ctx context.Context, prompt string) error {
	ctx, cancel := context.WithTimeout(ctx, o.cfg.Loop.TimeoutDuration)
	defer cancel()

	var wg sync.WaitGroup
	plannerDone := make(chan error, 1)

	// Start planner goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		plannerDone <- o.runPlanner(ctx, prompt)
	}()

	// Start worker pool
	for i := 0; i < o.cfg.Loop.MaxWorkers; i++ {
		wg.Add(1)
		workerID, _ := gonanoid.New()
		go func(id string) {
			defer wg.Done()
			o.runWorker(ctx, id)
		}(workerID)
	}

	// Wait for termination
	plannerExited := false
	for {
		select {
		case <-ctx.Done():
			cancel()
			wg.Wait()
			return fmt.Errorf("global timeout reached: %w", ctx.Err())

		case err := <-plannerDone:
			plannerExited = true
			if err != nil {
				log.Printf("[orchestrator] planner error: %v", err)
				cancel()
				wg.Wait()
				return fmt.Errorf("planner error: %w", err)
			}
			log.Println("[orchestrator] planner completed successfully")
			if o.queue.AllDone() {
				cancel()
				wg.Wait()
				return nil
			}

		case <-time.After(1 * time.Second):
			if plannerExited && o.queue.AllDone() {
				cancel()
				wg.Wait()
				return nil
			}
		}
	}
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
