package runlog_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/runlog"
)

func TestNew(t *testing.T) {
	rl := runlog.New("myprofile", "build a calculator")
	assert.NotEmpty(t, rl.ID)
	assert.Equal(t, "myprofile", rl.Profile)
	assert.Equal(t, "build a calculator", rl.Prompt)
	assert.False(t, rl.StartedAt.IsZero())
	assert.True(t, rl.CompletedAt.IsZero(), "CompletedAt should be zero before Finish()")
}

func TestFinish_Success(t *testing.T) {
	rl := runlog.New("", "test prompt")
	rl.Finish(true, nil, 100, 50, 150, 0, 3, 2)

	assert.True(t, rl.Success)
	assert.Empty(t, rl.Error)
	assert.Equal(t, 100, rl.PromptTokens)
	assert.Equal(t, 50, rl.CompletionTokens)
	assert.Equal(t, 150, rl.TotalTokens)
	assert.Equal(t, 0, rl.FailedTasks)
	assert.Equal(t, 3, rl.CompletedTasks)
	assert.Equal(t, 2, rl.PlannerIterations)
	assert.False(t, rl.CompletedAt.IsZero())
	assert.GreaterOrEqual(t, rl.DurationMs, int64(0))
}

func TestFinish_Error(t *testing.T) {
	rl := runlog.New("", "test prompt")
	rl.Finish(false, errors.New("something failed"), 200, 80, 280, 2, 1, 5)

	assert.False(t, rl.Success)
	assert.Equal(t, "something failed", rl.Error)
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	rl := runlog.New("profile-a", "build a web server")
	rl.Finish(true, nil, 500, 200, 700, 1, 5, 3)

	path, err := rl.Save(dir)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, ".json"))
	assert.FileExists(t, path)

	// Filename includes the date and ID.
	base := filepath.Base(path)
	assert.Contains(t, base, rl.ID)

	// Load it back and verify fields.
	loaded, err := runlog.Load(path)
	require.NoError(t, err)
	assert.Equal(t, rl.ID, loaded.ID)
	assert.Equal(t, rl.Profile, loaded.Profile)
	assert.Equal(t, rl.Prompt, loaded.Prompt)
	assert.Equal(t, rl.Success, loaded.Success)
	assert.Equal(t, rl.TotalTokens, loaded.TotalTokens)
	assert.Equal(t, rl.FailedTasks, loaded.FailedTasks)
	assert.Equal(t, rl.CompletedTasks, loaded.CompletedTasks)
	assert.Equal(t, rl.PlannerIterations, loaded.PlannerIterations)
}

func TestSave_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "runs")
	rl := runlog.New("", "prompt")
	rl.Finish(true, nil, 10, 5, 15, 0, 1, 1)

	_, err := rl.Save(dir)
	require.NoError(t, err)
	assert.DirExists(t, dir)
}

func TestLoad_NotFound(t *testing.T) {
	_, err := runlog.Load("/nonexistent/path.json")
	assert.Error(t, err)
}

func TestLoad_InvalidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.WriteString("not json")
	f.Close()

	_, err = runlog.Load(f.Name())
	assert.Error(t, err)
}

func TestCompare(t *testing.T) {
	dir := t.TempDir()

	a := runlog.New("baseline", "prompt A")
	a.Finish(true, nil, 1000, 400, 1400, 0, 5, 4)
	pathA, err := a.Save(dir)
	require.NoError(t, err)

	b := runlog.New("experiment", "prompt A")
	b.Finish(true, nil, 800, 300, 1100, 0, 5, 3)
	pathB, err := b.Save(dir)
	require.NoError(t, err)

	cr, err := runlog.Compare(pathA, pathB)
	require.NoError(t, err)

	assert.Equal(t, a.ID, cr.A.ID)
	assert.Equal(t, b.ID, cr.B.ID)
	assert.Contains(t, cr.Diff, "total_tokens")
	assert.Contains(t, cr.Diff, "planner_iterations")
	assert.Contains(t, cr.Diff, "failed_tasks")

	// total_tokens went from 1400 -> 1100 — should be a negative delta
	assert.Contains(t, cr.Diff["total_tokens"].Delta, "-")
}

func TestCompare_MissingFile(t *testing.T) {
	dir := t.TempDir()
	a := runlog.New("", "prompt")
	a.Finish(true, nil, 100, 50, 150, 0, 1, 1)
	pathA, _ := a.Save(dir)

	_, err := runlog.Compare(pathA, "/nonexistent.json")
	assert.Error(t, err)
}

func TestComparePrint(t *testing.T) {
	dir := t.TempDir()

	a := runlog.New("p1", "test")
	a.Finish(true, nil, 1000, 400, 1400, 1, 4, 5)
	pathA, _ := a.Save(dir)

	b := runlog.New("p2", "test")
	b.Finish(false, errors.New("timed out"), 900, 350, 1250, 2, 3, 6)
	pathB, _ := b.Save(dir)

	cr, err := runlog.Compare(pathA, pathB)
	require.NoError(t, err)

	// Print should not panic and should produce output.
	var sb strings.Builder
	cr.Print(&sb)
	out := sb.String()
	assert.Contains(t, out, "RUN COMPARISON")
	assert.Contains(t, out, "total_tokens")
	assert.Contains(t, out, "planner_iterations")
}
