package runlog

import (
	"fmt"
	"io"
	"math"
)

// CompareResult holds the side-by-side diff of two run logs.
type CompareResult struct {
	A    *RunLog
	B    *RunLog
	Diff map[string]DiffEntry
}

// DiffEntry captures the values of one metric across both runs.
type DiffEntry struct {
	A     interface{}
	B     interface{}
	Delta string // human-readable delta (e.g. "+12.3%", "-4")
}

// Compare produces a CompareResult from two RunLog files.
func Compare(pathA, pathB string) (*CompareResult, error) {
	a, err := Load(pathA)
	if err != nil {
		return nil, fmt.Errorf("loading first run: %w", err)
	}
	b, err := Load(pathB)
	if err != nil {
		return nil, fmt.Errorf("loading second run: %w", err)
	}

	cr := &CompareResult{A: a, B: b, Diff: make(map[string]DiffEntry)}

	cr.Diff["profile"] = DiffEntry{A: a.Profile, B: b.Profile}
	cr.Diff["success"] = DiffEntry{A: a.Success, B: b.Success}
	cr.Diff["duration_ms"] = diffInt("duration_ms", int(a.DurationMs), int(b.DurationMs))
	cr.Diff["prompt_tokens"] = diffInt("prompt_tokens", a.PromptTokens, b.PromptTokens)
	cr.Diff["completion_tokens"] = diffInt("completion_tokens", a.CompletionTokens, b.CompletionTokens)
	cr.Diff["total_tokens"] = diffInt("total_tokens", a.TotalTokens, b.TotalTokens)
	cr.Diff["failed_tasks"] = diffInt("failed_tasks", a.FailedTasks, b.FailedTasks)
	cr.Diff["completed_tasks"] = diffInt("completed_tasks", a.CompletedTasks, b.CompletedTasks)
	cr.Diff["planner_iterations"] = diffInt("planner_iterations", a.PlannerIterations, b.PlannerIterations)

	return cr, nil
}

// Print writes a human-readable comparison table to w.
func (cr *CompareResult) Print(w io.Writer) {
	fmt.Fprintf(w, "\n┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Fprintf(w, "│                    RUN COMPARISON                          │\n")
	fmt.Fprintf(w, "├──────────────────────┬────────────────┬────────────────┬───┤\n")
	fmt.Fprintf(w, "│ Metric               │ %-14s │ %-14s │ Δ │\n", truncate(cr.A.ID, 14), truncate(cr.B.ID, 14))
	fmt.Fprintf(w, "├──────────────────────┼────────────────┼────────────────┼───┤\n")

	metrics := []string{
		"profile",
		"success",
		"duration_ms",
		"prompt_tokens",
		"completion_tokens",
		"total_tokens",
		"failed_tasks",
		"completed_tasks",
		"planner_iterations",
	}
	for _, key := range metrics {
		e := cr.Diff[key]
		delta := e.Delta
		if delta == "" {
			if fmt.Sprintf("%v", e.A) == fmt.Sprintf("%v", e.B) {
				delta = "="
			} else {
				delta = "~"
			}
		}
		fmt.Fprintf(w, "│ %-20s │ %-14v │ %-14v │ %-3s │\n",
			truncate(key, 20), e.A, e.B, truncate(delta, 6))
	}
	fmt.Fprintf(w, "└──────────────────────┴────────────────┴────────────────┴───┘\n")

	// Prompts
	fmt.Fprintf(w, "\nRun A (%s) prompt: %s\n", cr.A.ID, truncate(cr.A.Prompt, 80))
	fmt.Fprintf(w, "Run B (%s) prompt: %s\n\n", cr.B.ID, truncate(cr.B.Prompt, 80))
}

func diffInt(_ string, a, b int) DiffEntry {
	delta := b - a
	var deltaStr string
	if a == 0 {
		if b == 0 {
			deltaStr = "="
		} else {
			deltaStr = fmt.Sprintf("+%d", b)
		}
	} else {
		pct := math.Round(float64(delta)/float64(a)*100*10) / 10
		if delta > 0 {
			deltaStr = fmt.Sprintf("+%.1f%%", pct)
		} else if delta < 0 {
			deltaStr = fmt.Sprintf("%.1f%%", pct)
		} else {
			deltaStr = "="
		}
	}
	return DiffEntry{A: a, B: b, Delta: deltaStr}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
