package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCollector_Snapshot(t *testing.T) {
	c := New()

	c.SetTasksPending(3)
	c.SetTasksCompleted(10)
	c.SetTasksFailed(1)
	c.AddTokens(100, 50)
	c.SetWorkers(2, 4)
	c.RecordQueueWait(2 * time.Second)
	c.RecordQueueWait(4 * time.Second)

	snap := c.Snapshot()

	require.Equal(t, int64(3), snap.TasksPending)
	require.Equal(t, int64(10), snap.TasksCompleted)
	require.Equal(t, int64(1), snap.TasksFailed)
	require.Equal(t, int64(100), snap.TokensPrompt)
	require.Equal(t, int64(50), snap.TokensCompletion)
	require.Equal(t, int64(150), snap.TokensTotal)
	require.Equal(t, int64(2), snap.WorkerBusy)
	require.Equal(t, int64(4), snap.WorkerTotal)
	require.InDelta(t, 0.5, snap.WorkerBusyRatio, 0.001)
	require.InDelta(t, 3.0, snap.QueueWaitAvgSec, 0.001) // avg of 2s and 4s
	require.GreaterOrEqual(t, snap.UptimeSeconds, 0.0)
}

func TestServer_StatusEndpoint(t *testing.T) {
	c := New()
	c.SetTasksPending(5)
	c.SetTasksCompleted(7)

	srv := NewServer(c, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	srv.handleStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var snap Snapshot
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&snap))
	require.Equal(t, int64(5), snap.TasksPending)
	require.Equal(t, int64(7), snap.TasksCompleted)
}

func TestServer_MetricsEndpoint(t *testing.T) {
	c := New()
	c.SetTasksPending(2)
	c.AddTokens(200, 100)

	srv := NewServer(c, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.handleMetrics(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	text := string(body)

	require.True(t, strings.Contains(text, "tasks_pending 2"), "expected tasks_pending 2 in: %s", text)
	require.True(t, strings.Contains(text, "tokens_used_total 300"), "expected tokens_used_total 300 in: %s", text)
}

func TestServer_UIEndpoint(t *testing.T) {
	c := New()
	srv := NewServer(c, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.handleUI(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), "Code-Agents Dashboard")
}
