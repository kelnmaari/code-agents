package agent

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
}
