package cmdutil

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
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/codeintel"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appindex "github.com/ponchione/sodoryard/internal/index"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestCodeIndexCommandQuietSuppressesSummary(t *testing.T) {
	configPath := writeIndexCommandTestConfig(t)
	cmd := NewCodeIndexCommand("index", "Index", &configPath, func(context.Context, appindex.Options) (*appindex.Result, error) {
		return &appindex.Result{Mode: "incremental", ChunksWritten: 3}, nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("quiet output = %q, want empty", out.String())
	}
}

func TestCodeIndexCommandJSONOverridesQuiet(t *testing.T) {
	configPath := writeIndexCommandTestConfig(t)
	cmd := NewCodeIndexCommand("index", "Index", &configPath, func(context.Context, appindex.Options) (*appindex.Result, error) {
		return &appindex.Result{Mode: "incremental", ChunksWritten: 3}, nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--quiet", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"mode": "incremental"`) {
		t.Fatalf("json output = %q, want mode", out.String())
	}
}

func TestRunBrainIndexShunterRebuildsFromProjectMemoryWithoutYardDB(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Brain.MemoryBackend = cfg.Memory.Backend
	cfg.Brain.ShunterDataDir = cfg.Memory.ShunterDataDir
	cfg.Brain.DurableAck = cfg.Memory.DurableAck
	brainDir := filepath.Join(projectRoot, ".brain-missing")

	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: cfg.Memory.ShunterDataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if err := backend.WriteDocument(ctx, "notes/current.md", "# Current\n\nLink to [[Next]] from Shunter project memory."); err != nil {
		t.Fatalf("WriteDocument current: %v", err)
	}
	if err := backend.MarkBrainIndexClean(ctx, time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC), `{"source":"brain_index","document_paths":["notes/current.md","notes/stale.md"]}`); err != nil {
		t.Fatalf("MarkBrainIndexClean previous state: %v", err)
	}

	store := &recordingBrainIndexStore{}
	result, err := RunBrainIndex(ctx, cfg, BrainIndexDeps{
		BuildBackend: func(context.Context, appconfig.BrainConfig, *slog.Logger) (brain.Backend, func(), error) {
			return backend, func() {}, nil
		},
		OpenStore: func(context.Context, string) (codeintel.Store, error) {
			return store, nil
		},
		NewEmbedder: func(appconfig.Embedding) codeintel.Embedder {
			return fakeBrainIndexEmbedder{}
		},
		MarkFresh: func(string, time.Time) error {
			t.Fatal("file-backed MarkFresh should not be called for Shunter brain index")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RunBrainIndex: %v", err)
	}
	if result.DocumentsIndexed != 1 || result.LinksIndexed != 1 || result.DocumentsDeleted != 1 || result.SemanticChunksIndexed == 0 || result.SemanticDocumentsDeleted != 1 {
		t.Fatalf("result = %+v, want one current doc/link, one stale delete, and semantic chunks", result)
	}
	if !reflect.DeepEqual(store.deleted, []string{"notes/current.md", "notes/stale.md"}) {
		t.Fatalf("semantic deletes = %#v, want current cleanup then stale delete", store.deleted)
	}
	if len(store.upserted) == 0 || store.upserted[0].FilePath != "notes/current.md" {
		t.Fatalf("semantic upserts = %#v, want chunks for notes/current.md", store.upserted)
	}
	state, found, err := backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState: %v", err)
	}
	if !found || state.Dirty || state.LastIndexedAtUS == 0 {
		t.Fatalf("brain index state = %+v found=%t, want clean Shunter state", state, found)
	}
	var metadata brainIndexStateMetadata
	if err := json.Unmarshal([]byte(state.MetadataJSON), &metadata); err != nil {
		t.Fatalf("unmarshal state metadata: %v", err)
	}
	if metadata.Source != "brain_index" || !reflect.DeepEqual(metadata.DocumentPaths, []string{"notes/current.md"}) {
		t.Fatalf("state metadata = %+v, want current Shunter document path", metadata)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("yard database stat err = %v, want no yard.db created in Shunter mode", err)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state file stat err = %v, want no file-backed state in Shunter mode", err)
	}
	if _, err := os.Stat(brainDir); !os.IsNotExist(err) {
		t.Fatalf("brain vault stat err = %v, want no .brain dependency in Shunter mode", err)
	}
}

func writeIndexCommandTestConfig(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := "project_root: " + projectRoot + "\n" +
		"brain:\n  enabled: true\n" +
		"local_services:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return configPath
}

type recordingBrainIndexStore struct {
	deleted  []string
	upserted []codeintel.Chunk
}

func (s *recordingBrainIndexStore) Upsert(_ context.Context, chunks []codeintel.Chunk) error {
	s.upserted = append(s.upserted, chunks...)
	return nil
}

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
