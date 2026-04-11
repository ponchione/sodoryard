package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/role"
	"github.com/ponchione/sodoryard/internal/tool"
)

const (
	runExitOK             = 0
	runExitInfrastructure = 1
	runExitSafetyLimit    = 2
	runExitEscalation     = 3
)

var buildRunRuntime = buildAppRuntime
var buildRunRoleRegistry = role.BuildRegistry

type exitCoder interface {
	ExitCode() int
}

type runExitError struct {
	code int
	err  error
}

func (e runExitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e runExitError) Unwrap() error { return e.err }
func (e runExitError) ExitCode() int { return e.code }

type runFlags struct {
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

type runExecutionResult struct {
	ReceiptPath string
	ExitCode    int
}

func newRunCmd(configPath *string) *cobra.Command {
	flags := runFlags{Timeout: 30 * time.Minute}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one autonomous headless agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runHeadless(cmd, *configPath, flags)
			if result != nil && (result.ExitCode == runExitOK || result.ExitCode == runExitSafetyLimit) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.ReceiptPath)
			}
			if err != nil {
				return err
			}
			if result != nil && result.ExitCode != runExitOK {
				return runExitError{code: result.ExitCode, err: fmt.Errorf("headless run exited with code %d", result.ExitCode)}
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

func runHeadless(cmd *cobra.Command, configPath string, flags runFlags) (*runExecutionResult, error) {
	if strings.TrimSpace(flags.Role) == "" {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("--role is required")}
	}
	if (strings.TrimSpace(flags.Task) == "") == (strings.TrimSpace(flags.TaskFile) == "") {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("exactly one of --task or --task-file is required")}
	}
	if flags.MaxTurns < 0 {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("--max-turns must be > 0 when supplied")}
	}
	if flags.MaxTokens < 0 {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("--max-tokens must be > 0 when supplied")}
	}
	if flags.Timeout <= 0 {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("--timeout must be > 0")}
	}

	taskText, err := readTask(flags.Task, flags.TaskFile)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("load config: %w", err)}
	}
	if strings.TrimSpace(flags.ProjectRoot) != "" {
		cfg.ProjectRoot = strings.TrimSpace(flags.ProjectRoot)
	}
	if strings.TrimSpace(flags.Brain) != "" {
		cfg.Brain.VaultPath = strings.TrimSpace(flags.Brain)
	}
	if err := cfg.Validate(); err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}

	roleCfg, ok := cfg.AgentRoles[flags.Role]
	if !ok {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("agent role %q not found in config", flags.Role)}
	}
	systemPrompt, err := loadRoleSystemPrompt(cfg.ProjectRoot, roleCfg.SystemPrompt)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}
	chainID := resolveChainID(flags.ChainID)
	ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
	defer cancel()

	runtimeBundle, err := buildRunRuntime(ctx, cfg)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}
	defer runtimeBundle.Cleanup()

	registry, scopedBrainCfg, err := buildRunRoleRegistry(cfg, roleCfg, role.BuilderDeps{
		BrainBackend:     runtimeBundle.BrainBackend,
		BrainSearcher:    runtimeBundle.BrainSearcher,
		SemanticSearcher: runtimeBundle.SemanticSearcher,
		ProviderRuntime:  runtimeBundle.ProviderRouter,
		Queries:          runtimeBundle.Queries,
		ProjectID:        cfg.ProjectRoot,
	})
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}

	executor := tool.NewExecutor(registry, tool.ExecutorConfig{MaxOutputTokens: cfg.Agent.ToolOutputMaxTokens, ProjectRoot: cfg.ProjectRoot}, runtimeBundle.Logger)
	executor.SetRecorder(tool.NewToolExecutionRecorder(runtimeBundle.Queries))
	adapter := tool.NewAgentLoopAdapter(executor)
	var sink agent.EventSink
	if !flags.Quiet {
		sink = newRunProgressSink(cmd.ErrOrStderr())
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

	titleGen := conversation.NewTitleGen(runtimeBundle.ConversationManager, runtimeBundle.ProviderRouter, cfg.Routing.Default.Model, runtimeBundle.Logger)
	agentLoop := agent.NewAgentLoop(agent.AgentLoopDeps{
		ContextAssembler:    runtimeBundle.ContextAssembler,
		ConversationManager: runtimeBundle.ConversationManager,
		ProviderRouter:      runtimeBundle.ProviderRouter,
		ToolExecutor:        adapter,
		ToolDefinitions:     registry.ToolDefinitions(),
		PromptBuilder:       agent.NewPromptBuilder(runtimeBundle.Logger),
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
		Logger: runtimeBundle.Logger,
	})
	defer agentLoop.Close()

	convOpts := []conversation.CreateOption{}
	if cfg.Routing.Default.Provider != "" {
		convOpts = append(convOpts, conversation.WithProvider(cfg.Routing.Default.Provider))
	}
	if cfg.Routing.Default.Model != "" {
		convOpts = append(convOpts, conversation.WithModel(cfg.Routing.Default.Model))
	}
	conv, err := runtimeBundle.ConversationManager.Create(ctx, cfg.ProjectRoot, convOpts...)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: fmt.Errorf("create conversation: %w", err)}
	}
	modelContextLimit, err := resolveModelContextLimit(cfg, cfg.Routing.Default.Provider)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}
	turnResult, turnErr := agentLoop.RunTurn(ctx, agent.RunTurnRequest{
		ConversationID:    conv.ID,
		TurnNumber:        1,
		Message:           taskText,
		ModelContextLimit: modelContextLimit,
	})

	receiptVerdict := "completed_no_receipt"
	exitCode := runExitOK
	if turnErr != nil {
		if errors.Is(turnErr, agent.ErrTurnCancelled) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			receiptVerdict = "safety_limit"
			exitCode = runExitSafetyLimit
		} else {
			return nil, runExitError{code: runExitInfrastructure, err: turnErr}
		}
	} else if (loopMaxTurns > 0 && turnResult.IterationCount >= loopMaxTurns) || exceededMaxTokens(turnResult, maxTokens) {
		receiptVerdict = "safety_limit"
		exitCode = runExitSafetyLimit
	}

	receiptPath, receipt, err := ensureReceipt(ctx, runtimeBundle.BrainBackend, scopedBrainCfg, flags.Role, chainID, resolveReceiptPath(flags.Role, chainID, flags.ReceiptPath), receiptVerdict, finalText(turnResult), turnResult)
	if err != nil {
		return nil, runExitError{code: runExitInfrastructure, err: err}
	}
	if receipt != nil {
		switch receipt.Verdict {
		case "escalate":
			exitCode = runExitEscalation
		case "safety_limit":
			exitCode = runExitSafetyLimit
		}
	}
	return &runExecutionResult{ReceiptPath: receiptPath, ExitCode: exitCode}, nil
}

func readTask(task string, taskFile string) (string, error) {
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

func resolveChainID(input string) string {
	if strings.TrimSpace(input) != "" {
		return strings.TrimSpace(input)
	}
	return id.New()
}

func loadRoleSystemPrompt(projectRoot string, promptPath string) (string, error) {
	cfg := &appconfig.Config{ProjectRoot: projectRoot}
	resolved := cfg.ResolveAgentRoleSystemPromptPath(promptPath)
	if strings.TrimSpace(resolved) == "" {
		return "", fmt.Errorf("role system_prompt is required")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read role system prompt %s: %w", resolved, err)
	}
	return string(data), nil
}

func resolveModelContextLimit(cfg *appconfig.Config, providerName string) (int, error) {
	if cfg == nil {
		return 0, fmt.Errorf("config is required")
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return 0, fmt.Errorf("unknown provider: %s", providerName)
	}
	if providerCfg.ContextLength > 0 {
		return providerCfg.ContextLength, nil
	}
	switch providerCfg.Type {
	case "anthropic", "codex":
		return 200000, nil
	case "openai-compatible":
		return 32768, nil
	default:
		return 0, fmt.Errorf("provider %s has no positive context_length configured", providerName)
	}
}

func exceededMaxTokens(turnResult *agent.TurnResult, maxTokens int) bool {
	if turnResult == nil || maxTokens <= 0 {
		return false
	}
	used := turnResult.TotalUsage.InputTokens + turnResult.TotalUsage.OutputTokens
	return used >= maxTokens
}

func finalText(turnResult *agent.TurnResult) string {
	if turnResult == nil {
		return ""
	}
	return strings.TrimSpace(turnResult.FinalText)
}

func writeString(out io.Writer, value string) {
	if out == nil {
		return
	}
	_, _ = fmt.Fprintln(out, value)
}
