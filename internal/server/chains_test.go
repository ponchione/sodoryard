package server_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/operator"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/server"
)

type chainTestBrain struct {
	docs map[string]string
}

func (b *chainTestBrain) ReadDocument(_ context.Context, path string) (string, error) {
	content, ok := b.docs[path]
	if !ok {
		return "", fmt.Errorf("missing document %s", path)
	}
	return content, nil
}

func (b *chainTestBrain) WriteDocument(_ context.Context, path string, content string) error {
	b.docs[path] = content
	return nil
}

func (b *chainTestBrain) PatchDocument(context.Context, string, string, string) error {
	return nil
}

func (b *chainTestBrain) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (b *chainTestBrain) ListDocuments(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestChainInspectorEndpoints(t *testing.T) {
	ctx := context.Background()
	db := newChainInspectorTestDB(t)
	store := chain.NewStore(db)
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-web", SourceTask: "inspect"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	receiptPath := "receipts/coder/chain-web-step-001.md"
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "accepted", ReceiptPath: receiptPath, TokensUsed: 42}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, chainID, "completed", "done"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}

	cfg := &config.Config{
		ProjectRoot: t.TempDir(),
		Routing: config.RoutingConfig{
			Default: config.RouteConfig{Provider: "codex", Model: "test-model"},
		},
	}
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     db,
		ChainStore:   store,
		BrainBackend: &chainTestBrain{docs: map[string]string{receiptPath: "receipt content"}},
		Cleanup:      func() {},
	}, operator.Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var chains []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	getJSON(t, base+"/api/chains", &chains)
	if len(chains) != 1 || chains[0].ID != chainID || chains[0].Status != "completed" {
		t.Fatalf("chains response = %+v, want completed chain-web", chains)
	}

	var detail struct {
		Chain struct {
			ID string `json:"id"`
		} `json:"chain"`
		Steps []struct {
			Role        string `json:"role"`
			ReceiptPath string `json:"receipt_path"`
		} `json:"steps"`
		Receipts []struct {
			Step string `json:"step"`
			Path string `json:"path"`
		} `json:"receipts"`
	}
	getJSON(t, base+"/api/chains/"+chainID, &detail)
	if detail.Chain.ID != chainID || len(detail.Steps) != 1 || detail.Steps[0].Role != "coder" {
		t.Fatalf("detail response = %+v, want chain detail", detail)
	}
	if len(detail.Receipts) != 1 || detail.Receipts[0].Path != receiptPath {
		t.Fatalf("receipts = %+v, want step receipt", detail.Receipts)
	}

	var receipt struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	getJSON(t, base+"/api/chains/"+chainID+"/receipt?step=1", &receipt)
	if receipt.Path != receiptPath || receipt.Content != "receipt content" {
		t.Fatalf("receipt = %+v, want content", receipt)
	}
}

func TestRuntimeStatusEndpointReadsShunterIndexStateWithoutLegacyStores(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Brain.ShunterDataDir = cfg.Memory.ShunterDataDir
	cfg.Routing.Default.Model = "test-model"

	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: cfg.Memory.ShunterDataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	codeIndexedAt := time.Date(2026, 5, 5, 17, 0, 0, 0, time.UTC)
	brainIndexedAt := time.Date(2026, 5, 5, 17, 5, 0, 0, time.UTC)
	if err := backend.MarkCodeIndexClean(ctx, "status123", codeIndexedAt, []projectmemory.CodeFileIndexArg{{FilePath: "main.go", FileHash: "hash-main", ChunkCount: 1}}, nil, `{"source":"test"}`); err != nil {
		t.Fatalf("MarkCodeIndexClean: %v", err)
	}
	if err := backend.MarkBrainIndexClean(ctx, brainIndexedAt, `{"source":"test"}`); err != nil {
		t.Fatalf("MarkBrainIndexClean: %v", err)
	}

	store := chain.NewStore(newChainInspectorTestDB(t))
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		ChainStore:   store,
		BrainBackend: backend,
		Cleanup:      func() {},
	}, operator.Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var status struct {
		CodeIndex struct {
			Status            string `json:"status"`
			LastIndexedAt     string `json:"last_indexed_at"`
			LastIndexedCommit string `json:"last_indexed_commit"`
		} `json:"code_index"`
		BrainIndex struct {
			Status        string `json:"status"`
			LastIndexedAt string `json:"last_indexed_at"`
		} `json:"brain_index"`
	}
	getJSON(t, base+"/api/runtime/status", &status)

	if status.CodeIndex.Status != "indexed" || status.CodeIndex.LastIndexedCommit != "status123" || status.CodeIndex.LastIndexedAt != codeIndexedAt.Format(time.RFC3339) {
		t.Fatalf("code_index = %+v, want Shunter indexed status123", status.CodeIndex)
	}
	if status.BrainIndex.Status != brainindexstate.StatusClean || status.BrainIndex.LastIndexedAt != brainIndexedAt.Format(time.RFC3339) {
		t.Fatalf("brain_index = %+v, want Shunter clean state", status.BrainIndex)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("database stat err = %v, want no yard.db created in Shunter mode", err)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state stat err = %v, want no file-backed state created in Shunter mode", err)
	}
}

func newChainInspectorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := appdb.OpenDB(context.Background(), filepath.Join(t.TempDir(), "server-chains.db"))
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := appdb.InitIfNeeded(context.Background(), db); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	return db
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}
