package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
)

// --- CreateTaskTool ---

// CreateTaskTool allows planners to create tasks on the queue.
type CreateTaskTool struct {
	queue        *task.Queue
	parentID     string
	depth        int
	tasksCreated int // counter: how many tasks this planner has created
	maxPerTurn   int // max tasks allowed per planner turn
}

// NewCreateTask creates a tool for adding tasks to the queue.
func NewCreateTask(queue *task.Queue, parentID string, depth int) *CreateTaskTool {
	return &CreateTaskTool{queue: queue, parentID: parentID, depth: depth, maxPerTurn: 3}
}

// ResetCounter resets the per-turn task creation counter.
// Must be called at the start of each planner reassessment cycle.
func (t *CreateTaskTool) ResetCounter() {
	t.tasksCreated = 0
}

func (t *CreateTaskTool) Name() string        { return "create_task" }
func (t *CreateTaskTool) Description() string { return "Create a new task for workers or subplanners" }
func (t *CreateTaskTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Brief task title",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Detailed task description with context",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Files or areas this task affects",
			},
			"constraints": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Constraints for the worker (boundaries, what NOT to do)",
			},
			"priority": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"low", "normal", "high"},
				"description": "Task priority. Default: normal",
			},
			"is_subplan": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, task will be handled by a subplanner instead of a worker",
			},
		},
		"required": []string{"title", "description"},
	}
}

func (t *CreateTaskTool) Execute(_ context.Context, args string) (string, error) {
	// RATE LIMIT: Prevent planner from creating too many tasks in one session
	if t.tasksCreated >= t.maxPerTurn {
		return fmt.Sprintf("REJECTED: You have already created %d tasks this turn (limit: %d). Stop creating tasks and yield control. Workers will execute what you have planned.", t.tasksCreated, t.maxPerTurn), nil
	}

	var params struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Scope       string   `json:"scope"`
		Constraints []string `json:"constraints"`
		Priority    string   `json:"priority"`
		IsSubplan   bool     `json:"is_subplan"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	// DEDUPLICATION: Reject if a task with the same scope already exists
	if params.Scope != "" && t.queue.HasActiveTaskForScope(params.Scope) {
		return fmt.Sprintf("REJECTED: A task for scope '%s' already exists (pending or in progress). Do NOT create duplicate tasks. Stop and yield control to let workers finish.", params.Scope), nil
	}

	id, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("generate task ID: %w", err)
	}

	priority := task.PriorityNormal
	switch params.Priority {
	case "low":
		priority = task.PriorityLow
	case "high":
		priority = task.PriorityHigh
	}

	newTask := &task.Task{
		ID:          id,
		ParentID:    t.parentID,
		Title:       params.Title,
		Description: params.Description,
		Scope:       params.Scope,
		Constraints: params.Constraints,
		Priority:    priority,
		CreatedAt:   time.Now(),
		IsSubplan:   params.IsSubplan,
		Depth:       t.depth + 1,
	}

	t.queue.Push(newTask)
	t.tasksCreated++

	msg := fmt.Sprintf("Task created (%d/%d): %s - %s. NOTE: This task is queued for WORKERS. Stop and yield.", t.tasksCreated, t.maxPerTurn, id, params.Title)
	return msg, nil
}

// --- CompleteTaskTool ---

// CompleteTaskTool allows workers to mark a task as completed with a handoff.
type CompleteTaskTool struct {
	queue   *task.Queue
	agentID string
}

// NewCompleteTask creates a tool for completing tasks.
func NewCompleteTask(queue *task.Queue, agentID string) *CompleteTaskTool {
	return &CompleteTaskTool{queue: queue, agentID: agentID}
}

func (t *CompleteTaskTool) Name() string { return "complete_task" }
func (t *CompleteTaskTool) Description() string {
	return "Mark a task as completed and submit a handoff document"
}
func (t *CompleteTaskTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the task being completed",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Summary of what was done",
			},
			"findings": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Things discovered during work",
			},
			"concerns": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Potential issues or risks",
			},
			"feedback": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Suggestions for the planner",
			},
			"files_changed": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of files created or modified",
			},
		},
		"required": []string{"task_id", "summary"},
	}
}

func (t *CompleteTaskTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		TaskID       string   `json:"task_id"`
		Summary      string   `json:"summary"`
		Findings     []string `json:"findings"`
		Concerns     []string `json:"concerns"`
		Feedback     []string `json:"feedback"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	handoff := &task.Handoff{
		TaskID:       params.TaskID,
		AgentID:      t.agentID,
		Summary:      params.Summary,
		Findings:     params.Findings,
		Concerns:     params.Concerns,
		Feedback:     params.Feedback,
		FilesChanged: params.FilesChanged,
	}

	t.queue.Complete(params.TaskID, handoff)
	return fmt.Sprintf("Task %s completed. Handoff submitted.", params.TaskID), nil
}

// --- SubmitHandoffTool ---

// SubmitHandoffTool allows subplanners to submit aggregate handoffs.
type SubmitHandoffTool struct {
	queue   *task.Queue
	agentID string
}

// NewSubmitHandoff creates a tool for subplanner handoff submission.
func NewSubmitHandoff(queue *task.Queue, agentID string) *SubmitHandoffTool {
	return &SubmitHandoffTool{queue: queue, agentID: agentID}
}

func (t *SubmitHandoffTool) Name() string { return "submit_handoff" }
func (t *SubmitHandoffTool) Description() string {
	return "Submit an aggregate handoff document for a subplan task"
}
func (t *SubmitHandoffTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the subplan task being completed",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Aggregate summary of all sub-tasks",
			},
			"findings": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Aggregated findings from sub-tasks",
			},
			"concerns": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Aggregated concerns from sub-tasks",
			},
			"feedback": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Aggregated feedback from sub-tasks",
			},
			"files_changed": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "All files changed across sub-tasks",
			},
		},
		"required": []string{"task_id", "summary"},
	}
}

func (t *SubmitHandoffTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		TaskID       string   `json:"task_id"`
		Summary      string   `json:"summary"`
		Findings     []string `json:"findings"`
		Concerns     []string `json:"concerns"`
		Feedback     []string `json:"feedback"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	handoff := &task.Handoff{
		TaskID:       params.TaskID,
		AgentID:      t.agentID,
		Summary:      params.Summary,
		Findings:     params.Findings,
		Concerns:     params.Concerns,
		Feedback:     params.Feedback,
		FilesChanged: params.FilesChanged,
	}

	t.queue.Complete(params.TaskID, handoff)
	return fmt.Sprintf("Handoff submitted for task %s.", params.TaskID), nil
}
