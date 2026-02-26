package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Collector holds all observability metrics for the agent system.
// All counter updates are lock-free via atomics; only wait-time history uses a mutex.
type Collector struct {
	tasksPending    atomic.Int64
	tasksCompleted  atomic.Int64
	tasksFailed     atomic.Int64
	tokensPrompt    atomic.Int64
	tokensCompletion atomic.Int64
	workerBusy      atomic.Int64
	workerTotal     atomic.Int64

	mu        sync.Mutex
	waitTimes []float64 // sliding window of recent queue wait times (seconds)
	startTime time.Time
}

// New creates a new Collector.
func New() *Collector {
	return &Collector{
		startTime: time.Now(),
	}
}

// SetTasksPending sets the current number of pending tasks.
func (c *Collector) SetTasksPending(n int64) {
	c.tasksPending.Store(n)
}

// SetTasksCompleted sets the total completed task count.
func (c *Collector) SetTasksCompleted(n int64) {
	c.tasksCompleted.Store(n)
}

// SetTasksFailed sets the total failed task count.
func (c *Collector) SetTasksFailed(n int64) {
	c.tasksFailed.Store(n)
}

// AddTokens accumulates token usage numbers.
func (c *Collector) AddTokens(prompt, completion int64) {
	c.tokensPrompt.Add(prompt)
	c.tokensCompletion.Add(completion)
}

// SetWorkers updates the busy / total worker counts.
func (c *Collector) SetWorkers(busy, total int64) {
	c.workerBusy.Store(busy)
	c.workerTotal.Store(total)
}

// RecordQueueWait records how long a task waited in the queue before being picked up.
func (c *Collector) RecordQueueWait(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waitTimes = append(c.waitTimes, d.Seconds())
	// Keep only the last 100 observations to bound memory usage.
	if len(c.waitTimes) > 100 {
		c.waitTimes = c.waitTimes[len(c.waitTimes)-100:]
	}
}

// Snapshot is a point-in-time read of all metrics, safe to serialise as JSON.
type Snapshot struct {
	TasksPending     int64   `json:"tasks_pending"`
	TasksCompleted   int64   `json:"tasks_completed"`
	TasksFailed      int64   `json:"tasks_failed"`
	TokensPrompt     int64   `json:"tokens_prompt"`
	TokensCompletion int64   `json:"tokens_completion"`
	TokensTotal      int64   `json:"tokens_total"`
	WorkerBusy       int64   `json:"worker_busy"`
	WorkerTotal      int64   `json:"worker_total"`
	WorkerBusyRatio  float64 `json:"worker_busy_ratio"`
	QueueWaitAvgSec  float64 `json:"queue_wait_time_avg_sec"`
	UptimeSeconds    float64 `json:"uptime_seconds"`
}

// Snapshot returns a consistent copy of all current metrics.
func (c *Collector) Snapshot() Snapshot {
	prompt := c.tokensPrompt.Load()
	completion := c.tokensCompletion.Load()
	busy := c.workerBusy.Load()
	total := c.workerTotal.Load()

	var ratio float64
	if total > 0 {
		ratio = float64(busy) / float64(total)
	}

	c.mu.Lock()
	var avgWait float64
	if len(c.waitTimes) > 0 {
		sum := 0.0
		for _, w := range c.waitTimes {
			sum += w
		}
		avgWait = sum / float64(len(c.waitTimes))
	}
	c.mu.Unlock()

	return Snapshot{
		TasksPending:     c.tasksPending.Load(),
		TasksCompleted:   c.tasksCompleted.Load(),
		TasksFailed:      c.tasksFailed.Load(),
		TokensPrompt:     prompt,
		TokensCompletion: completion,
		TokensTotal:      prompt + completion,
		WorkerBusy:       busy,
		WorkerTotal:      total,
		WorkerBusyRatio:  ratio,
		QueueWaitAvgSec:  avgWait,
		UptimeSeconds:    time.Since(c.startTime).Seconds(),
	}
}
