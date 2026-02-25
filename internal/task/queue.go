package task

import (
	"context"
	"sort"
	"sync"
)

// Queue is a thread-safe task store.
// Planners push tasks, workers pull them.
// Uses chan struct{} for notification to avoid busy-waiting.
type Queue struct {
	mu             sync.Mutex
	tasks          map[string]*Task
	pending        []*Task
	handoffs       map[string]*Handoff
	busyScopes     map[string]string // scope -> taskID
	approvedAgents map[string]bool   // agentID -> true (agents whose parent tasks have been approved)
	batchClosed    bool              // if true, Pull() returns nil when pending is empty
	notify         chan struct{}     // closed to broadcast, recreated to reset
}

// NewQueue creates an empty task queue.
func NewQueue() *Queue {
	return &Queue{
		tasks:          make(map[string]*Task),
		pending:        make([]*Task, 0),
		handoffs:       make(map[string]*Handoff),
		busyScopes:     make(map[string]string),
		approvedAgents: make(map[string]bool),
		batchClosed:    false,
		notify:         make(chan struct{}),
	}
}

// Push adds a task to the queue.
func (q *Queue) Push(t *Task) {
	q.mu.Lock()
	t.Status = StatusPending

	// Auto-approve if the parent agent is already "trusted" (approved)
	if q.approvedAgents[t.ParentID] {
		t.Approved = true
	}

	q.tasks[t.ID] = t
	q.pending = append(q.pending, t)
	// Sort by priority (high first), then by creation time (FIFO within same priority)
	sort.SliceStable(q.pending, func(i, j int) bool {
		if q.pending[i].Priority != q.pending[j].Priority {
			return q.pending[i].Priority > q.pending[j].Priority
		}
		return q.pending[i].CreatedAt.Before(q.pending[j].CreatedAt)
	})
	q.broadcast() // Wake up everyone
	q.mu.Unlock()
}

// Pull removes and returns the highest-priority pending task.
// Blocks until a task is available or the context is cancelled.
// Returns nil when context is done.
func (q *Queue) Pull(ctx context.Context) *Task {
	for {
		q.mu.Lock()
		var picked *Task
		var pickedIdx int

		for i, t := range q.pending {
			// A task can only be pulled if it is APPROVED.
			if !t.Approved {
				continue
			}

			// If scope is empty, it's always available.
			// If scope is set, check if it's currently busy.
			if t.Scope == "" || q.busyScopes[t.Scope] == "" {
				picked = t
				pickedIdx = i
				break
			}
		}

		if picked != nil {
			// Remove from pending
			q.pending = append(q.pending[:pickedIdx], q.pending[pickedIdx+1:]...)
			picked.Status = StatusAssigned
			if picked.Scope != "" {
				q.busyScopes[picked.Scope] = picked.ID
			}
			q.mu.Unlock()
			return picked
		}

		// If no task found and batch is closed, return nil to let worker exit.
		// We only return nil if there are NO MORE approved tasks in pending.
		if q.batchClosed {
			hasApproved := false
			for _, t := range q.pending {
				if t.Approved {
					hasApproved = true
					break
				}
			}
			if !hasApproved {
				q.mu.Unlock()
				return nil
			}
		}

		// Capture current notify channel under lock
		n := q.notify
		q.mu.Unlock()

		// Wait for notification or cancellation
		select {
		case <-ctx.Done():
			return nil
		case <-n:
			// Re-check pending (another goroutine may have pushed a task or released a scope)
			continue
		}
	}
}

// Complete marks a task as completed and stores its handoff.
func (q *Queue) Complete(taskID string, h *Handoff) {
	q.mu.Lock()
	if t, ok := q.tasks[taskID]; ok {
		t.Status = StatusCompleted
		t.Handoff = h
		if t.Scope != "" {
			delete(q.busyScopes, t.Scope)
		}

		// Invalidation Logic:
		// If this task modified files, any OTHER pending tasks for those files
		// might now have stale context (e.g. shifted line numbers).
		// We unapprove them to force the planner to re-verify.
		if h != nil {
			for _, file := range h.FilesChanged {
				q.resetPendingByScopeLocked(file, taskID)
			}
		}
	}
	q.handoffs[taskID] = h
	q.mu.Unlock()

	// Notify workers (a scope might have been released)
	q.broadcast()
}

// resetPendingByScopeLocked marks all pending tasks for a scope as unapproved.
// Internal version: caller must hold q.mu.
func (q *Queue) resetPendingByScopeLocked(scope string, excludeTaskID string) {
	if scope == "" {
		return
	}
	for _, t := range q.pending {
		if t.ID != excludeTaskID && t.Scope == scope && t.Approved {
			t.Approved = false // Set to false to trigger re-approval loop in Orchestrator
		}
	}
}

// Fail marks a task as failed.
func (q *Queue) Fail(taskID string, reason string) {
	q.mu.Lock()
	if t, ok := q.tasks[taskID]; ok {
		t.Status = StatusFailed
		t.FailReason = reason
		if t.Scope != "" {
			delete(q.busyScopes, t.Scope)
		}
	}
	q.mu.Unlock()

	q.broadcast()
}

// RetryTask resets a failed (or assigned) task back to pending for retry.
// It increments RetryCount and re-inserts the task into the pending queue.
// The caller is responsible for checking MaxRetries before calling this.
func (q *Queue) RetryTask(taskID string) {
	q.mu.Lock()
	if t, ok := q.tasks[taskID]; ok {
		// Release scope lock if held
		if t.Scope != "" {
			delete(q.busyScopes, t.Scope)
		}
		t.Status = StatusPending
		t.RetryCount++
		t.FailReason = ""
		q.pending = append(q.pending, t)
		sort.SliceStable(q.pending, func(i, j int) bool {
			if q.pending[i].Priority != q.pending[j].Priority {
				return q.pending[i].Priority > q.pending[j].Priority
			}
			return q.pending[i].CreatedAt.Before(q.pending[j].CreatedAt)
		})
		q.broadcast()
	}
	q.mu.Unlock()
}

// broadcast wakes up all waiting Pull calls by closing and recreating the notify channel.
// Caller MUST hold q.mu.
func (q *Queue) broadcast() {
	close(q.notify)
	q.notify = make(chan struct{})
}

// PendingCount returns the number of pending tasks.
func (q *Queue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// UnapprovedPendingCount returns the number of pending tasks that haven't been approved yet.
func (q *Queue) UnapprovedPendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.pending {
		if !t.Approved {
			count++
		}
	}
	return count
}

// ExecutablePendingCount returns the number of pending tasks that are approved.
func (q *Queue) ExecutablePendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.pending {
		if t.Approved {
			count++
		}
	}
	return count
}

// RegisterApprovedAgent marks an agent as trusted (its tasks will be auto-approved).
func (q *Queue) RegisterApprovedAgent(agentID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.approvedAgents[agentID] = true
}

// CloseBatch marks the current execution batch as closing.
// Workers in Pull() will return nil once all pending tasks are gone.
func (q *Queue) CloseBatch() {
	q.mu.Lock()
	q.batchClosed = true
	q.mu.Unlock()
	q.broadcast()
}

// OpenBatch resets the batch closed state for a new worker pool run.
func (q *Queue) OpenBatch() {
	q.mu.Lock()
	q.batchClosed = false
	q.mu.Unlock()
}

// ApproveTasks marks all currently pending tasks as approved.
func (q *Queue) ApproveTasks() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.pending {
		t.Approved = true
	}
}

// CompletedCount returns the number of completed tasks.
func (q *Queue) CompletedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.tasks {
		if t.Status == StatusCompleted {
			count++
		}
	}
	return count
}

// FailedCount returns the number of failed tasks.
func (q *Queue) FailedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.tasks {
		if t.Status == StatusFailed {
			count++
		}
	}
	return count
}

// AssignedCount returns the number of tasks currently assigned to workers.
func (q *Queue) AssignedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.tasks {
		if t.Status == StatusAssigned {
			count++
		}
	}
	return count
}

// HandoffsFor returns all completed handoffs for tasks created by the given parent.
func (q *Queue) HandoffsFor(parentID string) []*Handoff {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*Handoff
	for _, t := range q.tasks {
		if t.ParentID == parentID && t.Handoff != nil {
			result = append(result, t.Handoff)
		}
	}
	return result
}

// AllDone returns true when there are no pending or assigned tasks.
func (q *Queue) AllDone() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	pending := len(q.pending)
	busy := len(q.busyScopes)
	assigned := 0
	for _, t := range q.tasks {
		if t.Status == StatusAssigned {
			assigned++
		}
	}

	done := (pending == 0 && assigned == 0 && busy == 0)

	// Logging for debugging (only to file)
	// We use the package name logging here as it's imported in orchestrator but not in task.
	// Wait, task doesn't import logging. I should add a way to log or just use standard log
	// but better to avoid circle. I'll just keep it clean and let Orchestrator log.
	return done
}

// TasksByParent returns all tasks created by the given parent.
func (q *Queue) TasksByParent(parentID string) []*Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*Task
	for _, t := range q.tasks {
		if t.ParentID == parentID {
			result = append(result, t)
		}
	}
	return result
}

// IsTaskCompleted checks if a specific task has been completed.
func (q *Queue) IsTaskCompleted(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.tasks[taskID]; ok {
		return t.Status == StatusCompleted
	}
	return false
}

// Clear resets the queue, removing all tasks, handoffs, and approved agents.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = make(map[string]*Task)
	q.pending = make([]*Task, 0)
	q.handoffs = make(map[string]*Handoff)
	q.approvedAgents = make(map[string]bool)
	q.batchClosed = false
	// Re-create notify to ensure it's not closed
	q.notify = make(chan struct{})
}

// ListAll returns all tasks sorted by creation time.
func (q *Queue) ListAll() []*Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*Task, 0, len(q.tasks))
	for _, t := range q.tasks {
		result = append(result, t)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// HasActiveTaskForScope returns true if there is already a pending or assigned task
// with the given scope. Used by create_task to prevent duplicate task creation.
func (q *Queue) HasActiveTaskForScope(scope string) bool {
	if scope == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		if t.Scope == scope && (t.Status == StatusPending || t.Status == StatusAssigned) {
			return true
		}
	}
	return false
}
