package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/config"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/tool"
)

const (
	maxInnerIterations = 20
	maxNudges          = 3 // max "nudge" prompts per Step when model responds with text but no tool calls
)

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
	var result RunResult
	totalToolCalls := 0
	nudgeCount := 0

	// Determine max iterations based on role.
	// Planners should shouldn't loop forever internally.
	limit := maxInnerIterations
	if a.role == RolePlanner {
		limit = 5
	}

	for i := 0; i < limit; i++ {
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
		logging.File.Printf("[%s/%s] === LLM Response (iter %d) ===", a.role, a.id[:6], i+1)
		if msg.Content != "" {
			logging.File.Printf("[%s/%s] Content: %s", a.role, a.id[:6], msg.Content)
		}
		if len(msg.ToolCalls) > 0 {
			logging.File.Printf("[%s/%s] Tool calls: %d", a.role, a.id[:6], len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				logging.File.Printf("[%s/%s]   -> %s(%s)", a.role, a.id[:6], tc.Function.Name, truncateStr(tc.Function.Arguments, 500))
			}
		}

		// Try to parse tool calls from text if model embedded them
		if len(msg.ToolCalls) == 0 && msg.Content != "" {
			if synthetic := parseTextToolCalls(msg.Content, a.tools); len(synthetic) > 0 {
				logging.File.Printf("[%s/%s] parsed %d synthetic tool call(s) from text", a.role, a.id[:6], len(synthetic))
				msg.ToolCalls = synthetic
			}
		}

		a.appendMessage(msg)
		result.Usage.PromptTokens += resp.Usage.PromptTokens
		result.Usage.CompletionTokens += resp.Usage.CompletionTokens
		result.Usage.TotalTokens += resp.Usage.TotalTokens

		if len(msg.ToolCalls) == 0 {
			// Nudge: if the model described what it wants to do but didn't call tools,
			// prompt it to actually use them.
			// CRITICAL: only nudge if we haven't done ANY tool calls in this Turn yet.
			// If we've already done tools, the model is likely just summarizing/concluding.
			if totalToolCalls == 0 && nudgeCount < maxNudges {
				if nudgeMsg := getNudgeMessage(msg.Content); nudgeMsg != "" {
					nudgeCount++
					logging.File.Printf("[%s/%s] nudging model: %s (nudge %d/%d)", a.role, a.id[:6], nudgeMsg, nudgeCount, maxNudges)
					a.appendMessage(llm.ChatMessage{
						Role:    llm.RoleUser,
						Content: nudgeMsg,
					})
					continue
				}
			}
			result.Stop = true
			result.Output = msg.Content
			result.ToolCallsCount = totalToolCalls
			return result
		}

		nudgeCount = 0 // reset nudge counter on successful tool call
		totalToolCalls += a.processToolExecution(ctx, msg.ToolCalls)
	}

	// If we reached here, we hit the iteration limit.
	// For planners, this is a graceful yield. For others, it's an error.
	if a.role == RolePlanner {
		logging.File.Printf("[%s/%s] iteration limit (%d) reached, yielding", a.role, a.id[:6], limit)
		result.Output = "Turn limit reached. Yielding to workers/orchestrator."
		result.ToolCallsCount = totalToolCalls
		return result
	}

	result.Error = fmt.Errorf("max inner iterations (%d) reached in agent step", limit)
	return result
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
	logging.File.Printf("[%s/%s] === LLM Request (model: %s, messages: %d, tools: %d) ===",
		a.role, a.id[:6], req.Model, len(req.Messages), len(req.Tools))
	for i, m := range req.Messages {
		content := truncateStr(m.Content, 500)
		if m.ToolCallID != "" {
			logging.File.Printf("[%s/%s]   msg[%d] role=%s tool_call_id=%s: %s", a.role, a.id[:6], i, m.Role, m.ToolCallID, content)
		} else {
			logging.File.Printf("[%s/%s]   msg[%d] role=%s: %s", a.role, a.id[:6], i, m.Role, content)
		}
	}

	return a.client.Complete(ctx, req)
}

// processToolExecution executes tool calls.
func (a *Agent) processToolExecution(ctx context.Context, toolCalls []llm.ToolCall) int {
	totalToolCalls := 0

	for _, tc := range toolCalls {
		logging.File.Printf("[%s/%s] === Tool Call: %s ===", a.role, a.id[:6], tc.Function.Name)
		logging.File.Printf("[%s/%s]   Args: %s", a.role, a.id[:6], truncateStr(tc.Function.Arguments, 1000))

		t, ok := a.tools.Get(tc.Function.Name)
		if !ok {
			errMsg := fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
			logging.File.Printf("[%s/%s]   Result: %s", a.role, a.id[:6], errMsg)
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
			logging.File.Printf("[%s/%s]   Error: %v", a.role, a.id[:6], err)
			return totalToolCalls
		}

		logging.File.Printf("[%s/%s]   Result: %s", a.role, a.id[:6], truncateStr(result, 1000))

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

// PruneMessagesByPrefix removes messages whose content starts with the given prefix.
// This is useful for clearing stale status updates to prevent history bloat.
// It skips the first message (system prompt).
func (a *Agent) PruneMessagesByPrefix(prefix string) {
	if len(a.messages) <= 1 {
		return
	}
	newMsgs := []llm.ChatMessage{a.messages[0]}
	for i := 1; i < len(a.messages); i++ {
		if !strings.HasPrefix(a.messages[i].Content, prefix) {
			newMsgs = append(newMsgs, a.messages[i])
		}
	}
	a.messages = newMsgs
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

// getNudgeMessage returns a specific nudge message if the agent's response
// suggests it should have called a tool but didn't. Returns empty string if no nudge needed.
func getNudgeMessage(content string) string {
	lower := strings.ToLower(content)

	// Case 1: Already has tool calls or termination signals
	if strings.Contains(content, "CODEAGENTS_DONE") ||
		strings.Contains(content, "submit_handoff") ||
		strings.Contains(content, "Task created:") {
		return ""
	}

	// Case 2: Explicit "I'm finished/done" but missing the CODEAGENTS_DONE signal
	donePhrases := []string{
		"project is complete",
		"task is complete",
		"all tasks are complete",
		"successfully implemented",
		"ready for deployment",
		"consider this task complete",
		"we can conclude",
		"project finalized",
	}
	for _, phrase := range donePhrases {
		if strings.Contains(lower, phrase) {
			return "You said the project/task is complete, but you MUST respond with the exact string 'CODEAGENTS_DONE' to terminate the mission. Please do so now."
		}
	}

	// Case 3: Describing actions but no tool calls (Continuation)
	continuationPhrases := []string{
		"i'll use", "i will use",
		"i'll call", "i will call",
		"let me run", "let me list",
		"let me read", "let me edit",
		"let me write", "let me initialize",
		"i need to run", "i need to call",
		"i will create", "i will add",
	}
	for _, phrase := range continuationPhrases {
		if strings.Contains(lower, phrase) {
			return "You described what you want to do, but did not call any tools. Please proceed by calling the appropriate tool NOW. Do not describe the action — perform it using a tool call."
		}
	}

	return ""
}

// parseTextToolCalls attempts to extract tool calls from the model's text output.
// Some models (e.g. SWE-Dev-32B) embed tool calls as text instead of function calls.
// Patterns detected:
//   - <next>tool_name</next> with optional JSON
//   - tool_name({...}) inline
var textToolCallRe = regexp.MustCompile(`<next>\s*(\w+)\s*</next>`)
var inlineToolCallRe = regexp.MustCompile(`(\w+)\(\s*(\{[^}]*\})\s*\)`)

func parseTextToolCalls(content string, registry *tool.Registry) []llm.ToolCall {
	var calls []llm.ToolCall

	// Pattern 1: <next>tool_name</next>
	if matches := textToolCallRe.FindStringSubmatch(content); len(matches) > 1 {
		toolName := matches[1]
		if _, ok := registry.Get(toolName); ok {
			calls = append(calls, llm.ToolCall{
				ID:   generateCallID(),
				Type: "function",
				Function: llm.FunctionCall{
					Name:      toolName,
					Arguments: "{}",
				},
			})
		}
	}

	// Pattern 2: tool_name({...}) inline in text
	if len(calls) == 0 {
		for _, match := range inlineToolCallRe.FindAllStringSubmatch(content, -1) {
			if len(match) > 2 {
				toolName := match[1]
				args := match[2]
				if _, ok := registry.Get(toolName); ok {
					calls = append(calls, llm.ToolCall{
						ID:   generateCallID(),
						Type: "function",
						Function: llm.FunctionCall{
							Name:      toolName,
							Arguments: args,
						},
					})
					break // only take the first match
				}
			}
		}
	}

	return calls
}

func generateCallID() string {
	id, _ := gonanoid.New()
	return "synthetic-" + id
}
