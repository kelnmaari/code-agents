package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

// mockCompleter returns pre-scripted responses in sequence.
type mockCompleter struct {
	responses []*llm.ChatResponse
	index     int
	requests  []llm.ChatRequest // captured for inspection
}

func (m *mockCompleter) Complete(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.requests = append(m.requests, req)
	if m.index >= len(m.responses) {
		return &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message:      llm.ChatMessage{Role: llm.RoleAssistant, Content: "fallback response"},
				FinishReason: "stop",
			}},
		}, nil
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func testModelCfg() config.ModelConfig {
	return config.ModelConfig{Model: "test-model", Temperature: 0.3, MaxTokens: 1024}
}

func TestStep_NoToolCalls(t *testing.T) {
	mock := &mockCompleter{
		responses: []*llm.ChatResponse{
			{Choices: []llm.Choice{{
				Message:      llm.ChatMessage{Role: llm.RoleAssistant, Content: "Hello from planner"},
				FinishReason: "stop",
			}}},
		},
	}

	reg := tool.NewRegistry()
	a := New("test-1", RolePlanner, mock, testModelCfg(), "You are a planner.", reg)
	a.AddUserMessage("Plan this project")

	result := a.Step(context.Background())

	require.True(t, result.Stop)
	require.Equal(t, "Hello from planner", result.Output)
	require.NoError(t, result.Error)
	require.Equal(t, 0, result.ToolCallsCount)

	// Verify request had system + user messages
	require.Len(t, mock.requests, 1)
	require.Len(t, mock.requests[0].Messages, 2) // system + user
}

func TestStep_WithToolCalls(t *testing.T) {
	// Mock: first response has a tool call, second is text
	mock := &mockCompleter{
		responses: []*llm.ChatResponse{
			{Choices: []llm.Choice{{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "echo_tool",
							Arguments: `{"text": "hello"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}}},
			{Choices: []llm.Choice{{
				Message:      llm.ChatMessage{Role: llm.RoleAssistant, Content: "Done processing"},
				FinishReason: "stop",
			}}},
		},
	}

	// Create a simple echo tool
	reg := tool.NewRegistry()
	reg.Register(&echoTool{})

	a := New("test-2", RoleWorker, mock, testModelCfg(), "You are a worker.", reg)
	a.AddUserMessage("Do the task")

	result := a.Step(context.Background())

	require.True(t, result.Stop)
	require.Equal(t, "Done processing", result.Output)
	require.NoError(t, result.Error)
	require.Equal(t, 1, result.ToolCallsCount)

	// Second request should include: system, user, assistant(tool_calls), tool(result)
	require.Len(t, mock.requests, 2)
	require.Len(t, mock.requests[1].Messages, 4)
	require.Equal(t, llm.RoleTool, mock.requests[1].Messages[3].Role)
}

func TestStep_UnknownTool(t *testing.T) {
	mock := &mockCompleter{
		responses: []*llm.ChatResponse{
			{Choices: []llm.Choice{{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "nonexistent_tool",
							Arguments: `{}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}}},
			{Choices: []llm.Choice{{
				Message:      llm.ChatMessage{Role: llm.RoleAssistant, Content: "OK"},
				FinishReason: "stop",
			}}},
		},
	}

	reg := tool.NewRegistry()
	a := New("test-3", RoleWorker, mock, testModelCfg(), "You are a worker.", reg)

	result := a.Step(context.Background())

	require.True(t, result.Stop)
	require.Equal(t, 1, result.ToolCallsCount)
}

func TestStep_ContextCancelled(t *testing.T) {
	mock := &mockCompleter{} // won't be called
	reg := tool.NewRegistry()
	a := New("test-4", RolePlanner, mock, testModelCfg(), "You are a planner.", reg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := a.Step(ctx)
	require.Error(t, result.Error)
}

// echoTool is a simple test tool that echoes its input.
type echoTool struct{}

func (e *echoTool) Name() string                                           { return "echo_tool" }
func (e *echoTool) Description() string                                    { return "Echo input" }
func (e *echoTool) Parameters() interface{}                                { return map[string]interface{}{"type": "object"} }
func (e *echoTool) Execute(_ context.Context, args string) (string, error) { return "echo: " + args, nil }
