package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/vault"
)

type readInput struct {
	Path string `json:"path" jsonschema:"vault-relative document path"`
}

type readOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type writeInput struct {
	Path    string `json:"path" jsonschema:"vault-relative document path"`
	Content string `json:"content" jsonschema:"markdown document content"`
}

type patchInput struct {
	Path      string `json:"path" jsonschema:"vault-relative document path"`
	Operation string `json:"operation" jsonschema:"append, prepend, or replace_section"`
	Content   string `json:"content" jsonschema:"patch payload"`
}

type searchInput struct {
	Query      string `json:"query" jsonschema:"keyword query"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum number of results"`
}

type searchOutput struct {
	Hits []brain.SearchHit `json:"hits"`
}

type listInput struct {
	Directory string `json:"directory,omitempty" jsonschema:"vault-relative directory"`
}

type listOutput struct {
	Paths []string `json:"paths"`
}

func NewServer(vaultClient *vault.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "sirtopham-brain", Version: "v0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "vault_read", Description: "Read a markdown document from the vault"}, func(ctx context.Context, req *mcp.CallToolRequest, input readInput) (*mcp.CallToolResult, readOutput, error) {
		content, err := vaultClient.ReadDocument(ctx, input.Path)
		if err != nil {
			return nil, readOutput{}, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}, readOutput{Path: input.Path, Content: content}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "vault_write", Description: "Create or overwrite a markdown document in the vault"}, func(ctx context.Context, req *mcp.CallToolRequest, input writeInput) (*mcp.CallToolResult, map[string]any, error) {
		if err := vaultClient.WriteDocument(ctx, input.Path, input.Content); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, map[string]any{"path": input.Path}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "vault_patch", Description: "Patch a markdown document in the vault"}, func(ctx context.Context, req *mcp.CallToolRequest, input patchInput) (*mcp.CallToolResult, map[string]any, error) {
		if err := vaultClient.PatchDocument(ctx, input.Path, input.Operation, input.Content); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, map[string]any{"path": input.Path, "operation": input.Operation}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "vault_search", Description: "Keyword search across the vault"}, func(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		hits, err := vaultClient.SearchKeyword(ctx, input.Query, input.MaxResults)
		if err != nil {
			return nil, searchOutput{}, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, searchOutput{Hits: hits}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "vault_list", Description: "List markdown documents in the vault"}, func(ctx context.Context, req *mcp.CallToolRequest, input listInput) (*mcp.CallToolResult, listOutput, error) {
		paths, err := vaultClient.ListDocuments(ctx, input.Directory)
		if err != nil {
			return nil, listOutput{}, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, listOutput{Paths: paths}, nil
	})

	return server
}
