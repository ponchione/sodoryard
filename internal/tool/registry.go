package tool

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ponchione/sodoryard/internal/provider"
)

// Registry holds all registered tools and provides lookup and enumeration.
// Tools are registered at startup (single-threaded). After initialization,
// the registry is read-only — no mutex needed for concurrent reads.
type Registry struct {
	tools map[string]Tool
	order []string // insertion order for stable All()
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. Panics on duplicate names —
// this catches wiring bugs at startup, not at runtime.
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool: duplicate registration for %q", name))
	}
	r.tools[name] = t
	r.order = append(r.order, name)
}

// Get returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools in insertion order.
func (r *Registry) All() []Tool {
	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name])
	}
	return result
}

// Names returns all registered tool names sorted alphabetically.
func (r *Registry) Names() []string {
	names := make([]string, len(r.order))
	copy(names, r.order)
	sort.Strings(names)
	return names
}

// Schemas returns the JSON Schema definitions from all registered tools,
// suitable for injection into LLM API requests. Returned in insertion order.
func (r *Registry) Schemas() []json.RawMessage {
	result := make([]json.RawMessage, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name].Schema())
	}
	return result
}

// ToolDefinitions converts the registered tools into provider.ToolDefinition
// values suitable for the prompt builder and LLM request. Each tool's Schema()
// JSON is destructured into Name, Description, and InputSchema fields.
//
// Tools whose Schema() cannot be parsed are silently skipped — this catches
// malformed schemas at startup rather than at LLM call time.
func (r *Registry) ToolDefinitions() []provider.ToolDefinition {
	result := make([]provider.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		raw := t.Schema()
		var envelope struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			// Malformed schema — skip rather than panic.
			continue
		}
		result = append(result, provider.ToolDefinition{
			Name:        envelope.Name,
			Description: envelope.Description,
			InputSchema: envelope.InputSchema,
		})
	}
	return result
}
