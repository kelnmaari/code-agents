package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComplete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		resp := ChatResponse{
			ID: "chatcmpl-123",
			Choices: []Choice{
				{
					Message:      ChatMessage{Role: RoleAssistant, Content: "Hello!"},
					FinishReason: "stop",
				},
			},
			Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	resp, err := client.Complete(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hi"}},
	})

	require.NoError(t, err)
	require.Equal(t, "chatcmpl-123", resp.ID)
	require.Len(t, resp.Choices, 1)
	require.Equal(t, "Hello!", resp.Choices[0].Message.Content)
	require.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestComplete_Retry429(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
			return
		}
		resp := ChatResponse{
			ID:      "chatcmpl-retry",
			Choices: []Choice{{Message: ChatMessage{Role: RoleAssistant, Content: "OK"}, FinishReason: "stop"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	resp, err := client.Complete(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hi"}},
	})

	require.NoError(t, err)
	require.Equal(t, "chatcmpl-retry", resp.ID)
	require.Equal(t, int32(3), attempts.Load())
}

func TestComplete_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.Complete(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hi"}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestComplete_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond - context will cancel first
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient(srv.URL, "test-key")
	_, err := client.Complete(ctx, ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hi"}},
	})

	require.Error(t, err)
}

func TestComplete_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{ID: "chatcmpl-empty", Choices: []Choice{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.Complete(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hi"}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no choices")
}
