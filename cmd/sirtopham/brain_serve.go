package main

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ponchione/sodoryard/internal/brain/mcpserver"
	"github.com/ponchione/sodoryard/internal/brain/vault"
	"github.com/spf13/cobra"
)

func newBrainServeCmd() *cobra.Command {
	var vaultPath string
	cmd := &cobra.Command{
		Use:   "brain-serve",
		Short: "Run the project brain as a standalone MCP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultPath == "" {
				return fmt.Errorf("--vault is required")
			}
			vc, err := vault.New(vaultPath)
			if err != nil {
				return err
			}
			server := mcpserver.NewServer(vc)
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to the Obsidian vault directory")
	return cmd
}
