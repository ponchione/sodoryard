package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/mcpserver"
	"github.com/ponchione/sodoryard/internal/brain/vault"
)

type Client struct {
	session       *mcp.ClientSession
	serverSession *mcp.ServerSession
}

type readResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type searchResult struct {
	Hits []brain.SearchHit `json:"hits"`
}

type listResult struct {
	Paths []string `json:"paths"`
}

func Connect(ctx context.Context, vaultPath string) (*Client, error) {
	vc, err := vault.New(vaultPath)
	if err != nil {
		return nil, err
	}
	server := mcpserver.NewServer(vc)
	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	if err != nil {
		return nil, fmt.Errorf("connect brain MCP server: %w", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sodoryard", Version: "v0.1.0"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		ss.Close()
		return nil, fmt.Errorf("connect brain MCP client: %w", err)
	}
	return &Client{session: cs, serverSession: ss}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var firstErr error
	if c.session != nil {
		if err := c.session.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if c.serverSession != nil {
		if err := c.serverSession.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Client) ReadDocument(ctx context.Context, path string) (string, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: "vault_read", Arguments: map[string]any{"path": path}})
	if err != nil {
		return "", err
	}
	if toolErr := toolResultError(res); toolErr != nil {
		return "", toolErr
	}
	var out readResult
	if err := decodeStructured(res.StructuredContent, &out); err != nil {
		return "", err
	}
	return out.Content, nil
}

func (c *Client) WriteDocument(ctx context.Context, path string, content string) error {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: "vault_write", Arguments: map[string]any{"path": path, "content": content}})
	if err != nil {
		return err
	}
	return toolResultError(res)
}

func (c *Client) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: "vault_patch", Arguments: map[string]any{"path": path, "operation": operation, "content": content}})
	if err != nil {
		return err
	}
	return toolResultError(res)
}

func (c *Client) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return c.SearchKeywordLimit(ctx, query, 10)
}

func (c *Client) SearchKeywordLimit(ctx context.Context, query string, maxResults int) ([]brain.SearchHit, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: "vault_search", Arguments: map[string]any{"query": query, "max_results": maxResults}})
	if err != nil {
		return nil, err
	}
	if toolErr := toolResultError(res); toolErr != nil {
		return nil, toolErr
	}
	var out searchResult
	if err := decodeStructured(res.StructuredContent, &out); err != nil {
		return nil, err
	}
	return out.Hits, nil
}

func (c *Client) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: "vault_list", Arguments: map[string]any{"directory": directory}})
	if err != nil {
		return nil, err
	}
	if toolErr := toolResultError(res); toolErr != nil {
		return nil, toolErr
	}
	var out listResult
	if err := decodeStructured(res.StructuredContent, &out); err != nil {
		return nil, err
	}
	return out.Paths, nil
}

// toolResultError converts an MCP tool result carrying IsError=true into a Go
// error containing the handler-supplied message. Returns nil if the result
// reports no error (or is nil). The handler-side text content is preserved so
// callers that inspect error substrings (for example, "Document not found")
// continue to work.
func toolResultError(res *mcp.CallToolResult) error {
	if res == nil || !res.IsError {
		return nil
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("mcp tool call returned error result")
	}
	return fmt.Errorf("%s", strings.Join(parts, "\n"))
}

func decodeStructured(src any, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("marshal structured content: %w", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("unmarshal structured content: %w", err)
	}
	return nil
}
