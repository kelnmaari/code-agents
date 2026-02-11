package agent

import (
	"fmt"
	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/logging"
)

// LogLLMRequest logs the details of an LLM request
func LogLLMRequest(role Role, id string, req interface{}) {
	logging.File.Printf("[%s/%s] === LLM Request ===", role, id[:6])
	// Add request details here
}

// LogLLMResponse logs the details of an LLM response
func LogLLMResponse(role Role, id string, resp interface{}) {
	logging.File.Printf("[%s/%s] === LLM Response ===", role, id[:6])
	// Add response details here
}

// LogToolCall logs tool call details
func LogToolCall(role Role, id string, toolCall interface{}) {
	logging.File.Printf("[%s/%s] === Tool Call ===", role, id[:6])
	// Add tool call details here
}
