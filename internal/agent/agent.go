package agent

import (
	"context"
	"fmt"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

const maxInnerIterations = 20

// Agent wraps an LLM client, conversation history, tool registry, and system prompt.
// It runs an inner tool-use loop: send messages -> get response -> execute tools -> repeat.
type Agent struct {
	id           string
	role         Role
	client       llm.Completer
	modelCfg     config.ModelConfig
	systemPrompt string
	tools        *tool.Registry
	messages     []llm.ChatMessage
}

// New creates an agent with the given configuration.
// The system prompt is added as the first message.
func New(
	id string,
	role Role,
	client llm.Completer,
	modelCfg config.ModelConfig,
	systemPrompt string,
	tools *tool.Registry,
) *Agent {
	a := &Agent{
		id:           id,
		role:         role,
		client:       client,
		modelCfg:     modelCfg,
		systemPrompt: systemPrompt,
		tools:        tools,
		messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: systemPrompt},
		},
	}
	return a
}

// Step executes one full "turn" of the agent.
// It loops: call LLM -> if tool_calls, execute tools and repeat -> if no tool_calls, return.
// Max 20 inner iterations to prevent infinite loops.
func (a *Agent) Step(ctx context.Context) RunResult {
	totalToolCalls := 0

	for i := 0; i < maxInnerIterations; i++ {
		select {
		case <-ctx.Done():
			return RunResult{Error: ctx.Err()}
		default:
		}

		req := llm.ChatRequest{
			Model:       a.modelCfg.Model,
			Messages:    a.messages,
			Temperature: a.modelCfg.Temperature,
			MaxTokens:   a.modelCfg.MaxTokens,
		}

		// Only include tools if the registry is non-empty
		if defs := a.tools.Definitions(); len(defs) > 0 {
			req.Tools = defs
		}

		resp, err := a.client.Complete(ctx, req)
		if err != nil {
			return RunResult{Error: fmt.Errorf("llm complete: %w", err)}
		}

		msg := resp.Choices[0].Message
		// Append assistant message to conversation
		a.messages = append(a.messages, msg)

		// No tool calls -> agent is done for this step
		if len(msg.ToolCalls) == 0 {
			return RunResult{
				Stop:           true,
				Output:         msg.Content,
				ToolCallsCount: totalToolCalls,
			}
		}

		// Execute each tool call
		for _, tc := range msg.ToolCalls {
			t, ok := a.tools.Get(tc.Function.Name)
			if !ok {
				// Unknown tool - return error as tool result so LLM can see it
				a.messages = append(a.messages, llm.ChatMessage{
					Role:       llm.RoleTool,
					Content:    fmt.Sprintf("Error: unknown tool %q", tc.Function.Name),
					ToolCallID: tc.ID,
				})
				totalToolCalls++
				continue
			}

			result, err := t.Execute(ctx, tc.Function.Arguments)
			if err != nil {
				// Critical error (context cancelled, panic, etc)
				return RunResult{Error: fmt.Errorf("tool %q: %w", tc.Function.Name, err)}
			}

			a.messages = append(a.messages, llm.ChatMessage{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
			totalToolCalls++
		}
		// Continue loop: LLM will see tool results on next iteration
	}

	return RunResult{
		Error: fmt.Errorf("max inner iterations (%d) reached in agent step", maxInnerIterations),
	}
}

// AddUserMessage appends a user message to the conversation.
// Used by the orchestrator to inject status updates, task descriptions, etc.
func (a *Agent) AddUserMessage(content string) {
	a.messages = append(a.messages, llm.ChatMessage{
		Role:    llm.RoleUser,
		Content: content,
	})
}

// ID returns the unique identifier of this agent.
func (a *Agent) ID() string {
	return a.id
}

// Messages returns the current conversation history.
func (a *Agent) Messages() []llm.ChatMessage {
	// Return a copy to prevent external mutation
	cp := make([]llm.ChatMessage, len(a.messages))
	copy(cp, a.messages)
	return cp
}
