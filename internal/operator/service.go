package operator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

const activeChainScanLimit = 10000
const activeStartShutdownTimeout = 5 * time.Second

var ErrProcessNotRunning = errors.New("operator process not running")

type ChainStarter func(context.Context, *appconfig.Config, chainrun.Options, chainrun.Deps) (*chainrun.Result, error)

type Options struct {
	ConfigPath      string
	BuildRuntime    func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)
	ChainStarter    ChainStarter
	ProcessSignaler func(pid int) error
	ProcessID       func() int
	ReadOnly        bool
	StartupWarnings []RuntimeWarning
}

type Service struct {
	cfg                  *appconfig.Config
	rt                   *rtpkg.OrchestratorRuntime
	buildRuntime         func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)
	chainStarter         ChainStarter
	processSignaler      func(pid int) error
	processID            func() int
	startupWarnings      []RuntimeWarning
	authStatusFromConfig bool
	activeMu             sync.Mutex
	activeStarts         map[string]*activeStart
}

type activeStart struct {
	cancel context.CancelFunc
	done   <-chan startChainDone
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
	if err != nil {
		return nil, err
	}
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
	return &Service{
		cfg:                  cfg,
		rt:                   rt,
		buildRuntime:         buildRuntime,
		chainStarter:         starter,
		processSignaler:      signaler,
		processID:            processID,
		startupWarnings:      cloneRuntimeWarnings(opts.StartupWarnings),
		authStatusFromConfig: opts.ReadOnly && len(opts.StartupWarnings) > 0,
		activeStarts:         make(map[string]*activeStart),
	}, nil
}

func NewForRuntime(rt *rtpkg.OrchestratorRuntime, opts Options) (*Service, error) {
	if rt == nil {
		return nil, errors.New("operator: runtime is nil")
	}
	cfg := optsConfig(opts, rt)
	if cfg == nil {
		return nil, errors.New("operator: runtime config is nil")
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
	buildRuntime := opts.BuildRuntime
	if buildRuntime == nil {
		buildRuntime = rtpkg.BuildOrchestratorRuntime
	}
	return &Service{
		cfg:                  cfg,
		rt:                   rt,
		buildRuntime:         buildRuntime,
		chainStarter:         starter,
		processSignaler:      signaler,
		processID:            processID,
		startupWarnings:      cloneRuntimeWarnings(opts.StartupWarnings),
		authStatusFromConfig: opts.ReadOnly && len(opts.StartupWarnings) > 0,
		activeStarts:         make(map[string]*activeStart),
	}, nil
}

func optsConfig(opts Options, rt *rtpkg.OrchestratorRuntime) *appconfig.Config {
	if rt != nil && rt.Config != nil {
		return rt.Config
	}
	if opts.ConfigPath == "" {
		return nil
	}
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return nil
	}
	return cfg
}

func openRuntime(ctx context.Context, cfg *appconfig.Config, opts Options, buildRuntime func(context.Context, *appconfig.Config) (*rtpkg.OrchestratorRuntime, error)) (*rtpkg.OrchestratorRuntime, error) {
	if opts.ReadOnly {
		return buildReadOnlyRuntime(ctx, cfg)
	}
	return buildRuntime(ctx, cfg)
}

func buildReadOnlyRuntime(ctx context.Context, cfg *appconfig.Config) (*rtpkg.OrchestratorRuntime, error) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		if _, err := os.Stat(cfg.Memory.ShunterDataDir); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat project memory %q: %w", cfg.Memory.ShunterDataDir, err)
			}
		} else {
			logger := slog.New(slog.DiscardHandler)
			brainBackend, closeBrain, err := rtpkg.BuildBrainBackend(ctx, cfg.Brain, logger)
			if err != nil {
				return nil, fmt.Errorf("build brain backend: %w", err)
			}
			memoryBackend, closeMemory, err := rtpkg.BuildProjectMemoryStore(ctx, cfg, brainBackend, logger)
			if err != nil {
				closeBrain()
				return nil, fmt.Errorf("build project memory store: %w", err)
			}
			chainStore, err := rtpkg.BuildChainStore(cfg, nil, memoryBackend)
			if err != nil {
				closeMemory()
				closeBrain()
				return nil, fmt.Errorf("build chain store: %w", err)
			}
			return &rtpkg.OrchestratorRuntime{
				Config:        cfg,
				BrainBackend:  brainBackend,
				MemoryBackend: memoryBackend,
				ChainStore:    chainStore,
				Cleanup: rtpkg.ChainCleanup(closeBrain, func() {
					closeMemory()
				}),
			}, nil
		}
	}

	dbPath := cfg.DatabasePath()
	removeDBOnCleanup := false
	if _, err := os.Stat(dbPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat database %q: %w", dbPath, err)
		}
		tmp, err := os.CreateTemp("", "yard-readonly-*.db")
		if err != nil {
			return nil, fmt.Errorf("create temporary read-only database: %w", err)
		}
		dbPath = tmp.Name()
		if err := tmp.Close(); err != nil {
			_ = os.Remove(dbPath)
			return nil, fmt.Errorf("close temporary read-only database: %w", err)
		}
		removeDBOnCleanup = true
	}
	database, err := appdb.OpenDB(ctx, dbPath)
	if err != nil {
		if removeDBOnCleanup {
			_ = os.Remove(dbPath)
		}
		return nil, err
	}
	cleanup := func() {
		_ = database.Close()
		if removeDBOnCleanup {
			_ = os.Remove(dbPath)
			_ = os.Remove(dbPath + "-wal")
			_ = os.Remove(dbPath + "-shm")
		}
	}
	if removeDBOnCleanup {
		if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
			cleanup()
			return nil, fmt.Errorf("init database schema: %w", err)
		}
		if err := appdb.EnsureChainSchema(ctx, database); err != nil {
			cleanup()
			return nil, fmt.Errorf("ensure chain schema: %w", err)
		}
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
	s.stopActiveStarts(activeStartShutdownTimeout)
	cleanup := s.rt.Cleanup
	s.rt = nil
	if cleanup != nil {
		cleanup()
	}
}

func (s *Service) registerActiveStart(chainID string, cancel context.CancelFunc, done <-chan startChainDone) {
	if s == nil || strings.TrimSpace(chainID) == "" || cancel == nil || done == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if s.activeStarts == nil {
		s.activeStarts = make(map[string]*activeStart)
	}
	s.activeStarts[chainID] = &activeStart{cancel: cancel, done: done}
}

func (s *Service) unregisterActiveStart(chainID string) {
	if s == nil || strings.TrimSpace(chainID) == "" {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.activeStarts, chainID)
}

func (s *Service) cancelActiveStart(chainID string) bool {
	if s == nil || strings.TrimSpace(chainID) == "" {
		return false
	}
	s.activeMu.Lock()
	active := s.activeStarts[chainID]
	s.activeMu.Unlock()
	if active == nil || active.cancel == nil {
		return false
	}
	active.cancel()
	return true
}

func (s *Service) stopActiveStarts(timeout time.Duration) {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	active := make(map[string]*activeStart, len(s.activeStarts))
	for chainID, start := range s.activeStarts {
		active[chainID] = start
	}
	s.activeMu.Unlock()
	if len(active) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for chainID := range active {
		_, _ = s.CancelChain(ctx, chainID)
	}
	for chainID, start := range active {
		if start == nil || start.done == nil {
			continue
		}
		select {
		case <-start.done:
			s.unregisterActiveStart(chainID)
		case <-ctx.Done():
			return
		}
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
	warnings := cloneRuntimeWarnings(s.startupWarnings)
	codeIndex, codeWarning := s.codeIndexStatus(ctx, cfg)
	if codeWarning != nil {
		warnings = append(warnings, *codeWarning)
	}
	brainIndex, brainWarning := brainIndexStatus(ctx, cfg, s.rt.BrainBackend)
	if brainWarning != nil {
		warnings = append(warnings, *brainWarning)
	}
	warnings = append(warnings, indexReadinessWarnings(codeIndex, brainIndex)...)
	authStatus, authWarning := s.authStatus(ctx, cfg)
	if authWarning != nil {
		warnings = append(warnings, *authWarning)
	}
	return RuntimeStatus{
		ProjectRoot:         cfg.ProjectRoot,
		ProjectName:         cfg.ProjectName(),
		Provider:            cfg.Routing.Default.Provider,
		Model:               cfg.Routing.Default.Model,
		AuthStatus:          authStatus,
		CodeIndex:           codeIndex,
		BrainIndex:          brainIndex,
		LocalServicesStatus: localServicesStatus(cfg),
		ActiveChains:        activeChains,
		Warnings:            warnings,
	}, nil
}

func (s *Service) authStatus(ctx context.Context, cfg *appconfig.Config) (string, *RuntimeWarning) {
	if cfg == nil || strings.TrimSpace(cfg.Routing.Default.Provider) == "" {
		return "unconfigured", ptrWarning(warningf("routing.default.provider is not configured"))
	}
	if s == nil || s.rt == nil || s.rt.ProviderRouter == nil {
		if s != nil && s.authStatusFromConfig {
			return configProviderAuthStatus(ctx, cfg)
		}
		return "unavailable", ptrWarning(warningf("provider auth status is unavailable in this runtime"))
	}
	providerName := cfg.Routing.Default.Provider
	statuses, err := s.rt.ProviderRouter.AuthStatuses(ctx)
	if err != nil {
		return "unknown", ptrWarning(warningf("load auth status for %s: %v", providerName, err))
	}
	status, ok := statuses[providerName]
	if !ok {
		return "not registered", ptrWarning(warningf("default provider %s is not registered", providerName))
	}
	if status == nil {
		return "not reported", nil
	}
	display := formatAuthStatus(status)
	if authStatusReady(status) {
		return display, nil
	}
	message := fmt.Sprintf("default provider %s auth is %s", providerName, authStatusDisplayState(status))
	if strings.TrimSpace(status.Remediation) != "" {
		message = message + ": " + strings.TrimSpace(status.Remediation)
	} else if strings.TrimSpace(status.Detail) != "" {
		message = message + ": " + strings.TrimSpace(status.Detail)
	}
	return display, ptrWarning(warningf("%s", message))
}

func configProviderAuthStatus(ctx context.Context, cfg *appconfig.Config) (string, *RuntimeWarning) {
	providerName := strings.TrimSpace(cfg.Routing.Default.Provider)
	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return "not registered", ptrWarning(warningf("default provider %s is not configured", providerName))
	}
	p, err := rtpkg.BuildProvider(providerName, provCfg)
	if err != nil {
		return "unknown", ptrWarning(warningf("build default provider %s for auth status: %v", providerName, err))
	}
	reporter, ok := p.(provider.AuthStatusReporter)
	if !ok {
		return "not reported", ptrWarning(warningf("default provider %s does not report auth status", providerName))
	}
	status, err := reporter.AuthStatus(ctx)
	if err != nil {
		return "unknown", ptrWarning(warningf("load auth status for %s: %v", providerName, err))
	}
	display := formatAuthStatus(status)
	if authStatusReady(status) {
		return display, nil
	}
	message := fmt.Sprintf("default provider %s auth is %s", providerName, authStatusDisplayState(status))
	if status != nil && strings.TrimSpace(status.Remediation) != "" {
		message = message + ": " + strings.TrimSpace(status.Remediation)
	} else if status != nil && strings.TrimSpace(status.Detail) != "" {
		message = message + ": " + strings.TrimSpace(status.Detail)
	}
	return display, ptrWarning(warningf("%s", message))
}

func formatAuthStatus(status *provider.AuthStatus) string {
	if status == nil {
		return "not reported"
	}
	state := authStatusDisplayState(status)
	var parts []string
	if strings.TrimSpace(status.Mode) != "" {
		parts = append(parts, status.Mode)
	}
	if strings.TrimSpace(status.Source) != "" {
		parts = append(parts, status.Source)
	}
	if strings.TrimSpace(status.Detail) != "" {
		parts = append(parts, status.Detail)
	}
	if len(parts) == 0 {
		return state
	}
	return fmt.Sprintf("%s (%s)", state, strings.Join(parts, ", "))
}

func authStatusReady(status *provider.AuthStatus) bool {
	return provider.AuthStatusReady(status, time.Now())
}

func authStatusDisplayState(status *provider.AuthStatus) string {
	switch provider.AuthStatusState(status, time.Now()) {
	case "ready":
		return "ready"
	case "expired_access_token":
		return "expired"
	case "access_token_expires_soon":
		return "expires soon"
	case "missing_credentials":
		return "missing credentials"
	case "missing_access_token":
		return "missing access token"
	case "unavailable":
		return "unavailable"
	default:
		return "not ready"
	}
}

func (s *Service) codeIndexStatus(ctx context.Context, cfg *appconfig.Config) (RuntimeIndexStatus, *RuntimeWarning) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		reader, ok := s.rt.BrainBackend.(interface {
			ReadCodeIndexState(context.Context) (projectmemory.CodeIndexState, bool, error)
		})
		if !ok || reader == nil {
			return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load code index state: Shunter backend unavailable"))
		}
		state, found, err := reader.ReadCodeIndexState(ctx)
		if err != nil {
			return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load code index state: %v", err))
		}
		return runtimeStatusFromShunterCodeIndexState(state, found), nil
	}
	if s == nil || s.rt == nil || s.rt.Database == nil {
		return RuntimeIndexStatus{Status: "unavailable"}, nil
	}
	var commit sql.NullString
	var indexedAt sql.NullString
	err := s.rt.Database.QueryRowContext(ctx, `SELECT last_indexed_commit, last_indexed_at FROM projects WHERE id = ?`, cfg.ProjectRoot).Scan(&commit, &indexedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeIndexStatus{Status: "never_indexed"}, nil
	}
	if err != nil {
		return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load code index state: %v", err))
	}
	status := RuntimeIndexStatus{Status: "never_indexed"}
	if indexedAt.Valid && strings.TrimSpace(indexedAt.String) != "" {
		status.Status = "indexed"
		status.LastIndexedAt = indexedAt.String
	}
	if commit.Valid {
		status.LastIndexedCommit = commit.String
	}
	return status, nil
}

func runtimeStatusFromShunterCodeIndexState(state projectmemory.CodeIndexState, found bool) RuntimeIndexStatus {
	status := RuntimeIndexStatus{Status: "never_indexed"}
	if !found {
		return status
	}
	if state.LastIndexedAtUS > 0 {
		status.Status = "indexed"
		status.LastIndexedAt = time.UnixMicro(int64(state.LastIndexedAtUS)).UTC().Format(time.RFC3339)
	}
	status.LastIndexedCommit = state.LastIndexedCommit
	if state.Dirty {
		status.Status = brainindexstate.StatusStale
		status.StaleReason = state.DirtyReason
	}
	return status
}

func brainIndexStatus(ctx context.Context, cfg *appconfig.Config, backend any) (RuntimeIndexStatus, *RuntimeWarning) {
	if cfg == nil || !cfg.Brain.Enabled {
		return RuntimeIndexStatus{Status: "disabled"}, nil
	}
	if cfg.Brain.Backend == "shunter" {
		reader, ok := backend.(interface {
			ReadBrainIndexState(context.Context) (projectmemory.BrainIndexState, bool, error)
		})
		if !ok || reader == nil {
			return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load brain index state: Shunter backend unavailable"))
		}
		state, found, err := reader.ReadBrainIndexState(ctx)
		if err != nil {
			return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load brain index state: %v", err))
		}
		return runtimeStatusFromShunterBrainIndexState(state, found), nil
	}
	state, err := brainindexstate.Load(cfg.ProjectRoot)
	if err != nil {
		return RuntimeIndexStatus{Status: "unknown"}, ptrWarning(warningf("load brain index state: %v", err))
	}
	return RuntimeIndexStatus{
		Status:        state.Status,
		LastIndexedAt: state.LastIndexedAt,
		StaleSince:    state.StaleSince,
		StaleReason:   state.StaleReason,
	}, nil
}

func runtimeStatusFromShunterBrainIndexState(state projectmemory.BrainIndexState, found bool) RuntimeIndexStatus {
	status := RuntimeIndexStatus{Status: brainindexstate.StatusNeverIndexed}
	if !found {
		return status
	}
	if state.LastIndexedAtUS > 0 {
		status.Status = brainindexstate.StatusClean
		status.LastIndexedAt = time.UnixMicro(int64(state.LastIndexedAtUS)).UTC().Format(time.RFC3339)
	}
	if state.Dirty {
		status.Status = brainindexstate.StatusStale
		if state.DirtySinceUS > 0 {
			status.StaleSince = time.UnixMicro(int64(state.DirtySinceUS)).UTC().Format(time.RFC3339)
		}
		status.StaleReason = state.DirtyReason
	}
	return status
}

func indexReadinessWarnings(codeIndex RuntimeIndexStatus, brainIndex RuntimeIndexStatus) []RuntimeWarning {
	var warnings []RuntimeWarning
	switch codeIndex.Status {
	case "never_indexed":
		warnings = append(warnings, warningf("code index has not been built; run `yard index` before retrieval/runtime validation"))
	case "unknown", "unavailable":
		warnings = append(warnings, warningf("code index status is %s; run `yard index` if retrieval quality looks poor", codeIndex.Status))
	}
	switch brainIndex.Status {
	case brainindexstate.StatusNeverIndexed:
		warnings = append(warnings, warningf("brain index has not been built; run `yard brain index` if brain retrieval is enabled"))
	case brainindexstate.StatusStale:
		warnings = append(warnings, warningf("brain index is stale; run `yard brain index`"))
	case "unknown":
		warnings = append(warnings, warningf("brain index status is unknown; run `yard brain index` if brain retrieval is enabled"))
	}
	return warnings
}

func localServicesStatus(cfg *appconfig.Config) string {
	if cfg == nil || !cfg.LocalServices.Enabled {
		return "disabled"
	}
	mode := strings.TrimSpace(cfg.LocalServices.Mode)
	if mode == "" {
		return "enabled"
	}
	return mode
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

func (s *Service) database() (*sql.DB, error) {
	if s == nil || s.rt == nil {
		return nil, errors.New("operator service is closed")
	}
	if s.rt.Database == nil {
		return nil, errors.New("operator runtime database is nil")
	}
	return s.rt.Database, nil
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

func ptrWarning(warning RuntimeWarning) *RuntimeWarning {
	return &warning
}

func cloneRuntimeWarnings(warnings []RuntimeWarning) []RuntimeWarning {
	if len(warnings) == 0 {
		return nil
	}
	cloned := make([]RuntimeWarning, 0, len(warnings))
	for _, warning := range warnings {
		message := strings.TrimSpace(warning.Message)
		if message == "" {
			continue
		}
		cloned = append(cloned, RuntimeWarning{Message: message})
	}
	return cloned
}
