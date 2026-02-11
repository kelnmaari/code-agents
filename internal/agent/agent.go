package agent

import (
	"context"
	"fmt"
	"log"

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

		resp, err := a.handleChatRequest(ctx)
		if err != nil {
			return RunResult{Error: fmt.Errorf("llm complete: %w", err)}
		}

		msg := resp.Choices[0].Message

		// Log LLM response
		log.Printf("[%s/%s] === LLM Response (iter %d) ===", a.role, a.id[:6], i+1)
		if msg.Content != "" {
			log.Printf("[%s/%s] Content: %s", a.role, a.id[:6], msg.Content)
		}
		if len(msg.ToolCalls) > 0 {
			log.Printf("[%s/%s] Tool calls: %d", a.role, a.id[:6], len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				log.Printf("[%s/%s]   -> %s(%s)", a.role, a.id[:6], tc.Function.Name, truncateStr(tc.Function.Arguments, 500))
			}
		}

		a.appendMessage(msg)

		if len(msg.ToolCalls) == 0 {
			return RunResult{
				Stop:           true,
				Output:         msg.Content,
				ToolCallsCount: totalToolCalls,
			}
		}

		totalToolCalls += a.processToolExecution(ctx, msg.ToolCalls)
	}

	return RunResult{
		Error: fmt.Errorf("max inner iterations (%d) reached in agent step", maxInnerIterations),
	}
}

// handleChatRequest sends messages to the LLM and gets the response.
func (a *Agent) handleChatRequest(ctx context.Context) (*llm.ChatResponse, error) {
	req := llm.ChatRequest{
		Model:       a.modelCfg.Model,
		Messages:    a.messages,
		Temperature: a.modelCfg.Temperature,
		MaxTokens:   a.modelCfg.MaxTokens,
	}

	if defs := a.tools.Definitions(); len(defs) > 0 {
		req.Tools = defs
	}

	// Log outgoing request
	log.Printf("[%s/%s] === LLM Request (model: %s, messages: %d, tools: %d) ===",
		a.role, a.id[:6], req.Model, len(req.Messages), len(req.Tools))
	for i, m := range req.Messages {
		content := truncateStr(m.Content, 500)
		if m.ToolCallID != "" {
			log.Printf("[%s/%s]   msg[%d] role=%s tool_call_id=%s: %s", a.role, a.id[:6], i, m.Role, m.ToolCallID, content)
		} else {
			log.Printf("[%s/%s]   msg[%d] role=%s: %s", a.role, a.id[:6], i, m.Role, content)
		}
	}

	return a.client.Complete(ctx, req)
}

// processToolExecution executes tool calls.
func (a *Agent) processToolExecution(ctx context.Context, toolCalls []llm.ToolCall) int {
	totalToolCalls := 0

	for _, tc := range toolCalls {
		log.Printf("[%s/%s] === Tool Call: %s ===", a.role, a.id[:6], tc.Function.Name)
		log.Printf("[%s/%s]   Args: %s", a.role, a.id[:6], truncateStr(tc.Function.Arguments, 1000))

		t, ok := a.tools.Get(tc.Function.Name)
		if !ok {
			errMsg := fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
			log.Printf("[%s/%s]   Result: %s", a.role, a.id[:6], errMsg)
			a.appendMessage(llm.ChatMessage{
				Role:       llm.RoleTool,
				Content:    errMsg,
				ToolCallID: tc.ID,
			})
			totalToolCalls++
			continue
		}

		result, err := t.Execute(ctx, tc.Function.Arguments)
		if err != nil {
			log.Printf("[%s/%s]   Error: %v", a.role, a.id[:6], err)
			return totalToolCalls
		}

		log.Printf("[%s/%s]   Result: %s", a.role, a.id[:6], truncateStr(result, 1000))

		a.appendMessage(llm.ChatMessage{
			Role:       llm.RoleTool,
			Content:    result,
			ToolCallID: tc.ID,
		})
		totalToolCalls++
	}

	return totalToolCalls
}

// appendMessage appends a message to the conversation.
func (a *Agent) appendMessage(msg llm.ChatMessage) {
	a.messages = append(a.messages, msg)
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

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
