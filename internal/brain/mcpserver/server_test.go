package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ponchione/sodoryard/internal/brain/vault"
)

func TestServerExposesVaultTools(t *testing.T) {
	ctx := context.Background()
	vc, err := vault.New(t.TempDir())
	if err != nil {
		t.Fatalf("vault.New: %v", err)
	}
	server := NewServer(vc)
	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer cs.Close()

	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "vault_write",
		Arguments: map[string]any{"path": "notes/test.md", "content": "# Test\nMCP"},
	}); err != nil {
		t.Fatalf("vault_write: %v", err)
	}

	readRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "vault_read",
		Arguments: map[string]any{"path": "notes/test.md"},
	})
	if err != nil {
		t.Fatalf("vault_read: %v", err)
	}
	if len(readRes.Content) == 0 {
		t.Fatal("vault_read returned no content")
	}

	searchRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "vault_search",
		Arguments: map[string]any{"query": "mcp", "max_results": 10},
	})
	if err != nil {
		t.Fatalf("vault_search: %v", err)
	}
	if searchRes.StructuredContent == nil {
		t.Fatal("vault_search missing structured content")
	}
}
