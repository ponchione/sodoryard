package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool is a configurable test double implementing the Tool interface.
type mockTool struct {
	name        string
	description string
	purity      Purity
	schema      json.RawMessage
	executeFn   func(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error)
}

func (m *mockTool) Name() string              { return m.name }
func (m *mockTool) Description() string        { return m.description }
func (m *mockTool) ToolPurity() Purity         { return m.purity }
func (m *mockTool) Schema() json.RawMessage    { return m.schema }
func (m *mockTool) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, projectRoot, input)
	}
	return &ToolResult{Success: true, Content: "ok"}, nil
}

func newMockTool(name string, purity Purity) *mockTool {
	return &mockTool{
		name:        name,
		description: name + " tool",
		purity:      purity,
		schema:      json.RawMessage(`{"name":"` + name + `"}`),
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	m := newMockTool("file_read", Pure)
	reg.Register(m)

	got, ok := reg.Get("file_read")
	if !ok {
		t.Fatal("Get returned false for registered tool")
	}
	if got.Name() != "file_read" {
		t.Fatalf("Get returned tool with name %q, want file_read", got.Name())
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("shell", Mutating))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
	}()
	reg.Register(newMockTool("shell", Mutating))
}

func TestRegistryGetUnknown(t *testing.T) {
	reg := NewRegistry()
	got, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("Get returned true for unregistered tool")
	}
	if got != nil {
		t.Fatal("Get returned non-nil for unregistered tool")
	}
}

func TestRegistryAll(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))
	reg.Register(newMockTool("shell", Mutating))
	reg.Register(newMockTool("git_status", Pure))

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("All() returned %d tools, want 3", len(all))
	}
	// Verify insertion order.
	want := []string{"file_read", "shell", "git_status"}
	for i, name := range want {
		if all[i].Name() != name {
			t.Fatalf("All()[%d].Name() = %q, want %q", i, all[i].Name(), name)
		}
	}
}

func TestRegistrySchemas(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))
	reg.Register(newMockTool("shell", Mutating))

	schemas := reg.Schemas()
	if len(schemas) != 2 {
		t.Fatalf("Schemas() returned %d schemas, want 2", len(schemas))
	}
	// Verify each schema is valid JSON.
	for i, s := range schemas {
		if !json.Valid(s) {
			t.Fatalf("Schemas()[%d] is not valid JSON: %s", i, s)
		}
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("shell", Mutating))
	reg.Register(newMockTool("file_read", Pure))
	reg.Register(newMockTool("git_status", Pure))

	names := reg.Names()
	want := []string{"file_read", "git_status", "shell"} // alphabetical
	if len(names) != len(want) {
		t.Fatalf("Names() returned %d names, want %d", len(names), len(want))
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("Names()[%d] = %q, want %q", i, names[i], name)
		}
	}
}
