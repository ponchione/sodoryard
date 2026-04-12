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

func (m *mockTool) Name() string            { return m.name }
func (m *mockTool) Description() string     { return m.description }
func (m *mockTool) ToolPurity() Purity      { return m.purity }
func (m *mockTool) Schema() json.RawMessage { return m.schema }
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

func TestRegistryToolDefinitions(t *testing.T) {
	reg := NewRegistry()

	// Register tools with proper Anthropic-style Schema() JSON.
	readTool := &mockTool{
		name:        "file_read",
		description: "Read a file",
		purity:      Pure,
		schema: json.RawMessage(`{
			"name": "file_read",
			"description": "Read file contents",
			"input_schema": {
				"type": "object",
				"properties": {
					"path": {"type": "string"}
				},
				"required": ["path"]
			}
		}`),
	}
	shellTool := &mockTool{
		name:        "shell",
		description: "Run a shell command",
		purity:      Mutating,
		schema: json.RawMessage(`{
			"name": "shell",
			"description": "Execute a shell command",
			"input_schema": {
				"type": "object",
				"properties": {
					"command": {"type": "string"}
				},
				"required": ["command"]
			}
		}`),
	}

	reg.Register(readTool)
	reg.Register(shellTool)

	defs := reg.ToolDefinitions()
	if len(defs) != 2 {
		t.Fatalf("ToolDefinitions() returned %d, want 2", len(defs))
	}

	// Verify first tool.
	if defs[0].Name != "file_read" {
		t.Errorf("defs[0].Name = %q, want file_read", defs[0].Name)
	}
	if defs[0].Description != "Read file contents" {
		t.Errorf("defs[0].Description = %q, want 'Read file contents'", defs[0].Description)
	}
	if defs[0].InputSchema == nil {
		t.Error("defs[0].InputSchema is nil")
	}

	// Verify second tool.
	if defs[1].Name != "shell" {
		t.Errorf("defs[1].Name = %q, want shell", defs[1].Name)
	}

	// Verify InputSchema is valid JSON with expected structure.
	var schema map[string]interface{}
	if err := json.Unmarshal(defs[0].InputSchema, &schema); err != nil {
		t.Fatalf("defs[0].InputSchema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("InputSchema type = %v, want object", schema["type"])
	}
}

func TestRegistryToolDefinitionsMalformedSchema(t *testing.T) {
	reg := NewRegistry()

	// Tool with valid schema.
	goodTool := &mockTool{
		name:   "good",
		purity: Pure,
		schema: json.RawMessage(`{"name":"good","description":"A good tool","input_schema":{"type":"object"}}`),
	}
	// Tool with malformed JSON schema.
	badTool := &mockTool{
		name:   "bad",
		purity: Pure,
		schema: json.RawMessage(`{not valid json`),
	}

	reg.Register(goodTool)
	reg.Register(badTool)

	defs := reg.ToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("ToolDefinitions() returned %d, want 1 (bad tool should be skipped)", len(defs))
	}
	if defs[0].Name != "good" {
		t.Errorf("defs[0].Name = %q, want good", defs[0].Name)
	}
}

func TestRegistryToolDefinitionsWithRealTools(t *testing.T) {
	// Verify that the actual tool implementations produce valid ToolDefinitions.
	reg := NewRegistry()
	RegisterFileTools(reg)

	defs := reg.ToolDefinitions()
	if len(defs) != 3 {
		t.Fatalf("ToolDefinitions() with file tools returned %d, want 3", len(defs))
	}

	// All definitions should have non-empty names and descriptions.
	for i, d := range defs {
		if d.Name == "" {
			t.Errorf("defs[%d].Name is empty", i)
		}
		if d.Description == "" {
			t.Errorf("defs[%d].Description is empty", i)
		}
		if d.InputSchema == nil {
			t.Errorf("defs[%d].InputSchema is nil", i)
		}
		if !json.Valid(d.InputSchema) {
			t.Errorf("defs[%d].InputSchema is not valid JSON", i)
		}
	}
}

func TestRegisterFileReadToolsRegistersOnlyFileRead(t *testing.T) {
	reg := NewRegistry()
	RegisterFileReadTools(reg)

	got := reg.Names()
	want := []string{"file_read"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestRegisterFileWriteToolsRegistersOnlyMutatingFileTools(t *testing.T) {
	reg := NewRegistry()
	RegisterFileWriteTools(reg)

	got := reg.Names()
	want := []string{"file_edit", "file_write"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestRegisterDirectoryTools(t *testing.T) {
	reg := NewRegistry()
	RegisterDirectoryTools(reg)

	if _, ok := reg.Get("list_directory"); !ok {
		t.Fatal("list_directory not registered")
	}
	if _, ok := reg.Get("find_files"); !ok {
		t.Fatal("find_files not registered")
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
