package agent

import (
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
)

// Role defines the agent's role in the hierarchy.
type Role string

const (
	RolePlanner    Role = "planner"
	RoleSubplanner Role = "subplanner"
	RoleWorker     Role = "worker"
)

// RunResult captures the outcome of a single Agent.Step() call.
type RunResult struct {
	// Stop indicates the agent finished (no tool calls in LLM response).
	Stop bool
	// Output is the text content of the last assistant message.
	Output string
	// Error is set on critical failures (LLM errors, context cancel, etc).
	Error error
	// ToolCallsCount is the number of tool calls executed in this Step.
	ToolCallsCount int
	// Usage is the cumulative token consumption for this Step.
	Usage llm.Usage
	// ContextUtilization is the ratio of current message count to maxHistoryMessages (0.0–1.0+).
	ContextUtilization float64
}
