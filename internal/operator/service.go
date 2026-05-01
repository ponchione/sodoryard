package operator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

const activeChainScanLimit = 10000

var ErrProcessNotRunning = errors.New("operator process not running")

type ChainStarter func(context.Context, *appconfig.Config, chainrun.Options, chainrun.Deps) (*chainrun.Result, error)

type Options struct {
	ConfigPath      string
	BuildRuntime    func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)
	ChainStarter    ChainStarter
	ProcessSignaler func(pid int) error
	ProcessID       func() int
	ReadOnly        bool
}

type Service struct {
	cfg             *appconfig.Config
	rt              *rtpkg.OrchestratorRuntime
	buildRuntime    func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)
	chainStarter    ChainStarter
	processSignaler func(pid int) error
	processID       func() int
}

func Open(ctx context.Context, opts Options) (*Service, error) {
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		configPath = appconfig.DefaultConfigFilename()
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return nil, err
	}
	buildRuntime := opts.BuildRuntime
	if buildRuntime == nil {
		buildRuntime = rtpkg.BuildOrchestratorRuntime
	}
	rt, err := openRuntime(ctx, cfg, opts, buildRuntime)
	if rt == nil {
		return nil, errors.New("operator: build runtime returned nil")
	}
	if rt.Config != nil {
		cfg = rt.Config
	}
	signaler := opts.ProcessSignaler
	if signaler == nil {
		signaler = interruptProcess
	}
	starter := opts.ChainStarter
	if starter == nil {
		starter = chainrun.Start
	}
	processID := opts.ProcessID
	if processID == nil {
		processID = os.Getpid
	}
	return &Service{cfg: cfg, rt: rt, buildRuntime: buildRuntime, chainStarter: starter, processSignaler: signaler, processID: processID}, nil
}

func openRuntime(ctx context.Context, cfg *appconfig.Config, opts Options, buildRuntime func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)) (*rtpkg.OrchestratorRuntime, error) {
	if opts.ReadOnly {
		return buildReadOnlyRuntime(ctx, cfg)
	}
	return buildRuntime(ctx, cfg)
}

func buildReadOnlyRuntime(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
	database, err := appdb.OpenDB(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = database.Close()
	}
	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		cleanup()
		return nil, fmt.Errorf("init database schema: %w", err)
	}
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		cleanup()
		return nil, fmt.Errorf("ensure chain schema: %w", err)
	}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		cleanup()
		return nil, fmt.Errorf("ensure project record: %w", err)
	}
	brainBackend, closeBrain, err := rtpkg.BuildBrainBackend(ctx, cfg.Brain, slog.New(slog.DiscardHandler))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build brain backend: %w", err)
	}
	cleanup = rtpkg.ChainCleanup(cleanup, closeBrain)
	return &rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     database,
		Queries:      appdb.New(database),
		BrainBackend: brainBackend,
		ChainStore:   chain.NewStore(database),
		Cleanup:      cleanup,
	}, nil
}

func (s *Service) Close() {
	if s == nil || s.rt == nil {
		return
	}
	cleanup := s.rt.Cleanup
	s.rt = nil
	if cleanup != nil {
		cleanup()
	}
}

func (s *Service) RuntimeStatus(ctx context.Context) (RuntimeStatus, error) {
	cfg, err := s.config()
	if err != nil {
		return RuntimeStatus{}, err
	}
	chains, err := s.listChains(ctx, activeChainScanLimit)
	if err != nil {
		return RuntimeStatus{}, err
	}
	activeChains := 0
	for _, ch := range chains {
		if isActiveChainStatus(ch.Status) {
			activeChains++
		}
	}
	return RuntimeStatus{
		ProjectRoot:  cfg.ProjectRoot,
		ProjectName:  cfg.ProjectName(),
		Provider:     cfg.Routing.Default.Provider,
		Model:        cfg.Routing.Default.Model,
		ActiveChains: activeChains,
		Warnings:     nil,
	}, nil
}

func (s *Service) config() (*appconfig.Config, error) {
	if s == nil || s.rt == nil {
		return nil, errors.New("operator service is closed")
	}
	if s.rt.Config != nil {
		return s.rt.Config, nil
	}
	if s.cfg != nil {
		return s.cfg, nil
	}
	return nil, errors.New("operator runtime config is nil")
}

func interruptProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return ErrProcessNotRunning
		}
		return err
	}
	return nil
}

func warningf(format string, args ...any) RuntimeWarning {
	return RuntimeWarning{Message: fmt.Sprintf(format, args...)}
}
