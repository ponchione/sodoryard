package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	brainindexer "github.com/ponchione/sodoryard/internal/brain/indexer"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/brain/mcpserver"
	"github.com/ponchione/sodoryard/internal/brain/vault"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	"github.com/ponchione/sodoryard/internal/codestore"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

func newYardBrainCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brain",
		Short: "Brain operations (index, serve)",
	}
	cmd.AddCommand(newYardBrainIndexCmd(configPath), newYardBrainServeCmd())
	return cmd
}

func newYardBrainIndexCmd(configPath *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild derived brain metadata from the vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			result, err := yardRunBrainIndex(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			yardPrintBrainIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func yardRunBrainIndex(ctx context.Context, cfg *appconfig.Config) (brainindexer.Result, error) {
	if cfg == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: config is required")
	}
	if !cfg.Brain.Enabled {
		return brainindexer.Result{}, fmt.Errorf("brain index: brain.enabled must be true")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, cleanup, err := rtpkg.BuildBrainBackend(ctx, cfg.Brain, logger)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: build brain backend: %w", err)
	}
	defer cleanup()
	if backend == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: brain backend unavailable")
	}

	database, err := appdb.OpenDB(ctx, cfg.DatabasePath())
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: open database: %w", err)
	}
	defer database.Close()
	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: init database schema: %w", err)
	}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: ensure project record: %w", err)
	}

	queries := appdb.New(database)
	existingDocs, err := queries.ListBrainDocumentsByProject(ctx, cfg.ProjectRoot)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: list existing brain documents: %w", err)
	}
	previousPaths := make([]string, 0, len(existingDocs))
	for _, doc := range existingDocs {
		previousPaths = append(previousPaths, doc.Path)
	}

	result, err := brainindexer.New(database, backend).RebuildProject(ctx, cfg.ProjectRoot)
	if err != nil {
		return brainindexer.Result{}, err
	}

	store, err := codestore.Open(ctx, cfg.BrainLanceDBPath())
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: open brain vectorstore: %w", err)
	}
	defer store.Close()
	semanticResult, err := brainindexer.NewSemantic(backend, store, embedder.New(cfg.Embedding)).RebuildProject(ctx, cfg.ProjectName(), previousPaths)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: semantic rebuild: %w", err)
	}
	if err := brainindexstate.MarkFresh(cfg.ProjectRoot, time.Now().UTC()); err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: persist freshness state: %w", err)
	}
	result.SemanticChunksIndexed = semanticResult.SemanticChunksIndexed
	result.SemanticDocumentsDeleted = semanticResult.SemanticDocumentsDeleted
	return result, nil
}

func yardPrintBrainIndexSummary(out io.Writer, result brainindexer.Result) {
	fmt.Fprintln(out, "Brain reindex completed")
	fmt.Fprintf(out, "Brain documents indexed: %d\n", result.DocumentsIndexed)
	fmt.Fprintf(out, "Brain links indexed: %d\n", result.LinksIndexed)
	fmt.Fprintf(out, "Brain documents deleted: %d\n", result.DocumentsDeleted)
	fmt.Fprintf(out, "Brain semantic chunks indexed: %d\n", result.SemanticChunksIndexed)
	fmt.Fprintf(out, "Brain semantic documents deleted: %d\n", result.SemanticDocumentsDeleted)
}

func newYardBrainServeCmd() *cobra.Command {
	var vaultPath string
	cmd := &cobra.Command{
		Use:   "serve",
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
