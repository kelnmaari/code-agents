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
	mu       sync.Mutex
	tasks    map[string]*Task
	pending  []*Task
	handoffs map[string]*Handoff
	notify   chan struct{}
}

// NewQueue creates an empty task queue.
func NewQueue() *Queue {
	return &Queue{
		tasks:    make(map[string]*Task),
		pending:  make([]*Task, 0),
		handoffs: make(map[string]*Handoff),
		notify:   make(chan struct{}, 1), // buffered 1 to avoid missing signals
	}
}

// Push adds a task to the queue.
func (q *Queue) Push(t *Task) {
	q.mu.Lock()
	t.Status = StatusPending
	q.tasks[t.ID] = t
	q.pending = append(q.pending, t)
	// Sort by priority (high first), then by creation time (FIFO within same priority)
	sort.SliceStable(q.pending, func(i, j int) bool {
		if q.pending[i].Priority != q.pending[j].Priority {
			return q.pending[i].Priority > q.pending[j].Priority
		}
		return q.pending[i].CreatedAt.Before(q.pending[j].CreatedAt)
	})
	q.mu.Unlock()

	// Non-blocking send to wake up one waiting Pull
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Pull removes and returns the highest-priority pending task.
// Blocks until a task is available or the context is cancelled.
// Returns nil when context is done.
func (q *Queue) Pull(ctx context.Context) *Task {
	for {
		q.mu.Lock()
		if len(q.pending) > 0 {
			t := q.pending[0]
			q.pending = q.pending[1:]
			t.Status = StatusAssigned
			q.mu.Unlock()
			return t
		}
		q.mu.Unlock()

		// Wait for notification or cancellation
		select {
		case <-ctx.Done():
			return nil
		case <-q.notify:
			// Re-check pending (another goroutine may have taken the task)
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
	}
	q.handoffs[taskID] = h
	q.mu.Unlock()

	// Notify (e.g., planner waiting for handoffs)
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Fail marks a task as failed.
func (q *Queue) Fail(taskID string, reason string) {
	q.mu.Lock()
	if t, ok := q.tasks[taskID]; ok {
		t.Status = StatusFailed
		t.FailReason = reason
	}
	q.mu.Unlock()

	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// PendingCount returns the number of pending tasks.
func (q *Queue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
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
	for _, t := range q.tasks {
		if t.Status == StatusPending || t.Status == StatusAssigned {
			return false
		}
	}
	return true
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

// Clear resets the queue, removing all tasks and handoffs.
// Used when re-planning after user amendment.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = make(map[string]*Task)
	q.pending = make([]*Task, 0)
	q.handoffs = make(map[string]*Handoff)
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
