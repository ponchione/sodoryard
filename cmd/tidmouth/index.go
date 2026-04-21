package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	brainindexer "github.com/ponchione/sodoryard/internal/brain/indexer"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	"github.com/ponchione/sodoryard/internal/codestore"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	appindex "github.com/ponchione/sodoryard/internal/index"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

var runIndexService = appindex.Run
var runBrainIndexCommand = runBrainIndex
var openBrainVectorStore = codestore.Open
var newBrainEmbedder = func(cfg appconfig.Embedding) codeintel.Embedder { return embedder.New(cfg) }
var buildBrainIndexBackend = rtpkg.BuildBrainBackend
var markBrainIndexFresh = brainindexstate.MarkFresh

func newIndexCmd(configPath *string) *cobra.Command {
	var (
		full    bool
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build backend retrieval indexes for internal engine use",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			result, err := runIndexService(cmd.Context(), appindex.Options{
				ProjectRoot:  cfg.ProjectRoot,
				Full:         full,
				IncludeDirty: true,
				Config:       cfg,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			printIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Force a full rebuild of the semantic index")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.AddCommand(newIndexBrainCmd(configPath))
	return cmd
}

func newIndexBrainCmd(configPath *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "brain",
		Short: "Rebuild derived brain metadata for internal engine use",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			result, err := runBrainIndexCommand(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			printBrainIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func runBrainIndex(ctx context.Context, cfg *appconfig.Config) (brainindexer.Result, error) {
	if cfg == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: config is required")
	}
	if !cfg.Brain.Enabled {
		return brainindexer.Result{}, fmt.Errorf("brain index: brain.enabled must be true")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, cleanup, err := buildBrainIndexBackend(ctx, cfg.Brain, logger)
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

	store, err := openBrainVectorStore(ctx, cfg.BrainLanceDBPath())
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: open brain vectorstore: %w", err)
	}
	defer store.Close()
	semanticResult, err := brainindexer.NewSemantic(backend, store, newBrainEmbedder(cfg.Embedding)).RebuildProject(ctx, cfg.ProjectName(), previousPaths)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: semantic rebuild: %w", err)
	}
	if err := markBrainIndexFresh(cfg.ProjectRoot, time.Now().UTC()); err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: persist freshness state: %w", err)
	}
	result.SemanticChunksIndexed = semanticResult.SemanticChunksIndexed
	result.SemanticDocumentsDeleted = semanticResult.SemanticDocumentsDeleted
	return result, nil
}

func printIndexSummary(out io.Writer, result *appindex.Result) {
	if result == nil {
		fmt.Fprintln(out, "index completed")
		return
	}
	fmt.Fprintf(out, "Mode: %s\n", result.Mode)
	fmt.Fprintf(out, "Previous revision: %s\n", displayValue(result.PreviousRevision))
	fmt.Fprintf(out, "Current revision: %s\n", displayValue(result.CurrentRevision))
	fmt.Fprintf(out, "Changed files: %d\n", result.FilesChanged)
	fmt.Fprintf(out, "Deleted files: %d\n", result.FilesDeleted)
	fmt.Fprintf(out, "Skipped files: %d\n", result.FilesSkipped)
	fmt.Fprintf(out, "Chunks written: %d\n", result.ChunksWritten)
	fmt.Fprintf(out, "Worktree dirty: %t\n", result.WorktreeDirty)
	fmt.Fprintf(out, "Duration: %s\n", result.Duration.Round(10_000_000))
	if len(result.IndexedFiles) > 0 {
		fmt.Fprintf(out, "Indexed files: %s\n", strings.Join(result.IndexedFiles, ", "))
	}
}

func printBrainIndexSummary(out io.Writer, result brainindexer.Result) {
	fmt.Fprintln(out, "Brain reindex completed")
	fmt.Fprintf(out, "Brain documents indexed: %d\n", result.DocumentsIndexed)
	fmt.Fprintf(out, "Brain links indexed: %d\n", result.LinksIndexed)
	fmt.Fprintf(out, "Brain documents deleted: %d\n", result.DocumentsDeleted)
	fmt.Fprintf(out, "Brain semantic chunks indexed: %d\n", result.SemanticChunksIndexed)
	fmt.Fprintf(out, "Brain semantic documents deleted: %d\n", result.SemanticDocumentsDeleted)
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<none>"
	}
	return value
}
