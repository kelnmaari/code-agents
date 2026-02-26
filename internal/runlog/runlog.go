// Package runlog captures and persists mission run metrics for later analysis.
package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// RunLog captures metrics for a single mission run.
type RunLog struct {
	ID                string    `json:"id"`
	Profile           string    `json:"profile,omitempty"`
	Prompt            string    `json:"prompt"`
	StartedAt         time.Time `json:"started_at"`
	CompletedAt       time.Time `json:"completed_at"`
	DurationMs        int64     `json:"duration_ms"`
	Success           bool      `json:"success"`
	Error             string    `json:"error,omitempty"`
	PromptTokens      int       `json:"prompt_tokens"`
	CompletionTokens  int       `json:"completion_tokens"`
	TotalTokens       int       `json:"total_tokens"`
	FailedTasks       int       `json:"failed_tasks"`
	CompletedTasks    int       `json:"completed_tasks"`
	PlannerIterations int       `json:"planner_iterations"`
}

// New creates a new RunLog with a unique 8-char ID.
func New(profile, prompt string) *RunLog {
	id, _ := gonanoid.New(8)
	return &RunLog{
		ID:        id,
		Profile:   profile,
		Prompt:    prompt,
		StartedAt: time.Now(),
	}
}

// Finish marks the run as complete and records all outcome metrics.
func (r *RunLog) Finish(success bool, err error, promptTokens, completionTokens, totalTokens, failedTasks, completedTasks, plannerIterations int) {
	r.CompletedAt = time.Now()
	r.DurationMs = r.CompletedAt.Sub(r.StartedAt).Milliseconds()
	r.Success = success
	if err != nil {
		r.Error = err.Error()
	}
	r.PromptTokens = promptTokens
	r.CompletionTokens = completionTokens
	r.TotalTokens = totalTokens
	r.FailedTasks = failedTasks
	r.CompletedTasks = completedTasks
	r.PlannerIterations = plannerIterations
}

// Save writes the run log to <dir>/YYYY-MM-DD-<id>.json.
// Returns the path of the written file.
func (r *RunLog) Save(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create runs dir: %w", err)
	}

	date := r.StartedAt.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.json", date, r.ID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal run log: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write run log: %w", err)
	}
	return path, nil
}

// Load reads a RunLog from a JSON file.
func Load(path string) (*RunLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run log %q: %w", path, err)
	}
	var r RunLog
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse run log %q: %w", path, err)
	}
	return &r, nil
}
