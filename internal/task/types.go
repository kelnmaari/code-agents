package task

import "time"

// TaskStatus represents the state of a task in the queue.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusAssigned  TaskStatus = "assigned"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// TaskPriority determines task ordering in the queue.
type TaskPriority int

const (
	PriorityLow    TaskPriority = 0
	PriorityNormal TaskPriority = 1
	PriorityHigh   TaskPriority = 2
)

// Task is a unit of work in the system.
type Task struct {
	ID          string       `json:"id"`
	ParentID    string       `json:"parent_id,omitempty"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Status      TaskStatus   `json:"status"`
	Priority    TaskPriority `json:"priority"`
	Scope       string       `json:"scope"`
	Constraints []string     `json:"constraints"`
	CreatedAt   time.Time    `json:"created_at"`
	AssignedTo  string       `json:"assigned_to,omitempty"`
	Handoff     *Handoff     `json:"handoff,omitempty"`
	IsSubplan   bool         `json:"is_subplan"`
	Approved    bool         `json:"approved"`
	Depth       int          `json:"depth"`
	FailReason  string       `json:"fail_reason,omitempty"`
	RetryCount  int          `json:"retry_count,omitempty"`
	MaxRetries  int          `json:"max_retries,omitempty"`
	DependsOn   []string     `json:"depends_on,omitempty"`
}

// Handoff is the document workers/subplanners send back to planners.
// This is the primary mechanism for upward information flow.
type Handoff struct {
	TaskID       string   `json:"task_id"`
	AgentID      string   `json:"agent_id"`
	Summary      string   `json:"summary"`
	Findings     []string `json:"findings"`
	Concerns     []string `json:"concerns"`
	Feedback     []string `json:"feedback"`
	FilesChanged []string `json:"files_changed"`
}
