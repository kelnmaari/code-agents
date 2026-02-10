package tool

import (
	"context"
	"sort"

	"gitlab.alexue4.dev/kelnmaari/code-agent/internal/llm"
)

// Tool is anything an agent can invoke during its loop.
type Tool interface {
	Name() string
	Description() string
	Parameters() interface{} // returns JSON Schema as map[string]interface{}
	Execute(ctx context.Context, args string) (string, error)
}

// Registry holds available tools, keyed by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Panics on duplicate name.
func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; exists {
		panic("duplicate tool registration: " + t.Name() + " is already registered")
	}
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns tool definitions for the LLM ChatRequest.
func (r *Registry) Definitions() []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return sortToolDefinitions(defs)
}

// sortToolDefinitions sorts tool definitions by name.
func sortToolDefinitions(defs []llm.ToolDefinition) []llm.ToolDefinition {
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Function.Name < defs[j].Function.Name
	})
	return defs
}

// Names returns a sorted list of registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
