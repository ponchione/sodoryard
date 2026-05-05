package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexer "github.com/ponchione/sodoryard/internal/brain/indexer"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/codeintel"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appindex "github.com/ponchione/sodoryard/internal/index"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestIndexCommandPassesFlagsToService(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runIndexService
	defer func() { runIndexService = original }()

	var gotOpts appindex.Options
	runIndexService = func(_ context.Context, opts appindex.Options) (*appindex.Result, error) {
		gotOpts = opts
		return &appindex.Result{Mode: "full", Duration: time.Second}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !gotOpts.Full {
		t.Fatal("expected Full=true")
	}
	if !gotOpts.IncludeDirty {
		t.Fatal("expected IncludeDirty=true")
	}
	if gotOpts.Config == nil || gotOpts.Config.ProjectRoot != projectRoot {
		t.Fatalf("Config.ProjectRoot = %v, want %s", gotOpts.Config, projectRoot)
	}
}

func TestIndexCommandJSONOutput(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runIndexService
	defer func() { runIndexService = original }()

	runIndexService = func(context.Context, appindex.Options) (*appindex.Result, error) {
		return &appindex.Result{Mode: "incremental", FilesChanged: 2}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result appindex.Result
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%s", err, buf.String())
	}
	if result.Mode != "incremental" || result.FilesChanged != 2 {
		t.Fatalf("result = %+v, want incremental/2", result)
	}
}

func TestIndexBrainSubcommandPassesConfigAndPrintsSummary(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: true\n  vault_path: " + vaultPath + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runBrainIndexCommand
	defer func() { runBrainIndexCommand = original }()

	var gotCfg *appconfig.Config
	runBrainIndexCommand = func(_ context.Context, cfg *appconfig.Config) (brainindexer.Result, error) {
		gotCfg = cfg
		return brainindexer.Result{DocumentsIndexed: 3, LinksIndexed: 5, DocumentsDeleted: 1, SemanticChunksIndexed: 7, SemanticDocumentsDeleted: 1}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"brain"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotCfg == nil {
		t.Fatal("expected brain reindex config to be passed")
	}
	if gotCfg.ProjectRoot != projectRoot {
		t.Fatalf("ProjectRoot = %q, want %q", gotCfg.ProjectRoot, projectRoot)
	}
	if !gotCfg.Brain.Enabled {
		t.Fatal("expected Brain.Enabled=true")
	}
	if gotCfg.Brain.VaultPath != vaultPath {
		t.Fatalf("Brain.VaultPath = %q, want %q", gotCfg.Brain.VaultPath, vaultPath)
	}
	output := buf.String()
	for _, want := range []string{
		"Brain reindex completed",
		"Brain documents indexed: 3",
		"Brain links indexed: 5",
		"Brain documents deleted: 1",
		"Brain semantic chunks indexed: 7",
		"Brain semantic documents deleted: 1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput=%s", want, output)
		}
	}
}

func TestIndexBrainSubcommandJSONOutput(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: true\n  vault_path: " + vaultPath + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runBrainIndexCommand
	defer func() { runBrainIndexCommand = original }()

	runBrainIndexCommand = func(context.Context, *appconfig.Config) (brainindexer.Result, error) {
		return brainindexer.Result{DocumentsIndexed: 2, LinksIndexed: 4, DocumentsDeleted: 1, SemanticChunksIndexed: 6, SemanticDocumentsDeleted: 1}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"brain", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result brainindexer.Result
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%s", err, buf.String())
	}
	if result.DocumentsIndexed != 2 || result.LinksIndexed != 4 || result.DocumentsDeleted != 1 || result.SemanticChunksIndexed != 6 || result.SemanticDocumentsDeleted != 1 {
		t.Fatalf("result = %+v, want 2/4/1 plus semantic 6/1", result)
	}
}

type fakeBrainIndexBackend struct {
	docs map[string]string
}

func (f fakeBrainIndexBackend) ReadDocument(_ context.Context, path string) (string, error) {
	content, ok := f.docs[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return content, nil
}
func (fakeBrainIndexBackend) WriteDocument(context.Context, string, string) error         { return nil }
func (fakeBrainIndexBackend) PatchDocument(context.Context, string, string, string) error { return nil }
func (fakeBrainIndexBackend) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}
func (f fakeBrainIndexBackend) ListDocuments(context.Context, string) ([]string, error) {
	paths := make([]string, 0, len(f.docs))
	for path := range f.docs {
		paths = append(paths, path)
	}
	return paths, nil
}

type fakeShunterBrainIndexBackend struct {
	fakeBrainIndexBackend
	cleanAt      time.Time
	cleanMeta    string
	cleanCalled  bool
	cleanCallErr error
	state        projectmemory.BrainIndexState
	stateFound   bool
	stateErr     error
}

func (f *fakeShunterBrainIndexBackend) ReadBrainIndexState(context.Context) (projectmemory.BrainIndexState, bool, error) {
	return f.state, f.stateFound, f.stateErr
}

func (f *fakeShunterBrainIndexBackend) MarkBrainIndexClean(_ context.Context, indexedAt time.Time, metadataJSON string) error {
	f.cleanCalled = true
	f.cleanAt = indexedAt
	f.cleanMeta = metadataJSON
	return f.cleanCallErr
}

type fakeBrainIndexStore struct{}

func (fakeBrainIndexStore) Upsert(context.Context, []codeintel.Chunk) error { return nil }
func (fakeBrainIndexStore) VectorSearch(context.Context, []float32, int, codeintel.Filter) ([]codeintel.SearchResult, error) {
	return nil, nil
}
func (fakeBrainIndexStore) GetByFilePath(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (fakeBrainIndexStore) GetByName(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (fakeBrainIndexStore) DeleteByFilePath(context.Context, string) error { return nil }
func (fakeBrainIndexStore) Close() error                                   { return nil }

type recordingBrainIndexStore struct {
	deleted []string
}

func (s *recordingBrainIndexStore) Upsert(context.Context, []codeintel.Chunk) error { return nil }
func (s *recordingBrainIndexStore) VectorSearch(context.Context, []float32, int, codeintel.Filter) ([]codeintel.SearchResult, error) {
	return nil, nil
}
func (s *recordingBrainIndexStore) GetByFilePath(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (s *recordingBrainIndexStore) GetByName(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (s *recordingBrainIndexStore) DeleteByFilePath(_ context.Context, path string) error {
	s.deleted = append(s.deleted, path)
	return nil
}
func (s *recordingBrainIndexStore) Close() error { return nil }

type fakeBrainIndexEmbedder struct{}

func (fakeBrainIndexEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}
func (fakeBrainIndexEmbedder) EmbedQuery(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func TestRunBrainIndexMarksBrainIndexFresh(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "legacy"
	cfg.Brain.Backend = "vault"
	cfg.Brain.Enabled = true
	cfg.Brain.VaultPath = vaultPath
	if err := os.MkdirAll(cfg.StateDir(), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	brainDoc := "---\ntags: [debug]\n---\n# Runtime Notes\n\nBrain freshness proof."
	if err := brainindexstate.MarkStale(projectRoot, "brain_write", time.Date(2026, 4, 9, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MarkStale: %v", err)
	}

	origBackend := buildBrainIndexBackend
	origStore := openBrainVectorStore
	origEmbedder := newBrainEmbedder
	origMarkFresh := markBrainIndexFresh
	defer func() {
		buildBrainIndexBackend = origBackend
		openBrainVectorStore = origStore
		newBrainEmbedder = origEmbedder
		markBrainIndexFresh = origMarkFresh
	}()

	backend := fakeBrainIndexBackend{docs: map[string]string{"notes.md": brainDoc}}
	buildBrainIndexBackend = func(context.Context, appconfig.BrainConfig, *slog.Logger) (brain.Backend, func(), error) {
		return backend, func() {}, nil
	}
	openBrainVectorStore = func(context.Context, string) (codeintel.Store, error) { return fakeBrainIndexStore{}, nil }
	newBrainEmbedder = func(appconfig.Embedding) codeintel.Embedder { return fakeBrainIndexEmbedder{} }
	markBrainIndexFresh = brainindexstate.MarkFresh

	result, err := runBrainIndex(context.Background(), cfg)
	if err != nil {
		t.Fatalf("runBrainIndex: %v", err)
	}
	if result.DocumentsIndexed != 1 {
		t.Fatalf("DocumentsIndexed = %d, want 1", result.DocumentsIndexed)
	}
	state, err := brainindexstate.Load(projectRoot)
	if err != nil {
		t.Fatalf("Load brain index state: %v", err)
	}
	if state.Status != brainindexstate.StatusClean {
		t.Fatalf("brain index status = %q, want clean", state.Status)
	}
	if state.LastIndexedAt == "" {
		t.Fatal("expected last_indexed_at to be populated after reindex")
	}
	if state.StaleSince != "" || state.StaleReason != "" {
		t.Fatalf("stale fields should be cleared after reindex: %+v", state)
	}
}

func TestRunBrainIndexMarksShunterBrainIndexCleanWithoutFileState(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Memory.Backend = "shunter"
	cfg.Brain.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Brain.DurableAck = true
	brainDoc := "---\ntags: [debug]\n---\n# Runtime Notes\n\nShunter freshness proof."

	origBackend := buildBrainIndexBackend
	origStore := openBrainVectorStore
	origEmbedder := newBrainEmbedder
	origMarkFresh := markBrainIndexFresh
	defer func() {
		buildBrainIndexBackend = origBackend
		openBrainVectorStore = origStore
		newBrainEmbedder = origEmbedder
		markBrainIndexFresh = origMarkFresh
	}()

	backend := &fakeShunterBrainIndexBackend{fakeBrainIndexBackend: fakeBrainIndexBackend{docs: map[string]string{"notes.md": brainDoc}}}
	buildBrainIndexBackend = func(context.Context, appconfig.BrainConfig, *slog.Logger) (brain.Backend, func(), error) {
		return backend, func() {}, nil
	}
	openBrainVectorStore = func(context.Context, string) (codeintel.Store, error) { return fakeBrainIndexStore{}, nil }
	newBrainEmbedder = func(appconfig.Embedding) codeintel.Embedder { return fakeBrainIndexEmbedder{} }
	markBrainIndexFresh = func(string, time.Time) error {
		t.Fatal("file-backed MarkFresh should not be called for Shunter brain index")
		return nil
	}

	result, err := runBrainIndex(context.Background(), cfg)
	if err != nil {
		t.Fatalf("runBrainIndex: %v", err)
	}
	if result.DocumentsIndexed != 1 {
		t.Fatalf("DocumentsIndexed = %d, want 1", result.DocumentsIndexed)
	}
	var metadata struct {
		Source        string   `json:"source"`
		DocumentPaths []string `json:"document_paths"`
	}
	if err := json.Unmarshal([]byte(backend.cleanMeta), &metadata); err != nil {
		t.Fatalf("unmarshal clean metadata: %v", err)
	}
	if !backend.cleanCalled || backend.cleanAt.IsZero() || metadata.Source != "brain_index" || !reflect.DeepEqual(metadata.DocumentPaths, []string{"notes.md"}) {
		t.Fatalf("clean call = called:%t at:%s meta:%q, want Shunter clean mark", backend.cleanCalled, backend.cleanAt, backend.cleanMeta)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state file stat err = %v, want not-exist", err)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("yard database stat err = %v, want not-exist", err)
	}
}

func TestRunBrainIndexShunterUsesStateMetadataForStaleSemanticDeletes(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Memory.Backend = "shunter"
	cfg.Brain.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Brain.DurableAck = true

	origBackend := buildBrainIndexBackend
	origStore := openBrainVectorStore
	origEmbedder := newBrainEmbedder
	origMarkFresh := markBrainIndexFresh
	defer func() {
		buildBrainIndexBackend = origBackend
		openBrainVectorStore = origStore
		newBrainEmbedder = origEmbedder
		markBrainIndexFresh = origMarkFresh
	}()

	backend := &fakeShunterBrainIndexBackend{
		fakeBrainIndexBackend: fakeBrainIndexBackend{docs: map[string]string{
			"notes/current.md": "# Current\n\nCurrent semantic content.",
		}},
		stateFound: true,
		state: projectmemory.BrainIndexState{
			MetadataJSON: `{"source":"brain_index","document_paths":["notes/current.md","notes/stale.md"]}`,
		},
	}
	store := &recordingBrainIndexStore{}
	buildBrainIndexBackend = func(context.Context, appconfig.BrainConfig, *slog.Logger) (brain.Backend, func(), error) {
		return backend, func() {}, nil
	}
	openBrainVectorStore = func(context.Context, string) (codeintel.Store, error) { return store, nil }
	newBrainEmbedder = func(appconfig.Embedding) codeintel.Embedder { return fakeBrainIndexEmbedder{} }
	markBrainIndexFresh = func(string, time.Time) error {
		t.Fatal("file-backed MarkFresh should not be called for Shunter brain index")
		return nil
	}

	result, err := runBrainIndex(context.Background(), cfg)
	if err != nil {
		t.Fatalf("runBrainIndex: %v", err)
	}
	if result.DocumentsIndexed != 1 || result.DocumentsDeleted != 1 || result.SemanticDocumentsDeleted != 1 {
		t.Fatalf("result = %+v, want one current doc and one stale delete", result)
	}
	if !reflect.DeepEqual(store.deleted, []string{"notes/current.md", "notes/stale.md"}) {
		t.Fatalf("semantic deletes = %#v, want current cleanup then stale delete", store.deleted)
	}
	var metadata struct {
		DocumentPaths []string `json:"document_paths"`
	}
	if err := json.Unmarshal([]byte(backend.cleanMeta), &metadata); err != nil {
		t.Fatalf("unmarshal clean metadata: %v", err)
	}
	if !reflect.DeepEqual(metadata.DocumentPaths, []string{"notes/current.md"}) {
		t.Fatalf("clean metadata paths = %#v, want only current doc", metadata.DocumentPaths)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("yard database stat err = %v, want not-exist", err)
	}
}
