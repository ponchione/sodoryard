package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/server"
)

const smokeChainID = "projectmemory-sdk-smoke-chain"
const smokeStepID = "projectmemory-sdk-smoke-step"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, err := repoRoot()
	if err != nil {
		return err
	}

	dataDir, err := os.MkdirTemp("", "yard-projectmemory-sdk-smoke-*")
	if err != nil {
		return fmt.Errorf("create project memory smoke data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
		DataDir:        dataDir,
		DurableAck:     true,
		EnableProtocol: true,
	})
	if err != nil {
		return fmt.Errorf("open project memory backend: %w", err)
	}
	defer backend.Close()

	if err := seedSmokeProjectMemory(ctx, backend); err != nil {
		return err
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, logger)
	server.NewProjectMemoryHandler(srv, backend, logger)
	serverCtx, stopServer := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(serverCtx) }()
	baseURL := "http://" + srv.ListenAddr()

	cmd := exec.CommandContext(
		ctx,
		"npm",
		"run",
		"test",
		"--",
		"src/lib/project-memory/runtime-smoke.test.ts",
		"--environment",
		"node",
	)
	cmd.Dir = filepath.Join(root, "web")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"PROJECT_MEMORY_SMOKE_BASE_URL="+baseURL,
		"PROJECT_MEMORY_SMOKE_CHAIN_ID="+smokeChainID,
	)
	runErr := cmd.Run()

	stopServer()
	if err := <-errCh; err != nil {
		return fmt.Errorf("stop project memory smoke server: %w", err)
	}
	if runErr != nil {
		return fmt.Errorf("run project memory SDK smoke test: %w", runErr)
	}
	return nil
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(wd, "web", "package.json")); err == nil {
				return wd, nil
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("repo root not found from %s", wd)
		}
		wd = parent
	}
}

func seedSmokeProjectMemory(ctx context.Context, backend *projectmemory.BrainBackend) error {
	now := uint64(time.Now().UnixMicro())
	if err := backend.StartChain(ctx, projectmemory.StartChainArgs{
		ID:              smokeChainID,
		SourceSpecsJSON: "[]",
		SourceTask:      "validate Shunter TypeScript SDK runtime smoke",
		MaxSteps:        1,
		CreatedAtUS:     now,
	}); err != nil {
		return fmt.Errorf("seed smoke chain: %w", err)
	}
	if err := backend.StartStep(ctx, projectmemory.StartStepArgs{
		ID:          smokeStepID,
		ChainID:     smokeChainID,
		Sequence:    1,
		Role:        "coder",
		Task:        "exercise generated Project Memory bindings",
		CreatedAtUS: now + 1_000,
	}); err != nil {
		return fmt.Errorf("seed smoke step: %w", err)
	}
	if err := backend.LogChainEvent(ctx, projectmemory.LogChainEventArgs{
		ChainID:     smokeChainID,
		StepID:      smokeStepID,
		EventType:   "step_started",
		PayloadJSON: `{"role":"coder"}`,
		CreatedAtUS: now + 2_000,
	}); err != nil {
		return fmt.Errorf("seed smoke started event: %w", err)
	}
	if err := backend.LogChainEvent(ctx, projectmemory.LogChainEventArgs{
		ChainID:     smokeChainID,
		StepID:      smokeStepID,
		EventType:   "approval_required",
		PayloadJSON: `{"tool":"shell","reason":"smoke"}`,
		CreatedAtUS: now + 3_000,
	}); err != nil {
		return fmt.Errorf("seed smoke approval event: %w", err)
	}
	return nil
}
