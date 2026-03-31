package tool

import (
	"encoding/json"
	"fmt"
	"sort"
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
