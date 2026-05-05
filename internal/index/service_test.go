//go:build sqlite_fts5
// +build sqlite_fts5

package index

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

type fakeStore struct {
	deleted []string
	upserts [][]codeintel.Chunk
}

func (f *fakeStore) Upsert(_ context.Context, chunks []codeintel.Chunk) error {
	copied := append([]codeintel.Chunk(nil), chunks...)
	f.upserts = append(f.upserts, copied)
	return nil
}
func (f *fakeStore) VectorSearch(context.Context, []float32, int, codeintel.Filter) ([]codeintel.SearchResult, error) {
	return nil, nil
}
func (f *fakeStore) GetByFilePath(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (f *fakeStore) GetByName(context.Context, string) ([]codeintel.Chunk, error) { return nil, nil }
func (f *fakeStore) DeleteByFilePath(_ context.Context, filePath string) error {
	f.deleted = append(f.deleted, filePath)
	return nil
}
func (f *fakeStore) Close() error { return nil }

type fakeParser struct{}

func (fakeParser) Parse(_ string, content []byte) ([]codeintel.RawChunk, error) {
	return []codeintel.RawChunk{{
		Name:      "Example",
		Signature: "func Example()",
		Body:      string(content),
		ChunkType: codeintel.ChunkTypeFunction,
		LineStart: 1,
		LineEnd:   3,
	}}, nil
}

type fakeEmbedder struct{}

func (fakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}
func (fakeEmbedder) EmbedQuery(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

type fakeDescriber struct {
	called int
}

func (f *fakeDescriber) DescribeFile(context.Context, string, string) ([]codeintel.Description, error) {
	f.called++
	return []codeintel.Description{{Name: "Example", Description: "Semantic example description."}}, nil
}

func TestRunWithDependenciesIndexesIncrementallyAndDeletesRemovedFiles(t *testing.T) {
	projectRoot := t.TempDir()
	writeTestFile(t, projectRoot, "main.go", "package main\n\nfunc Example() {}\n")

	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Index.Include = []string{"**/*.go"}
	cfg.Index.Exclude = []string{"**/.git/**"}
	cfg.Index.MaxFileSizeBytes = 1024 * 1024

	store := &fakeStore{}
	describer := &fakeDescriber{}
	graphRebuilds := 0
	now := time.Date(2026, 4, 2, 17, 0, 0, 0, time.UTC)
	deps := dependencies{
		openDB: appdb.OpenDB,
		newStore: func(context.Context, string) (codeintel.Store, error) {
			return store, nil
		},
		newParser: func(string) (codeintel.Parser, error) {
			return fakeParser{}, nil
		},
		newEmbedder: func(config.Embedding) codeintel.Embedder {
			return fakeEmbedder{}
		},
		newDescriber: func(*config.Config) codeintel.Describer {
			return describer
		},
		ensureIndexServices: func(context.Context, *config.Config) error {
			return nil
		},
		rebuildGraphIndex: func(_ context.Context, cfg *config.Config) error {
			graphRebuilds++
			if err := os.MkdirAll(filepath.Dir(cfg.GraphDBPath()), 0o755); err != nil {
				return err
			}
			return os.WriteFile(cfg.GraphDBPath(), []byte("graph"), 0o644)
		},
		now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}

	first, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Mode != "incremental" {
		t.Fatalf("Mode = %q, want incremental", first.Mode)
	}
	if first.FilesChanged != 1 || len(first.IndexedFiles) != 1 || first.IndexedFiles[0] != "main.go" {
		t.Fatalf("first result = %+v, want one indexed main.go", first)
	}
	if first.ChunksWritten != 1 {
		t.Fatalf("ChunksWritten = %d, want 1", first.ChunksWritten)
	}
	if describer.called != 1 {
		t.Fatalf("describer called %d times, want 1", describer.called)
	}
	if got := store.upserts[0][0].Description; got != "Semantic example description." {
		t.Fatalf("stored chunk description = %q, want semantic describer output", got)
	}
	if graphRebuilds != 1 {
		t.Fatalf("graph rebuilds after first run = %d, want 1", graphRebuilds)
	}

	db := mustOpenDB(t, cfg.DatabasePath())
	defer db.Close()
	assertIndexStateRow(t, db, cfg.ProjectRoot, "main.go", 1)

	writeTestFile(t, projectRoot, "main.go", "package main\n\nfunc Example() { println(1) }\n")
	second, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.FilesChanged != 1 || len(second.DeletedFiles) != 0 {
		t.Fatalf("second result = %+v, want one changed file and no deletions", second)
	}
	if graphRebuilds != 2 {
		t.Fatalf("graph rebuilds after second run = %d, want 2", graphRebuilds)
	}

	if err := os.Remove(filepath.Join(projectRoot, "main.go")); err != nil {
		t.Fatalf("remove main.go: %v", err)
	}
	third, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if third.FilesDeleted != 1 || len(third.DeletedFiles) != 1 || third.DeletedFiles[0] != "main.go" {
		t.Fatalf("third result = %+v, want deleted main.go", third)
	}
	if len(store.deleted) < 3 {
		t.Fatalf("DeleteByFilePath calls = %v, want deletes for replace/delete flow", store.deleted)
	}
	if graphRebuilds != 3 {
		t.Fatalf("graph rebuilds after third run = %d, want 3", graphRebuilds)
	}
	assertIndexStateMissing(t, db, cfg.ProjectRoot, "main.go")

	fourth, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("fourth run: %v", err)
	}
	if fourth.FilesChanged != 0 || fourth.FilesDeleted != 0 {
		t.Fatalf("fourth result = %+v, want no-op run", fourth)
	}
	if graphRebuilds != 3 {
		t.Fatalf("graph rebuilds after no-op run = %d, want unchanged 3", graphRebuilds)
	}
}

func TestRunWithDependenciesUsesShunterCodeIndexState(t *testing.T) {
	projectRoot := t.TempDir()
	writeTestFile(t, projectRoot, "main.go", "package main\n\nfunc Example() {}\n")

	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Index.Include = []string{"**/*.go"}
	cfg.Index.Exclude = []string{"**/.git/**"}
	cfg.Index.MaxFileSizeBytes = 1024 * 1024
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true

	store := &fakeStore{}
	now := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
	deps := dependencies{
		openDB: appdb.OpenDB,
		newStore: func(context.Context, string) (codeintel.Store, error) {
			return store, nil
		},
		newParser: func(string) (codeintel.Parser, error) {
			return fakeParser{}, nil
		},
		newEmbedder: func(config.Embedding) codeintel.Embedder {
			return fakeEmbedder{}
		},
		newDescriber: func(*config.Config) codeintel.Describer {
			return noopDescriber{}
		},
		ensureIndexServices: func(context.Context, *config.Config) error {
			return nil
		},
		rebuildGraphIndex: func(context.Context, *config.Config) error {
			return nil
		},
		now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}

	first, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.FilesChanged != 1 || len(first.IndexedFiles) != 1 || first.IndexedFiles[0] != "main.go" {
		t.Fatalf("first result = %+v, want indexed main.go", first)
	}
	db := mustOpenDB(t, cfg.DatabasePath())
	defer db.Close()
	assertIndexStateMissing(t, db, cfg.ProjectRoot, "main.go")

	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{DataDir: cfg.Memory.ShunterDataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	codeState, found, err := backend.ReadCodeIndexState(context.Background())
	if err != nil {
		t.Fatalf("ReadCodeIndexState: %v", err)
	}
	if !found || codeState.LastIndexedAtUS == 0 || codeState.Dirty {
		t.Fatalf("code state = %+v found=%t, want clean indexed state", codeState, found)
	}
	fileStates, err := backend.ListCodeFileIndexStates(context.Background())
	if err != nil {
		t.Fatalf("ListCodeFileIndexStates: %v", err)
	}
	if len(fileStates) != 1 || fileStates[0].FilePath != "main.go" || fileStates[0].ChunkCount != 1 {
		t.Fatalf("file states = %+v, want main.go state", fileStates)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := runWithDependencies(context.Background(), Options{Config: cfg}, deps)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.FilesChanged != 0 || second.FilesDeleted != 0 {
		t.Fatalf("second result = %+v, want no-op from Shunter state", second)
	}
}

func TestAcquireProjectLockRejectsOverlap(t *testing.T) {
	projectRoot := "/tmp/lock-test"
	if err := acquireProjectLock(projectRoot); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer releaseProjectLock(projectRoot)
	if err := acquireProjectLock(projectRoot); err != ErrIndexAlreadyRunning {
		t.Fatalf("second lock error = %v, want %v", err, ErrIndexAlreadyRunning)
	}
}

func TestScanProjectFilesPrunesExcludedDirectories(t *testing.T) {
	projectRoot := t.TempDir()
	writeTestFile(t, projectRoot, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, projectRoot, ".yard/lancedb/code/ignored.go", "package ignored\n")
	writeTestFile(t, projectRoot, "web/node_modules/pkg/ignored.go", "package ignored\n")

	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Index.Include = []string{"**/*.go"}
	cfg.Index.Exclude = []string{"**/.yard/**", "**/node_modules/**"}

	files, filesSeen, skipped, err := scanProjectFiles(cfg)
	if err != nil {
		t.Fatalf("scanProjectFiles: %v", err)
	}
	if filesSeen != 1 {
		t.Fatalf("filesSeen = %d, want 1 visited file outside excluded directories", filesSeen)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v, want only main.go", files)
	}
	if _, ok := files["main.go"]; !ok {
		t.Fatalf("files = %v, want main.go", files)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}
}

func TestScanProjectFilesEnforcesTotalFileSizeBudget(t *testing.T) {
	projectRoot := t.TempDir()
	writeTestFile(t, projectRoot, "a.go", "1234567890")
	writeTestFile(t, projectRoot, "b.go", "abcdefghij")
	writeTestFile(t, projectRoot, "c.go", "klmnopqrst")

	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Index.Include = []string{"**/*.go"}
	cfg.Index.Exclude = nil
	cfg.Index.MaxFileSizeBytes = 100
	cfg.Index.MaxTotalFileSizeBytes = 15

	files, filesSeen, skipped, err := scanProjectFiles(cfg)
	if err != nil {
		t.Fatalf("scanProjectFiles: %v", err)
	}
	if filesSeen != 3 {
		t.Fatalf("filesSeen = %d, want 3", filesSeen)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v, want one file within total budget", files)
	}
	if _, ok := files["a.go"]; !ok {
		t.Fatalf("files = %v, want a.go", files)
	}
	wantSkipped := []string{"b.go", "c.go"}
	if !slices.Equal(skipped, wantSkipped) {
		t.Fatalf("skipped = %v, want %v", skipped, wantSkipped)
	}
}

func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func mustOpenDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := appdb.OpenDB(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenDB(%s): %v", path, err)
	}
	return db
}

func assertIndexStateRow(t *testing.T, db *sql.DB, projectID, filePath string, chunkCount int) {
	t.Helper()
	var gotCount int
	if err := db.QueryRow(`SELECT chunk_count FROM index_state WHERE project_id = ? AND file_path = ?`, projectID, filePath).Scan(&gotCount); err != nil {
		t.Fatalf("query index_state(%s): %v", filePath, err)
	}
	if gotCount != chunkCount {
		t.Fatalf("chunk_count = %d, want %d", gotCount, chunkCount)
	}
}

func assertIndexStateMissing(t *testing.T, db *sql.DB, projectID, filePath string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM index_state WHERE project_id = ? AND file_path = ?`, projectID, filePath).Scan(&count); err != nil {
		t.Fatalf("count index_state(%s): %v", filePath, err)
	}
	if count != 0 {
		t.Fatalf("index_state count = %d, want 0", count)
	}
}
