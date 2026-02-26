package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/task"
)

// --- safePath tests ---

func TestSafePath_Valid(t *testing.T) {
	workDir := t.TempDir()
	got, err := safePath(workDir, "foo/bar.txt")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(workDir, "foo", "bar.txt"), got)
}

func TestSafePath_CurrentDir(t *testing.T) {
	workDir := t.TempDir()
	got, err := safePath(workDir, ".")
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(workDir), got)
}

func TestSafePath_TraversalBlocked(t *testing.T) {
	workDir := t.TempDir()
	_, err := safePath(workDir, "../../../etc/passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")
}

func TestSafePath_DotDotInMiddle(t *testing.T) {
	workDir := t.TempDir()
	_, err := safePath(workDir, "foo/../../etc/passwd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")
}

// --- Registry tests ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoToolTest{})

	tool, ok := reg.Get("echo_test")
	require.True(t, ok)
	require.Equal(t, "echo_test", tool.Name())
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("nonexistent")
	require.False(t, ok)
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoToolTest{})
	require.Panics(t, func() {
		reg.Register(&echoToolTest{})
	})
}

func TestRegistry_Definitions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoToolTest{})
	defs := reg.Definitions()
	require.Len(t, defs, 1)
	require.Equal(t, "echo_test", defs[0].Function.Name)
	require.Equal(t, "function", defs[0].Type)
}

func TestRegistry_Names(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoToolTest{})
	names := reg.Names()
	require.Equal(t, []string{"echo_test"}, names)
}

// --- ReadFileTool tests ---

func TestReadFile_Success(t *testing.T) {
	workDir := t.TempDir()
	os.WriteFile(filepath.Join(workDir, "hello.txt"), []byte("world"), 0o644)

	tool := NewReadFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "hello.txt"}`)
	require.NoError(t, err)
	require.Contains(t, result, "world")
}

func TestReadFile_NotFound(t *testing.T) {
	workDir := t.TempDir()
	tool := NewReadFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "missing.txt"}`)
	require.NoError(t, err) // expected errors as string
	require.Contains(t, result, "Error")
}

func TestReadFile_PathTraversal(t *testing.T) {
	workDir := t.TempDir()
	tool := NewReadFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "../../../etc/passwd"}`)
	require.NoError(t, err)
	require.Contains(t, result, "path traversal")
}

func TestReadFile_TooLarge(t *testing.T) {
	workDir := t.TempDir()
	// Create a file slightly over 1MB
	bigData := make([]byte, maxFileSize+100)
	os.WriteFile(filepath.Join(workDir, "big.bin"), bigData, 0o644)

	tool := NewReadFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "big.bin"}`)
	require.NoError(t, err)
	require.Contains(t, result, "too large")
}

// --- WriteFileTool tests ---

func TestWriteFile_Success(t *testing.T) {
	workDir := t.TempDir()
	tool := NewWriteFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "sub/out.txt", "content": "hello"}`)
	require.NoError(t, err)
	require.Contains(t, result, "File written")

	// Verify file was created
	data, readErr := os.ReadFile(filepath.Join(workDir, "sub", "out.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "hello", string(data))
}

func TestWriteFile_PathTraversal(t *testing.T) {
	workDir := t.TempDir()
	tool := NewWriteFile(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "../../evil.txt", "content": "bad"}`)
	require.NoError(t, err)
	require.Contains(t, result, "path traversal")
}

// --- ListDirTool tests ---

func TestListDir_Success(t *testing.T) {
	workDir := t.TempDir()
	os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(workDir, "subdir"), 0o755)

	tool := NewListDir(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "."}`)
	require.NoError(t, err)
	require.Contains(t, result, "a.txt")
	require.Contains(t, result, "subdir/")
}

func TestListDir_NotFound(t *testing.T) {
	workDir := t.TempDir()
	tool := NewListDir(workDir)
	result, err := tool.Execute(context.Background(), `{"path": "nonexistent"}`)
	require.NoError(t, err)
	require.Contains(t, result, "Error")
}

// --- ShellExecTool tests (whitelist only, no actual shell execution) ---

func TestIsAllowed_EmptyWhitelist(t *testing.T) {
	// Empty whitelist => isAllowed doesn't apply (caller checks len first)
	require.False(t, isAllowed("ls", []string{}))
}

func TestIsAllowed_Allowed(t *testing.T) {
	require.True(t, isAllowed("go test ./...", []string{"go", "npm", "git"}))
}

func TestIsAllowed_Blocked(t *testing.T) {
	require.False(t, isAllowed("rm -rf /", []string{"go", "npm", "git"}))
}

func TestIsAllowed_FullPath(t *testing.T) {
	require.True(t, isAllowed("/usr/bin/go build", []string{"go"}))
}

func TestIsAllowed_EmptyCommand(t *testing.T) {
	require.False(t, isAllowed("", []string{"go"}))
}

// --- CreateTaskTool tests ---

func TestCreateTask_Execute(t *testing.T) {
	q := task.NewQueue()
	q.RegisterApprovedAgent("parent-1")
	tool := NewCreateTask(q, "parent-1", 0)

	result, err := tool.Execute(context.Background(), `{"title":"Test task","description":"Do the thing","priority":"high"}`)
	require.NoError(t, err)
	require.Contains(t, result, "Task created")

	require.Equal(t, 1, q.PendingCount())

	// Approve tasks so Pull() doesn't block (tasks from create_task are not auto-approved)
	q.ApproveTasks()

	// Pull and verify
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pulled := q.Pull(ctx)
	require.NotNil(t, pulled)
	require.Equal(t, "Test task", pulled.Title)
	require.Equal(t, "Do the thing", pulled.Description)
	require.Equal(t, "parent-1", pulled.ParentID)
	require.Equal(t, 1, pulled.Depth)
	require.Equal(t, task.PriorityHigh, pulled.Priority)
}

func TestCreateTask_Subplan(t *testing.T) {
	q := task.NewQueue()
	q.RegisterApprovedAgent("parent-1")
	tool := NewCreateTask(q, "parent-1", 1)

	_, err := tool.Execute(context.Background(), `{"title":"Sub","description":"Sub work","is_subplan":true}`)
	require.NoError(t, err)

	// Approve tasks so Pull() doesn't block
	q.ApproveTasks()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pulled := q.Pull(ctx)
	require.True(t, pulled.IsSubplan)
	require.Equal(t, 2, pulled.Depth)
}

// --- CompleteTaskTool tests ---

func TestCompleteTask_Execute(t *testing.T) {
	q := task.NewQueue()
	// Push a dummy task
	q.Push(&task.Task{ID: "task-1", Title: "Test", Approved: true})
	// Pull to assign
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Pull(ctx)

	tool := NewCompleteTask(q, "worker-1")
	result, err := tool.Execute(context.Background(), `{"task_id":"task-1","summary":"All done","files_changed":["main.go"]}`)
	require.NoError(t, err)
	require.Contains(t, result, "completed")

	require.True(t, q.IsTaskCompleted("task-1"))
	handoffs := q.HandoffsFor("")
	require.Len(t, handoffs, 1)
	require.Equal(t, "All done", handoffs[0].Summary)
	require.Equal(t, []string{"main.go"}, handoffs[0].FilesChanged)
}

// --- SubmitHandoffTool tests ---

func TestSubmitHandoff_Execute(t *testing.T) {
	q := task.NewQueue()
	q.Push(&task.Task{ID: "task-2", Title: "Sub", Approved: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Pull(ctx)

	tool := NewSubmitHandoff(q, "sub-1")
	result, err := tool.Execute(context.Background(), `{"task_id":"task-2","summary":"Sub done","findings":["Found X"]}`)
	require.NoError(t, err)
	require.Contains(t, result, "Handoff submitted")
	require.True(t, q.IsTaskCompleted("task-2"))
}

// echoToolTest is a test helper tool
type echoToolTest struct{}

func (e *echoToolTest) Name() string                                           { return "echo_test" }
func (e *echoToolTest) Description() string                                    { return "Echo test" }
func (e *echoToolTest) Parameters() interface{}                                { return map[string]interface{}{"type": "object"} }
func (e *echoToolTest) Execute(_ context.Context, args string) (string, error) { return "echo: " + args, nil }
