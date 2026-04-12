package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/role"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

const (
	yardRunExitOK             = 0
	yardRunExitInfrastructure = 1
	yardRunExitSafetyLimit    = 2
	yardRunExitEscalation     = 3
)

type yardRunExitError struct {
	code int
	err  error
}

func (e yardRunExitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e yardRunExitError) Unwrap() error { return e.err }
func (e yardRunExitError) ExitCode() int { return e.code }

type yardRunFlags struct {
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

type yardRunResult struct {
	ReceiptPath string
	ExitCode    int
}

func newYardRunCmd(configPath *string) *cobra.Command {
	flags := yardRunFlags{Timeout: 30 * time.Minute}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one autonomous headless agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := yardRunHeadless(cmd, *configPath, flags)
			if result != nil && (result.ExitCode == yardRunExitOK || result.ExitCode == yardRunExitSafetyLimit) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.ReceiptPath)
			}
			if err != nil {
				return err
			}
			if result != nil && result.ExitCode != yardRunExitOK {
				return yardRunExitError{code: result.ExitCode, err: fmt.Errorf("headless run exited with code %d", result.ExitCode)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.Role, "role", "", "Agent role from config")
	cmd.Flags().StringVar(&flags.Task, "task", "", "Task text for the headless run")
	cmd.Flags().StringVar(&flags.TaskFile, "task-file", "", "Read task text from file")
	cmd.Flags().StringVar(&flags.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().StringVar(&flags.Brain, "brain", "", "Override brain vault path")
	cmd.Flags().IntVar(&flags.MaxTurns, "max-turns", 0, "Override max turns for this run")
	cmd.Flags().IntVar(&flags.MaxTokens, "max-tokens", 0, "Override max total tokens for this run")
	cmd.Flags().DurationVar(&flags.Timeout, "timeout", 30*time.Minute, "Wall-clock timeout for the entire session")
	cmd.Flags().StringVar(&flags.ReceiptPath, "receipt-path", "", "Override brain-relative receipt path")
	cmd.Flags().BoolVar(&flags.Quiet, "quiet", false, "Suppress progress output")
	cmd.Flags().StringVar(&flags.ProjectRoot, "project-root", "", "Override project root")
	return cmd
}

func yardRunHeadless(cmd *cobra.Command, configPath string, flags yardRunFlags) (*yardRunResult, error) {
	if strings.TrimSpace(flags.Role) == "" {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("--role is required")}
	}
	if (strings.TrimSpace(flags.Task) == "") == (strings.TrimSpace(flags.TaskFile) == "") {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("exactly one of --task or --task-file is required")}
	}
	if flags.MaxTurns < 0 {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("--max-turns must be > 0 when supplied")}
	}
	if flags.MaxTokens < 0 {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("--max-tokens must be > 0 when supplied")}
	}
	if flags.Timeout <= 0 {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("--timeout must be > 0")}
	}

	taskText, err := yardReadTask(flags.Task, flags.TaskFile)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("load config: %w", err)}
	}
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
	if strings.TrimSpace(flags.Brain) != "" {
		cfg.Brain.VaultPath = strings.TrimSpace(flags.Brain)
	}
	if err := cfg.Validate(); err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}

	roleCfg, ok := cfg.AgentRoles[flags.Role]
	if !ok {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("agent role %q not found in config", flags.Role)}
	}
	systemPrompt, err := rtpkg.LoadRoleSystemPrompt(cfg.ProjectRoot, roleCfg.SystemPrompt)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}
	chainID := strings.TrimSpace(flags.ChainID)
	if chainID == "" {
		chainID = id.New()
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
	defer cancel()

	rt, err := rtpkg.BuildEngineRuntime(ctx, cfg)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}
	defer rt.Cleanup()

	registry, scopedBrainCfg, err := role.BuildRegistry(cfg, roleCfg, role.BuilderDeps{
		BrainBackend:     rt.BrainBackend,
		BrainSearcher:    rt.BrainSearcher,
		SemanticSearcher: rt.SemanticSearcher,
		ProviderRuntime:  rt.ProviderRouter,
		Queries:          rt.Queries,
		ProjectID:        cfg.ProjectRoot,
	})
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens, ProjectRoot: cfg.ProjectRoot}, rt.Logger)
	executor.SetRecorder(tool.NewToolExecutionRecorder(rt.Queries))
	adapter := tool.NewAgentLoopAdapter(executor)
	var sink agent.EventSink
	if !flags.Quiet {
		sink = newYardRunProgressSink(cmd.ErrOrStderr())
	}
	loopMaxTurns := roleCfg.MaxTurns
	if flags.MaxTurns > 0 {
		loopMaxTurns = flags.MaxTurns
	}
	maxTokens := roleCfg.MaxTokens
	if flags.MaxTokens > 0 {
		maxTokens = flags.MaxTokens
	}
	if loopMaxTurns == 0 {
		loopMaxTurns = cfg.Agent.MaxIterationsPerTurn
	}

	titleGen := conversation.NewTitleGen(rt.ConversationManager, rt.ProviderRouter, cfg.Routing.Default.Model, rt.Logger)
	agentLoop := agent.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    rt.ContextAssembler,
		ConversationManager: rt.ConversationManager,
		ProviderRouter:      rt.ProviderRouter,
		ToolExecutor:        adapter,
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(rt.Logger),
		TitleGenerator:      titleGen,
		EventSink:           sink,
		Config: agent.AgentLoopConfig{
			MaxIterations:              loopMaxTurns,
			LoopDetectionThreshold:     cfg.Agent.LoopDetectionThreshold,
			ExtendedThinking:           cfg.Agent.ExtendedThinking,
			BasePrompt:                 systemPrompt,
			ProviderName:               cfg.Routing.Default.Provider,
			ModelName:                  cfg.Routing.Default.Model,
			EmitContextDebug:           cfg.Context.EmitContextDebug,
			ContextConfig:              cfg.Context,
			ToolResultStoreRoot:        cfg.Agent.ToolResultStoreRoot,
			CacheSystemPrompt:          cfg.Agent.CacheSystemPrompt,
			CacheAssembledContext:      cfg.Agent.CacheAssembledContext,
			CacheConversationHistory:   cfg.Agent.CacheConversationHistory,
			CompressHistoricalResults:  cfg.Agent.CompressHistoricalResults,
			StripHistoricalLineNumbers: cfg.Agent.StripHistoricalLineNumbers,
			ElideDuplicateReads:        cfg.Agent.ElideDuplicateReads,
			HistorySummarizeAfterTurns: cfg.Agent.HistorySummarizeAfterTurns,
		},
		Logger: rt.Logger,
	})
	defer agentLoop.Close()

	convOpts := []conversation.CreateOption{}
	if cfg.Routing.Default.Provider != "" {
		convOpts = append(convOpts, conversation.WithProvider(cfg.Routing.Default.Provider))
	}
	if cfg.Routing.Default.Model != "" {
		convOpts = append(convOpts, conversation.WithModel(cfg.Routing.Default.Model))
	}
	conv, err := rt.ConversationManager.Create(ctx, cfg.ProjectRoot, convOpts...)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: fmt.Errorf("create conversation: %w", err)}
	}
	modelContextLimit, err := rtpkg.ResolveModelContextLimit(cfg, cfg.Routing.Default.Provider)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}
	turnResult, turnErr := agentLoop.RunTurn(ctx, agent.RunTurnRequest{
		ConversationID:    conv.ID,
		TurnNumber:        1,
		Message:           taskText,
		ModelContextLimit: modelContextLimit,
	})

	receiptVerdict := "completed_no_receipt"
	exitCode := yardRunExitOK
	if turnErr != nil {
		if errors.Is(turnErr, agent.ErrTurnCancelled) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			receiptVerdict = "safety_limit"
			exitCode = yardRunExitSafetyLimit
		} else {
			return nil, yardRunExitError{code: yardRunExitInfrastructure, err: turnErr}
		}
	} else if (loopMaxTurns > 0 && turnResult.IterationCount >= loopMaxTurns) || yardExceededMaxTokens(turnResult, maxTokens) {
		receiptVerdict = "safety_limit"
		exitCode = yardRunExitSafetyLimit
	}

	receiptPath, r, err := yardEnsureReceipt(ctx, rt.BrainBackend, scopedBrainCfg, flags.Role, chainID, yardResolveReceiptPath(flags.Role, chainID, flags.ReceiptPath), receiptVerdict, yardFinalText(turnResult), turnResult)
	if err != nil {
		return nil, yardRunExitError{code: yardRunExitInfrastructure, err: err}
	}
	if r != nil {
		switch r.Verdict {
		case "escalate":
			exitCode = yardRunExitEscalation
		case "safety_limit":
			exitCode = yardRunExitSafetyLimit
		}
	}
	return &yardRunResult{ReceiptPath: receiptPath, ExitCode: exitCode}, nil
}

func yardReadTask(task string, taskFile string) (string, error) {
	if strings.TrimSpace(task) != "" && strings.TrimSpace(taskFile) != "" {
		return "", fmt.Errorf("--task and --task-file are mutually exclusive")
	}
	if strings.TrimSpace(task) != "" {
		return strings.TrimSpace(task), nil
	}
	if strings.TrimSpace(taskFile) == "" {
		return "", fmt.Errorf("task text is required")
	}
	data, err := os.ReadFile(strings.TrimSpace(taskFile))
	if err != nil {
		return "", fmt.Errorf("read task file: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("task file is empty")
	}
	return text, nil
}
