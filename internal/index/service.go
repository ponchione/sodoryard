package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/codeintel/embedder"
	"github.com/ponchione/sodoryard/internal/codeintel/goparser"
	codegraph "github.com/ponchione/sodoryard/internal/codeintel/graph"
	"github.com/ponchione/sodoryard/internal/codeintel/indexer"
	"github.com/ponchione/sodoryard/internal/codeintel/treesitter"
	"github.com/ponchione/sodoryard/internal/codestore"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/pathglob"
)

var ErrIndexAlreadyRunning = errors.New("index already running for project")

var indexLocks = struct {
	mu     sync.Mutex
	active map[string]struct{}
}{active: map[string]struct{}{}}

type dependencies struct {
	openDB              func(context.Context, string) (*sql.DB, error)
	newStore            func(context.Context, string) (codeintel.Store, error)
	newParser           func(string) (codeintel.Parser, error)
	newEmbedder         func(config.Embedding) codeintel.Embedder
	newDescriber        func(*config.Config) codeintel.Describer
	ensureIndexServices func(context.Context, *config.Config) error
	rebuildGraphIndex   func(context.Context, *config.Config) error
	now                 func() time.Time
}

func defaultDependencies() dependencies {
	return dependencies{
		openDB:   appdb.OpenDB,
		newStore: codestore.Open,
		newParser: func(projectRoot string) (codeintel.Parser, error) {
			parser, err := goparser.New(projectRoot)
			if err != nil {
				return nil, err
			}
			return parser.WithFallback(treesitter.New()), nil
		},
		newEmbedder: func(cfg config.Embedding) codeintel.Embedder {
			return embedder.New(cfg)
		},
		newDescriber:        newRuntimeDescriber,
		ensureIndexServices: runIndexPrecheck,
		rebuildGraphIndex:   rebuildGraphIndex,
		now:                 time.Now,
	}
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	return runWithDependencies(ctx, opts, defaultDependencies())
}

func runWithDependencies(ctx context.Context, opts Options, deps dependencies) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := resolveConfig(opts)
	if err != nil {
		return nil, err
	}

	projectRoot := cfg.ProjectRoot
	if projectRoot == "" {
		projectRoot = opts.ProjectRoot
	}
	if projectRoot == "" {
		return nil, fmt.Errorf("index: project root is required")
	}
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("index: resolve project root: %w", err)
	}
	cfg.ProjectRoot = projectRoot

	if err := acquireProjectLock(projectRoot); err != nil {
		return nil, err
	}
	defer releaseProjectLock(projectRoot)

	if deps.ensureIndexServices != nil {
		if err := deps.ensureIndexServices(ctx, cfg); err != nil {
			return nil, err
		}
	}

	startedAt := deps.now().UTC()
	result := &Result{
		Mode:      modeFromFull(opts.Full),
		StartedAt: startedAt,
	}

	var database *sql.DB
	if cfg.Memory.Backend != "shunter" {
		database, err = deps.openDB(ctx, cfg.DatabasePath())
		if err != nil {
			return nil, fmt.Errorf("index: open database: %w", err)
		}
		defer database.Close()

		if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
			return nil, fmt.Errorf("index: init database schema: %w", err)
		}
		if err := appdb.EnsureContextReportsIncludeTokenBudget(ctx, database); err != nil {
			return nil, fmt.Errorf("index: upgrade context report token budget storage: %w", err)
		}
		if err := ensureProjectRecord(ctx, database, cfg); err != nil {
			return nil, err
		}
	}

	stateStore, err := newStateStore(ctx, database, cfg)
	if err != nil {
		return nil, err
	}
	defer stateStore.Close()
	projectState, fileStates, err := stateStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	currentRevision, err := currentRevision(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	dirtyFiles, err := dirtyTrackedFiles(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	result.WorktreeDirty = len(dirtyFiles) > 0
	result.PreviousRevision = projectState.LastIndexedCommit
	result.CurrentRevision = currentRevision

	currentFiles, filesSeen, skippedFiles, err := scanProjectFiles(cfg)
	if err != nil {
		return nil, err
	}
	result.FilesSeen = filesSeen
	result.SkippedFiles = skippedFiles
	result.FilesSkipped = len(skippedFiles)

	changedFiles, deletedFiles := diffIndexState(currentFiles, fileStates, opts.Full)
	result.ChangedFiles = changedFiles
	result.DeletedFiles = deletedFiles
	result.FilesChanged = len(changedFiles)
	result.FilesDeleted = len(deletedFiles)

	store, err := deps.newStore(ctx, cfg.CodeLanceDBPath())
	if err != nil {
		return nil, fmt.Errorf("index: open vectorstore: %w", err)
	}
	defer store.Close()

	for _, deleted := range deletedFiles {
		if err := store.DeleteByFilePath(ctx, deleted); err != nil {
			return nil, fmt.Errorf("index: delete stale chunks for %s: %w", deleted, err)
		}
	}
	for _, changed := range changedFiles {
		if err := store.DeleteByFilePath(ctx, changed); err != nil {
			return nil, fmt.Errorf("index: clear existing chunks for %s: %w", changed, err)
		}
	}

	indexedStates := make([]fileState, 0, len(changedFiles))
	if len(changedFiles) > 0 {
		parser, err := deps.newParser(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("index: build parser: %w", err)
		}
		var describer codeintel.Describer = noopDescriber{}
		if deps.newDescriber != nil {
			describer = deps.newDescriber(cfg)
		}
		indexResult, err := indexer.IndexFiles(ctx, indexer.IndexConfig{
			ProjectName:     cfg.ProjectName(),
			ProjectRoot:     projectRoot,
			KnownFileHashes: currentFiles,
		}, parser, store, deps.newEmbedder(cfg.Embedding), describer, changedFiles)
		if err != nil {
			return nil, fmt.Errorf("index: run indexer: %w", err)
		}
		result.ChunksWritten = indexResult.TotalChunks
		for _, indexed := range indexResult.Files {
			result.IndexedFiles = append(result.IndexedFiles, indexed.Path)
			indexedStates = append(indexedStates, fileState{
				FilePath:   indexed.Path,
				FileHash:   indexed.FileHash,
				ChunkCount: indexed.ChunkCount,
			})
		}
	}

	if shouldRebuildGraphIndex(cfg, changedFiles, deletedFiles, opts.Full) && deps.rebuildGraphIndex != nil {
		if err := deps.rebuildGraphIndex(ctx, cfg); err != nil {
			return nil, fmt.Errorf("index: rebuild graph index: %w", err)
		}
	}

	finishedAt := deps.now().UTC()
	if err := stateStore.Persist(ctx, currentRevision, finishedAt, indexedStates, deletedFiles); err != nil {
		return nil, err
	}

	result.FinishedAt = finishedAt
	result.Duration = finishedAt.Sub(startedAt)
	return result, nil
}

func resolveConfig(opts Options) (*config.Config, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("index: config is required")
	}
	cfg := *opts.Config
	if opts.ProjectRoot != "" {
		cfg.ProjectRoot = opts.ProjectRoot
	}
	return &cfg, nil
}

func persistSQLiteState(ctx context.Context, db *sql.DB, projectID, revision string, indexedAt time.Time, indexed []fileState, deletedFiles []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index: begin metadata transaction: %w", err)
	}
	defer tx.Rollback()

	if err := deleteFileStates(ctx, tx, projectID, deletedFiles); err != nil {
		return err
	}
	if err := upsertFileStates(ctx, tx, projectID, indexedAt, indexed); err != nil {
		return err
	}
	if err := updateProjectMetadata(ctx, tx, projectID, revision, indexedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("index: commit metadata transaction: %w", err)
	}
	return nil
}

func scanProjectFiles(cfg *config.Config) (map[string]string, int, []string, error) {
	currentFiles := make(map[string]string)
	skipped := make([]string, 0)
	filesSeen := 0
	maxFileSize := cfg.Index.MaxFileSizeBytes
	maxTotalFileSize := int64(cfg.Index.MaxTotalFileSizeBytes)
	var totalFileSize int64

	err := filepath.WalkDir(cfg.ProjectRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(cfg.ProjectRoot, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if d.IsDir() {
			if shouldSkipIndexDir(cfg.Index.Exclude, relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		filesSeen++

		if len(cfg.Index.Include) > 0 && !indexerMatchesAny(cfg.Index.Include, relPath) {
			return nil
		}
		if indexerMatchesAny(cfg.Index.Exclude, relPath) {
			return nil
		}

		var fileSize int64
		if maxFileSize > 0 || maxTotalFileSize > 0 {
			info, err := d.Info()
			if err != nil {
				return err
			}
			fileSize = info.Size()
			if maxFileSize > 0 && fileSize > int64(maxFileSize) {
				skipped = append(skipped, relPath)
				return nil
			}
			if maxTotalFileSize > 0 && totalFileSize+fileSize > maxTotalFileSize {
				skipped = append(skipped, relPath)
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(content) {
			skipped = append(skipped, relPath)
			return nil
		}
		totalFileSize += fileSize
		currentFiles[relPath] = codeintel.ContentHash(string(content))
		return nil
	})
	if err != nil {
		return nil, 0, nil, fmt.Errorf("index: scan project files: %w", err)
	}

	sort.Strings(skipped)
	return currentFiles, filesSeen, skipped, nil
}

func diffIndexState(currentFiles map[string]string, existing map[string]fileState, full bool) ([]string, []string) {
	changed := make([]string, 0)
	deleted := make([]string, 0)

	for path, hash := range currentFiles {
		if full {
			changed = append(changed, path)
			continue
		}
		state, ok := existing[path]
		if !ok || state.FileHash != hash {
			changed = append(changed, path)
		}
	}
	for path := range existing {
		if _, ok := currentFiles[path]; !ok {
			deleted = append(deleted, path)
		}
	}

	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted
}

func indexerMatchesAny(patterns []string, relPath string) bool {
	return pathglob.MatchAny(patterns, relPath)
}

func shouldSkipIndexDir(patterns []string, relPath string) bool {
	if relPath == "" || relPath == "." {
		return false
	}
	if indexerMatchesAny(patterns, relPath) {
		return true
	}
	return indexerMatchesAny(patterns, relPath+"/.index-dir-probe")
}

func acquireProjectLock(projectRoot string) error {
	indexLocks.mu.Lock()
	defer indexLocks.mu.Unlock()
	if _, ok := indexLocks.active[projectRoot]; ok {
		return ErrIndexAlreadyRunning
	}
	indexLocks.active[projectRoot] = struct{}{}
	return nil
}

func releaseProjectLock(projectRoot string) {
	indexLocks.mu.Lock()
	defer indexLocks.mu.Unlock()
	delete(indexLocks.active, projectRoot)
}

func modeFromFull(full bool) string {
	if full {
		return "full"
	}
	return "incremental"
}

func shouldRebuildGraphIndex(cfg *config.Config, changedFiles []string, deletedFiles []string, full bool) bool {
	if full || len(changedFiles) > 0 || len(deletedFiles) > 0 {
		return true
	}
	_, err := os.Stat(cfg.GraphDBPath())
	return err != nil
}

func rebuildGraphIndex(ctx context.Context, cfg *config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(cfg.GraphDBPath()), 0o755); err != nil {
		return fmt.Errorf("ensure graph state dir: %w", err)
	}
	store, err := codegraph.NewStore(cfg.GraphDBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.DropAndRecreate(); err != nil {
		return err
	}
	analyzerCfg := codegraph.DefaultAnalyzerConfig()
	resolver := codegraph.NewResolverWithIndexRules(cfg.ProjectRoot, &analyzerCfg, cfg.Index.Include, cfg.Index.Exclude)
	result, err := resolver.Analyze()
	if err != nil {
		return err
	}
	if err := store.StoreAnalysisResult(result); err != nil {
		return err
	}
	return store.SetMeta("project_root", cfg.ProjectRoot)
}

type noopDescriber struct{}

func (noopDescriber) DescribeFile(context.Context, string, string) ([]codeintel.Description, error) {
	return nil, nil
}
