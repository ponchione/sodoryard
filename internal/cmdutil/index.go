package cmdutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexer "github.com/ponchione/sodoryard/internal/brain/indexer"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	"github.com/ponchione/sodoryard/internal/codestore"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	appindex "github.com/ponchione/sodoryard/internal/index"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

type CodeIndexRunner func(context.Context, appindex.Options) (*appindex.Result, error)

type BrainIndexDeps struct {
	BuildBackend func(context.Context, appconfig.BrainConfig, *slog.Logger) (brain.Backend, func(), error)
	OpenStore    func(context.Context, string) (codeintel.Store, error)
	NewEmbedder  func(appconfig.Embedding) codeintel.Embedder
	MarkFresh    func(string, time.Time) error
}

type shunterBrainIndexCleaner interface {
	MarkBrainIndexClean(context.Context, time.Time, string) error
}

type shunterBrainIndexStateReader interface {
	ReadBrainIndexState(context.Context) (projectmemory.BrainIndexState, bool, error)
}

type brainIndexStateMetadata struct {
	Source        string   `json:"source"`
	DocumentPaths []string `json:"document_paths,omitempty"`
}

func DefaultBrainIndexDeps() BrainIndexDeps {
	return BrainIndexDeps{
		BuildBackend: rtpkg.BuildBrainBackend,
		OpenStore:    codestore.Open,
		NewEmbedder:  func(cfg appconfig.Embedding) codeintel.Embedder { return embedder.New(cfg) },
		MarkFresh:    brainindexstate.MarkFresh,
	}
}

func RunCodeIndex(ctx context.Context, configPath string, full bool, runner CodeIndexRunner) (*appindex.Result, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		runner = appindex.Run
	}
	return runner(ctx, appindex.Options{
		ProjectRoot:  cfg.ProjectRoot,
		Full:         full,
		IncludeDirty: true,
		Config:       cfg,
	})
}

func NewCodeIndexCommand(use, short string, configPath *string, runner CodeIndexRunner) *cobra.Command {
	var (
		full    bool
		jsonOut bool
		quiet   bool
	)

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := RunCodeIndex(cmd.Context(), *configPath, full, runner)
			if err != nil {
				return err
			}

			if jsonOut {
				return WriteJSON(cmd.OutOrStdout(), result)
			}
			if quiet {
				return nil
			}
			PrintCodeIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Force a full rebuild of the semantic index")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress human-readable index summary")
	return cmd
}

func RunBrainIndexForConfig(ctx context.Context, configPath string, deps BrainIndexDeps) (brainindexer.Result, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return brainindexer.Result{}, err
	}
	return RunBrainIndex(ctx, cfg, deps)
}

func RunBrainIndex(ctx context.Context, cfg *appconfig.Config, deps BrainIndexDeps) (brainindexer.Result, error) {
	if cfg == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: config is required")
	}
	if !cfg.Brain.Enabled {
		return brainindexer.Result{}, fmt.Errorf("brain index: brain.enabled must be true")
	}
	if deps.BuildBackend == nil || deps.OpenStore == nil || deps.NewEmbedder == nil || deps.MarkFresh == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: dependencies are required")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, cleanup, err := deps.BuildBackend(ctx, cfg.Brain, logger)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: build brain backend: %w", err)
	}
	defer cleanup()
	if backend == nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: brain backend unavailable")
	}

	var (
		result        brainindexer.Result
		previousPaths []string
		currentPaths  []string
	)
	if cfg.Brain.Backend == "shunter" {
		previousPaths, err = previousShunterBrainIndexPaths(ctx, backend)
		if err != nil {
			return brainindexer.Result{}, err
		}
		metadataResult, err := brainindexer.NewMetadata(backend).RebuildProject(ctx, cfg.ProjectRoot, previousPaths)
		if err != nil {
			return brainindexer.Result{}, err
		}
		result = metadataResult.Result
		currentPaths = metadataResult.DocumentPaths
	} else {
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
		previousPaths = make([]string, 0, len(existingDocs))
		for _, doc := range existingDocs {
			previousPaths = append(previousPaths, doc.Path)
		}

		result, err = brainindexer.New(database, backend).RebuildProject(ctx, cfg.ProjectRoot)
		if err != nil {
			return brainindexer.Result{}, err
		}
	}

	store, err := deps.OpenStore(ctx, cfg.BrainLanceDBPath())
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: open brain vectorstore: %w", err)
	}
	defer store.Close()
	semanticResult, err := brainindexer.NewSemantic(backend, store, deps.NewEmbedder(cfg.Embedding)).RebuildProject(ctx, cfg.ProjectName(), previousPaths)
	if err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: semantic rebuild: %w", err)
	}
	indexedAt := time.Now().UTC()
	if cfg.Brain.Backend == "shunter" {
		cleaner, ok := backend.(shunterBrainIndexCleaner)
		if !ok {
			return brainindexer.Result{}, fmt.Errorf("brain index: Shunter backend cannot mark index clean")
		}
		metadataJSON, err := encodeBrainIndexStateMetadata(currentPaths)
		if err != nil {
			return brainindexer.Result{}, fmt.Errorf("brain index: encode Shunter index metadata: %w", err)
		}
		if err := cleaner.MarkBrainIndexClean(ctx, indexedAt, metadataJSON); err != nil {
			return brainindexer.Result{}, fmt.Errorf("brain index: mark Shunter index clean: %w", err)
		}
	} else if err := deps.MarkFresh(cfg.ProjectRoot, indexedAt); err != nil {
		return brainindexer.Result{}, fmt.Errorf("brain index: persist freshness state: %w", err)
	}
	result.SemanticChunksIndexed = semanticResult.SemanticChunksIndexed
	result.SemanticDocumentsDeleted = semanticResult.SemanticDocumentsDeleted
	return result, nil
}

func previousShunterBrainIndexPaths(ctx context.Context, backend brain.Backend) ([]string, error) {
	reader, ok := backend.(shunterBrainIndexStateReader)
	if !ok || reader == nil {
		return nil, fmt.Errorf("brain index: Shunter backend cannot read index state")
	}
	state, found, err := reader.ReadBrainIndexState(ctx)
	if err != nil {
		return nil, fmt.Errorf("brain index: read Shunter index state: %w", err)
	}
	if !found {
		return nil, nil
	}
	paths, err := decodeBrainIndexStateMetadataPaths(state.MetadataJSON)
	if err != nil {
		return nil, fmt.Errorf("brain index: decode Shunter index metadata: %w", err)
	}
	return paths, nil
}

func encodeBrainIndexStateMetadata(paths []string) (string, error) {
	metadata := brainIndexStateMetadata{
		Source:        "brain_index",
		DocumentPaths: normalizeBrainIndexPaths(paths),
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeBrainIndexStateMetadataPaths(metadataJSON string) ([]string, error) {
	metadataJSON = strings.TrimSpace(metadataJSON)
	if metadataJSON == "" {
		return nil, nil
	}
	var metadata brainIndexStateMetadata
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return nil, err
	}
	return normalizeBrainIndexPaths(metadata.DocumentPaths), nil
}

func normalizeBrainIndexPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func PrintCodeIndexSummary(out io.Writer, result *appindex.Result) {
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

func PrintBrainIndexSummary(out io.Writer, result brainindexer.Result) {
	fmt.Fprintln(out, "Brain reindex completed")
	fmt.Fprintf(out, "Brain documents indexed: %d\n", result.DocumentsIndexed)
	fmt.Fprintf(out, "Brain links indexed: %d\n", result.LinksIndexed)
	fmt.Fprintf(out, "Brain documents deleted: %d\n", result.DocumentsDeleted)
	fmt.Fprintf(out, "Brain semantic chunks indexed: %d\n", result.SemanticChunksIndexed)
	fmt.Fprintf(out, "Brain semantic documents deleted: %d\n", result.SemanticDocumentsDeleted)
}

func WriteJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<none>"
	}
	return value
}
