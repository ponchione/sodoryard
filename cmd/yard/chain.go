package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

var errYardChainPIDNotRunning = errors.New("chain orchestrator pid not running")

var interruptYardChainPID = func(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return errYardChainPIDNotRunning
		}
		return err
	}
	return nil
}

type yardChainFlags struct {
	Specs            string
	Task             string
	Role             string
	ChainID          string
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
	StepMaxTurns     int
	StepMaxTokens    int
	DryRun           bool
	Watch            bool
	Verbosity        string
	ProjectRoot      string
}

const (
	yardChainWatchFlushTimeout = 2 * time.Second
	chainVerbosityNormal       = "normal"
	chainVerbosityDebug        = "debug"
)

type chainRenderOptions struct {
	Verbosity string
}

type chainTurnRunner = chainrun.TurnRunner

var buildYardChainRuntime = rtpkg.BuildOrchestratorRuntime
var buildYardChainRegistry = rtpkg.BuildOrchestratorRegistry
var newYardChainTurnRunner = func(deps agent.AgentLoopDeps) chainTurnRunner { return agent.NewAgentLoop(deps) }

func newYardChainCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chain",
		Short: "Chain orchestration commands",
	}
	cmd.AddCommand(
		newYardChainStartCmd(configPath),
		newYardChainStatusCmd(configPath),
		newYardChainMetricsCmd(configPath),
		newYardChainLogsCmd(configPath),
		newYardChainReceiptCmd(configPath),
		newYardChainCancelCmd(configPath),
		newYardChainPauseCmd(configPath),
		newYardChainResumeCmd(configPath),
	)
	return cmd
}

func newYardChainStartCmd(configPath *string) *cobra.Command {
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new chain execution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateYardChainFlags(flags); err != nil {
				return err
			}
			return yardRunChain(cmd.Context(), *configPath, flags, cmd)
		},
	}
	cmd.Flags().StringVar(&flags.Specs, "specs", "", "Comma-separated brain-relative paths to spec docs")
	cmd.Flags().StringVar(&flags.Task, "task", "", "Free-form task description")
	cmd.Flags().StringVar(&flags.Role, "role", "", "Run a one-step chain with the selected role instead of the orchestrator")
	cmd.Flags().StringVar(&flags.ProjectRoot, "project", "", "Override project root")
	cmd.Flags().StringVar(&flags.ChainID, "chain-id", "", "Chain execution identifier")
	cmd.Flags().IntVar(&flags.MaxSteps, "max-steps", 100, "Maximum total agent invocations")
	cmd.Flags().IntVar(&flags.MaxResolverLoops, "max-resolver-loops", 3, "Maximum fix-audit cycles per task")
	cmd.Flags().DurationVar(&flags.MaxDuration, "max-duration", 4*time.Hour, "Wall-clock timeout for entire chain")
	cmd.Flags().IntVar(&flags.TokenBudget, "token-budget", 5_000_000, "Total token ceiling across all agents")
	cmd.Flags().IntVar(&flags.StepMaxTurns, "step-max-turns", 0, "Optional maximum model iterations for each spawned headless step")
	cmd.Flags().IntVar(&flags.StepMaxTokens, "step-max-tokens", 0, "Optional total token ceiling for each spawned headless step")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Create the chain row but do not run the orchestrator")
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func yardRunChain(ctx context.Context, configPath string, flags yardChainFlags, cmd *cobra.Command) (err error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	applyYardChainOverrides(cfg, flags)
	if err := cfg.Validate(); err != nil {
		return err
	}

	_, err = chainrun.Start(ctx, cfg, chainrun.Options{
		ChainID:          flags.ChainID,
		Role:             strings.TrimSpace(flags.Role),
		SourceSpecs:      yardParseSpecs(flags.Specs),
		SourceTask:       strings.TrimSpace(flags.Task),
		MaxSteps:         flags.MaxSteps,
		MaxResolverLoops: flags.MaxResolverLoops,
		MaxDuration:      flags.MaxDuration,
		TokenBudget:      flags.TokenBudget,
		StepMaxTurns:     flags.StepMaxTurns,
		StepMaxTokens:    flags.StepMaxTokens,
		DryRun:           flags.DryRun,
		OnChainID: func(chainID string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", chainID)
		},
		OnMessage: func(message string) {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), message)
		},
		StartWatch: func(ctx context.Context, store *chain.Store, chainID string) chainrun.WatchHandle {
			return yardChainWatchAdapter{handle: startYardChainWatch(ctx, cmd.ErrOrStderr(), store, chainID, flags.Watch, chainRenderOptions{Verbosity: normalizeChainVerbosity(flags.Verbosity)})}
		},
		WatchFlushTimeout: yardChainWatchFlushTimeout,
	}, chainrun.Deps{
		BuildRuntime:  buildYardChainRuntime,
		BuildRegistry: buildYardChainRegistry,
		NewTurnRunner: newYardChainTurnRunner,
		ProcessID:     os.Getpid,
	})
	return err
}

func newYardChainCancelCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "cancel <chain-id>", Short: "Cancel a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return yardSetChainStatus(cmd, *configPath, args[0], "cancelled")
	}}
}

func newYardChainPauseCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "pause <chain-id>", Short: "Pause a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return yardSetChainStatus(cmd, *configPath, args[0], "paused")
	}}
}

func newYardChainResumeCmd(configPath *string) *cobra.Command {
	flags := yardChainFlags{MaxSteps: 100, MaxResolverLoops: 3, MaxDuration: 4 * time.Hour, TokenBudget: 5_000_000, Watch: true, Verbosity: chainVerbosityNormal}
	cmd := &cobra.Command{Use: "resume <chain-id>", Short: "Resume a paused chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		flags.ChainID = args[0]
		return yardRunChain(cmd.Context(), *configPath, flags, cmd)
	}}
	cmd.Flags().BoolVar(&flags.Watch, "watch", true, "Stream live chain progress to stderr while the command runs")
	cmd.Flags().StringVar(&flags.Verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}
