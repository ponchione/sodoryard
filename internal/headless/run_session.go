package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/role"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

type ExitCode int

const (
	ExitOK             ExitCode = 0
	ExitInfrastructure ExitCode = 1
	ExitSafetyLimit    ExitCode = 2
	ExitEscalation     ExitCode = 3
)

type RunRequest struct {
	Role        string
	Task        string
	TaskFile    string
	ChainID     string
	Brain       string
	MaxTurns    int
	MaxTokens   int
	Timeout     time.Duration
	ReceiptPath string
	Quiet       bool
	ProjectRoot string
}

type RunResult struct {
	ReceiptPath string
	ExitCode    ExitCode
}

const defaultRunTimeout = 30 * time.Minute

type AgentLoop interface {
	RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error)
	Close()
}

type RuntimeFactory func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.EngineRuntime, error)
type RegistryFactory func(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, deps role.BuilderDeps) (*tool.Registry, appconfig.BrainConfig, error)
type ConversationFactory func(ctx context.Context, manager *conversation.Manager, projectRoot string, opts ...conversation.CreateOption) (*conversation.Conversation, error)
type AgentLoopFactory func(deps agent.AgentLoopDeps) AgentLoop
type ProgressSinkFactory func(out io.Writer) agent.EventSink
type ChainIDFactory func() string

type Deps struct {
	BuildRuntime       RuntimeFactory
	BuildRegistry      RegistryFactory
	CreateConversation ConversationFactory
	NewAgentLoop       AgentLoopFactory
	NewProgressSink    ProgressSinkFactory
	NewChainID         ChainIDFactory
}

func withDefaultDeps(deps Deps) Deps {
	if deps.BuildRuntime == nil {
		deps.BuildRuntime = rtpkg.BuildEngineRuntime
	}
	if deps.BuildRegistry == nil {
		deps.BuildRegistry = role.BuildRegistry
	}
	if deps.CreateConversation == nil {
		deps.CreateConversation = func(ctx context.Context, manager *conversation.Manager, projectRoot string, opts ...conversation.CreateOption) (*conversation.Conversation, error) {
			return manager.Create(ctx, projectRoot, opts...)
		}
	}
	if deps.NewAgentLoop == nil {
		deps.NewAgentLoop = func(deps agent.AgentLoopDeps) AgentLoop {
			return agent.NewAgentLoop(deps)
		}
	}
	if deps.NewProgressSink == nil {
		deps.NewProgressSink = func(out io.Writer) agent.EventSink {
			return NewProgressSink(out)
		}
	}
	if deps.NewChainID == nil {
		deps.NewChainID = id.New
	}
	return deps
}

func RunSession(parentCtx context.Context, progressOut io.Writer, configPath string, req RunRequest, deps Deps) (*RunResult, error) {
	deps = withDefaultDeps(deps)
	if err := validateRunRequest(req); err != nil {
		return nil, err
	}

	taskText, cfg, roleName, roleCfg, systemPrompt, chainID, err := prepareRunRequest(configPath, req, deps.NewChainID)
	if err != nil {
		return nil, err
	}
	req.Role = roleName

	timeout := resolveRunTimeout(roleCfg, req.Timeout)
	ctx, cancel := context.WithTimeout(parentContext(parentCtx), timeout)
	defer cancel()

	rt, err := deps.BuildRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanupRuntime(rt)

	registry, scopedBrainCfg, err := deps.BuildRegistry(cfg, roleCfg, role.BuilderDeps{
		BrainBackend:     rt.BrainBackend,
		BrainSearcher:    rt.BrainSearcher,
		SemanticSearcher: rt.SemanticSearcher,
		ProviderRuntime:  rt.ProviderRouter,
		Queries:          rt.Queries,
		ProjectID:        cfg.ProjectRoot,
	})
	if err != nil {
		return nil, err
	}

	loopMaxTurns, maxTokens := resolveRunLimits(cfg, roleCfg, req)
	turnResult, turnErr, err := executeRunTurn(ctx, progressOut, cfg, req, taskText, systemPrompt, rt, registry, loopMaxTurns, deps)
	if err != nil {
		return nil, err
	}

	receiptVerdict, exitCode, err := determineExitStatus(ctx, turnResult, turnErr, loopMaxTurns, maxTokens)
	if err != nil {
		return nil, err
	}

	receiptPath, receiptMeta, err := EnsureReceipt(
		ctx,
		rt.BrainBackend,
		scopedBrainCfg,
		req.Role,
		chainID,
		ResolveReceiptPath(req.Role, chainID, req.ReceiptPath),
		receiptVerdict,
		FinalText(turnResult),
		turnResult,
	)
	if err != nil {
		return nil, err
	}
	if receiptMeta != nil {
		switch receiptMeta.Verdict {
		case "escalate":
			exitCode = ExitEscalation
		case "safety_limit":
			exitCode = ExitSafetyLimit
		}
	}

	return &RunResult{ReceiptPath: receiptPath, ExitCode: exitCode}, nil
}

func validateRunRequest(req RunRequest) error {
	if strings.TrimSpace(req.Role) == "" {
		return fmt.Errorf("--role is required")
	}
	if (strings.TrimSpace(req.Task) == "") == (strings.TrimSpace(req.TaskFile) == "") {
		return fmt.Errorf("exactly one of --task or --task-file is required")
	}
	if req.MaxTurns < 0 {
		return fmt.Errorf("--max-turns must be > 0 when supplied")
	}
	if req.MaxTokens < 0 {
		return fmt.Errorf("--max-tokens must be > 0 when supplied")
	}
	if req.Timeout < 0 {
		return fmt.Errorf("--timeout must be > 0 when supplied")
	}
	return nil
}

func prepareRunRequest(configPath string, req RunRequest, newChainID func() string) (string, *appconfig.Config, string, appconfig.AgentRoleConfig, string, string, error) {
	taskText, err := ReadTask(req.Task, req.TaskFile)
	if err != nil {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", err
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", fmt.Errorf("load config: %w", err)
	}
	if strings.TrimSpace(req.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(req.ProjectRoot)
	}
	if strings.TrimSpace(req.Brain) != "" {
		cfg.Brain.VaultPath = strings.TrimSpace(req.Brain)
	}
	if err := cfg.Validate(); err != nil {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", err
	}
	roleName, roleCfg, err := cfg.ResolveAgentRole(req.Role)
	if err != nil {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", err
	}
	systemPrompt, _, err := rtpkg.LoadRoleSystemPrompt(roleName, cfg.ProjectRoot, roleCfg.SystemPrompt)
	if err != nil {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", err
	}
	chainID := strings.TrimSpace(req.ChainID)
	if chainID == "" {
		chainID = strings.TrimSpace(newChainID())
	}
	if chainID == "" {
		return "", nil, "", appconfig.AgentRoleConfig{}, "", "", fmt.Errorf("chain ID generator returned empty ID")
	}
	return taskText, cfg, roleName, roleCfg, systemPrompt, chainID, nil
}

func executeRunTurn(ctx context.Context, progressOut io.Writer, cfg *appconfig.Config, req RunRequest, taskText string, systemPrompt string, rt *rtpkg.EngineRuntime, registry *tool.Registry, loopMaxTurns int, deps Deps) (*agent.TurnResult, error, error) {
	executor := tool.NewExecutor(registry, tool.ExecutorConfig{MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens, ProjectRoot: cfg.ProjectRoot}, rt.Logger)
	executor.SetRecorder(rt.ToolRecorder)
	adapter := tool.NewAgentLoopAdapter(executor)

	var sink agent.EventSink
	if !req.Quiet {
		sink = deps.NewProgressSink(progressOut)
	}

	titleGen := conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger)
	agentLoop := deps.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    rt.ContextAssembler,
		ConversationManager: rt.ConversationManager,
		ProviderRouter:      rt.ProviderRouter,
		ToolExecutor:        adapter,
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(rt.Logger),
		TitleGenerator:      titleGen,
		EventSink:           sink,
		Config:              rtpkg.BuildAgentLoopConfig(cfg, loopMaxTurns, systemPrompt),
		Logger:              rt.Logger,
	})
	defer agentLoop.Close()

	conv, err := deps.CreateConversation(ctx, rt.ConversationManager, cfg.ProjectRoot, buildConversationOptions(cfg)...)
	if err != nil {
		return nil, nil, fmt.Errorf("create conversation: %w", err)
	}
	modelContextLimit, err := rtpkg.ResolveModelContextLimit(cfg, cfg.Routing.Default.Provider)
	if err != nil {
		return nil, nil, err
	}
	turnResult, turnErr := agentLoop.RunTurn(ctx, agent.RunTurnRequest{
		ConversationID:    conv.ID,
		TurnNumber:        1,
		Message:           taskText,
		ModelContextLimit: modelContextLimit,
	})
	return turnResult, turnErr, nil
}

func determineExitStatus(ctx context.Context, turnResult *agent.TurnResult, turnErr error, loopMaxTurns int, maxTokens int) (string, ExitCode, error) {
	receiptVerdict := "completed_no_receipt"
	exitCode := ExitOK
	if turnErr != nil {
		if errors.Is(turnErr, agent.ErrTurnCancelled) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "safety_limit", ExitSafetyLimit, nil
		}
		return "", ExitInfrastructure, turnErr
	}
	if (loopMaxTurns > 0 && turnResult.IterationCount >= loopMaxTurns) || ExceededMaxTokens(turnResult, maxTokens) {
		receiptVerdict = "safety_limit"
		exitCode = ExitSafetyLimit
	}
	return receiptVerdict, exitCode, nil
}

func resolveRunLimits(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, req RunRequest) (int, int) {
	loopMaxTurns := roleCfg.MaxTurns
	if loopMaxTurns == 0 {
		loopMaxTurns = cfg.Agent.MaxIterationsPerTurn
	}
	if req.MaxTurns > 0 && (loopMaxTurns == 0 || req.MaxTurns < loopMaxTurns) {
		loopMaxTurns = req.MaxTurns
	}
	maxTokens := roleCfg.MaxTokens
	if req.MaxTokens > 0 && (maxTokens == 0 || req.MaxTokens < maxTokens) {
		maxTokens = req.MaxTokens
	}
	return loopMaxTurns, maxTokens
}

func resolveRunTimeout(roleCfg appconfig.AgentRoleConfig, requested time.Duration) time.Duration {
	roleTimeout := roleCfg.Timeout.Duration()
	if roleTimeout <= 0 {
		if requested > 0 {
			return requested
		}
		return defaultRunTimeout
	}
	if requested > 0 && requested < roleTimeout {
		return requested
	}
	return roleTimeout
}

func buildConversationOptions(cfg *appconfig.Config) []conversation.CreateOption {
	convOpts := []conversation.CreateOption{}
	if cfg.Routing.Default.Provider != "" {
		convOpts = append(convOpts, conversation.WithProvider(cfg.Routing.Default.Provider))
	}
	if cfg.Routing.Default.Model != "" {
		convOpts = append(convOpts, conversation.WithModel(cfg.Routing.Default.Model))
	}
	return convOpts
}

func cleanupRuntime(rt *rtpkg.EngineRuntime) {
	if rt == nil || rt.Cleanup == nil {
		return
	}
	rt.Cleanup()
}

func parentContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
