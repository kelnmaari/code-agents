package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func makeTask(id string, priority TaskPriority) *Task {
	return &Task{
		ID:        id,
		Title:     "Task " + id,
		Priority:  priority,
		CreatedAt: time.Now(),
	}
}

func TestQueue_PushPull(t *testing.T) {
	q := NewQueue()
	task1 := makeTask("1", PriorityNormal)

	q.Push(task1)

	ctx := context.Background()
	got := q.Pull(ctx)

	require.NotNil(t, got)
	require.Equal(t, "1", got.ID)
	require.Equal(t, StatusAssigned, got.Status)
}

func TestQueue_Priority(t *testing.T) {
	q := NewQueue()

	low := makeTask("low", PriorityLow)
	low.CreatedAt = time.Now()
	high := makeTask("high", PriorityHigh)
	high.CreatedAt = time.Now().Add(time.Second) // created later but higher priority

	q.Push(low)
	q.Push(high)

	ctx := context.Background()
	first := q.Pull(ctx)
	require.Equal(t, "high", first.ID)

	second := q.Pull(ctx)
	require.Equal(t, "low", second.ID)
}

func TestQueue_PullBlocks(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	got := make(chan *Task, 1)
	go func() {
		got <- q.Pull(ctx)
	}()

	// Give goroutine time to block
	time.Sleep(50 * time.Millisecond)

	select {
	case <-got:
		t.Fatal("Pull should be blocking")
	default:
	}

	// Push a task to unblock
	q.Push(makeTask("1", PriorityNormal))

	select {
	case task := <-got:
		require.Equal(t, "1", task.ID)
	case <-time.After(time.Second):
		t.Fatal("Pull should have returned")
	}
}

func TestQueue_PullCancelled(t *testing.T) {
	q := NewQueue()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	got := q.Pull(ctx)
	require.Nil(t, got)
}

func TestQueue_Complete(t *testing.T) {
	q := NewQueue()
	task1 := makeTask("1", PriorityNormal)
	task1.ParentID = "planner-1"
	q.Push(task1)

	ctx := context.Background()
	pulled := q.Pull(ctx)
	require.Equal(t, StatusAssigned, pulled.Status)

	handoff := &Handoff{
		TaskID:  "1",
		AgentID: "worker-1",
		Summary: "Done",
	}
	q.Complete("1", handoff)

	require.True(t, q.AllDone())
	require.Equal(t, 1, q.CompletedCount())

	handoffs := q.HandoffsFor("planner-1")
	require.Len(t, handoffs, 1)
	require.Equal(t, "Done", handoffs[0].Summary)
}

func TestQueue_Fail(t *testing.T) {
	q := NewQueue()
	q.Push(makeTask("1", PriorityNormal))

	ctx := context.Background()
	q.Pull(ctx)
	q.Fail("1", "timeout")

	require.True(t, q.AllDone())
	require.Equal(t, 1, q.FailedCount())
}

func TestQueue_HandoffsFor(t *testing.T) {
	q := NewQueue()

	t1 := makeTask("1", PriorityNormal)
	t1.ParentID = "planner-A"
	t2 := makeTask("2", PriorityNormal)
	t2.ParentID = "planner-A"
	t3 := makeTask("3", PriorityNormal)
	t3.ParentID = "planner-B"

	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	ctx := context.Background()
	q.Pull(ctx)
	q.Pull(ctx)
	q.Pull(ctx)

	q.Complete("1", &Handoff{TaskID: "1", Summary: "S1"})
	q.Complete("2", &Handoff{TaskID: "2", Summary: "S2"})
	q.Complete("3", &Handoff{TaskID: "3", Summary: "S3"})

	handoffsA := q.HandoffsFor("planner-A")
	require.Len(t, handoffsA, 2)

	handoffsB := q.HandoffsFor("planner-B")
	require.Len(t, handoffsB, 1)
}

func TestQueue_AllDone_EmptyQueue(t *testing.T) {
	q := NewQueue()
	require.True(t, q.AllDone())
}

func TestQueue_ConcurrentAccess(t *testing.T) {
	q := NewQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numTasks = 100
	var wg sync.WaitGroup

	// Push tasks concurrently
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			q.Push(makeTask(
				time.Now().Format("150405.000000000")+"-push",
				PriorityNormal,
			))
		}(i)
	}
	wg.Wait()

	// Pull all tasks concurrently
	pulled := make(chan *Task, numTasks)
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := q.Pull(ctx)
			if task != nil {
				pulled <- task
			}
		}()
	}
	wg.Wait()
	close(pulled)

	count := 0
	for range pulled {
		count++
	}
	require.Equal(t, numTasks, count)
}
