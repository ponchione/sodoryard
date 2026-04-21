package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/spf13/cobra"
)

const (
	chainVerbosityNormal = "normal"
	chainVerbosityDebug  = "debug"
)

type chainRenderOptions struct {
	Verbosity string
}

func newLogsCmd(configPath *string) *cobra.Command {
	var follow bool
	var verbosity string
	cmd := &cobra.Command{Use: "logs <chain-id>", Short: "Show chain event log", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := buildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		if !follow {
			events, err := rt.ChainStore.ListEvents(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, event := range events {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), formatChainEvent(event, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)}))
			}
			return nil
		}
		return followChainEvents(cmd.Context(), cmd.OutOrStdout(), rt.ChainStore, args[0], 0, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll and print new events until the chain stops")
	cmd.Flags().StringVar(&verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func streamChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) (int64, error) {
	events, err := store.ListEventsSince(ctx, chainID, afterID)
	if err != nil {
		return afterID, err
	}
	for _, event := range events {
		_, _ = fmt.Fprint(out, formatChainEvent(event, opts))
		afterID = event.ID
	}
	return afterID, nil
}

func followChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) error {
	for {
		nextAfterID, err := streamChainEvents(ctx, out, store, chainID, afterID, opts)
		if err != nil {
			return err
		}
		afterID = nextAfterID
		ch, err := store.GetChain(ctx, chainID)
		if err != nil {
			return err
		}
		if ch.Status != "running" && ch.Status != "pause_requested" && ch.Status != "cancel_requested" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func formatChainEvent(event chain.Event, opts chainRenderOptions) string {
	formatted := formatKnownChainEvent(event, opts)
	if formatted == "" {
		if event.EventType == chain.EventStepOutput {
			return ""
		}
		formatted = event.EventData
		if formatted == "" {
			return ""
		}
	}
	return fmt.Sprintf("%d\t%s\t%s\t%s\n", event.ID, event.CreatedAt.Format(time.RFC3339), event.EventType, formatted)
}

func shouldSuppressStepOutput(line string, opts chainRenderOptions) bool {
	if normalizeChainVerbosity(opts.Verbosity) == chainVerbosityDebug {
		return false
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	for _, noisy := range []string{
		"provider registered",
		"registered provider",
		"provider failed ping() startup validation",
		"brain backend: mcp (in-process)",
		"status: waiting_for_llm",
		"status: executing_tools",
		"status: assembling_context",
	} {
		if strings.Contains(trimmed, noisy) {
			return true
		}
	}
	return false
}

func normalizeChainVerbosity(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), chainVerbosityDebug) {
		return chainVerbosityDebug
	}
	return chainVerbosityNormal
}

func formatKnownChainEvent(event chain.Event, opts chainRenderOptions) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
		return ""
	}
	quoted := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf(`%s=%q`, key, text)
	}
	plain := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf("%s=%s", key, text)
	}
	join := func(parts ...string) string {
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				filtered = append(filtered, part)
			}
		}
		return strings.Join(filtered, " ")
	}

	switch event.EventType {
	case chain.EventChainStarted:
		return join(quoted("task"), plain("orchestrator_pid"), plain("execution_id"), plain("continued_by"), plain("resumed_by"))
	case chain.EventChainResumed:
		return join(plain("resumed_by"), plain("continued_by"), plain("orchestrator_pid"), plain("execution_id"))
	case chain.EventStepStarted:
		return join(plain("role"), quoted("task"), plain("receipt_path"))
	case chain.EventStepOutput:
		line := strings.TrimSpace(fmt.Sprint(payload["line"]))
		if line == "" || line == "<nil>" || shouldSuppressStepOutput(line, opts) {
			return ""
		}
		stream := strings.TrimSpace(fmt.Sprint(payload["stream"]))
		if stream == "" || stream == "<nil>" {
			stream = "stdout"
		}
		return fmt.Sprintf("[%s] %s", stream, line)
	case chain.EventStepCompleted, chain.EventStepFailed:
		return join(plain("role"), plain("verdict"), plain("tokens_used"), plain("duration_secs"), plain("exit_code"), quoted("error"))
	case chain.EventResolverLoop:
		return join(plain("count"), quoted("task_context"))
	case chain.EventReindexStarted, chain.EventReindexCompleted, chain.EventSafetyLimitHit, chain.EventChainPaused, chain.EventChainCancelled, chain.EventChainCompleted:
		parts := make([]string, 0, len(payload))
		for _, key := range []string{"status", "summary", "duration_secs", "limit", "role", "exit_code", "execution_id", "finalized_from"} {
			if key == "summary" {
				parts = append(parts, quoted(key))
				continue
			}
			parts = append(parts, plain(key))
		}
		return join(parts...)
	default:
		return ""
	}
}
