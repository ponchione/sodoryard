// Package runtime provides shared runtime construction helpers used by
// cmd/yard and cmd/tidmouth. It exists because Go does not allow importing
// main packages across binaries.
package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/embeddedprompts"
	"github.com/ponchione/sodoryard/internal/logging"
)

// ChainCleanup extends a teardown chain without falling into the closure
// capture-by-reference trap. Each call captures prev as a value parameter,
// so later extensions get a fresh copy rather than sharing one variable that
// eventually points at the final extension and self-recurses.
func ChainCleanup(prev func(), next func()) func() {
	return func() {
		next()
		if prev != nil {
			prev()
		}
	}
}

type runtimeBase struct {
	logger   *slog.Logger
	database *sql.DB
	queries  *appdb.Queries
	cleanup  func()
}

func buildRuntimeBase(ctx context.Context, cfg *appconfig.Config) (*runtimeBase, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}

	logger, err := logging.Init(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return nil, fmt.Errorf("init logging: %w", err)
	}

	if cfg.Memory.Backend == "shunter" {
		return &runtimeBase{
			logger:  logger,
			cleanup: func() {},
		}, nil
	}

	database, err := appdb.OpenDB(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	cleanup := func() {
		_ = database.Close()
	}

	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		cleanup()
		return nil, fmt.Errorf("init database schema: %w", err)
	}
	for _, upgrade := range []struct {
		label string
		fn    func(context.Context, *sql.DB) error
	}{
		{"upgrade message search indexes", appdb.EnsureMessageSearchIndexesIncludeTools},
		{"upgrade context report token budget storage", appdb.EnsureContextReportsIncludeTokenBudget},
		{"ensure chain schema", appdb.EnsureChainSchema},
		{"ensure launch schema", appdb.EnsureLaunchSchema},
	} {
		if err := upgrade.fn(ctx, database); err != nil {
			cleanup()
			return nil, fmt.Errorf("%s: %w", upgrade.label, err)
		}
	}

	return &runtimeBase{
		logger:   logger,
		database: database,
		queries:  appdb.New(database),
		cleanup:  cleanup,
	}, nil
}

func BuildAgentLoopConfig(cfg *appconfig.Config, maxIterations int, basePrompt string) agent.AgentLoopConfig {
	return agent.AgentLoopConfig{
		MaxIterations:              maxIterations,
		LoopDetectionThreshold:     cfg.Agent.LoopDetectionThreshold,
		ExtendedThinking:           cfg.Agent.ExtendedThinking,
		BasePrompt:                 basePrompt,
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
	}
}

// EnsureProjectRecord upserts the project row in the projects table so
// that downstream queries referencing project_id can join against it.
func EnsureProjectRecord(ctx context.Context, database *sql.DB, cfg *appconfig.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	if database == nil {
		if cfg.Memory.Backend == "shunter" {
			return nil
		}
		return fmt.Errorf("database is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	name := filepath.Base(cfg.ProjectRoot)
	_, err := database.ExecContext(ctx, `
INSERT INTO projects(id, name, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	root_path = excluded.root_path,
	updated_at = excluded.updated_at
`, cfg.ProjectRoot, name, cfg.ProjectRoot, now, now)
	return err
}

// LoadRoleSystemPrompt resolves and returns the system prompt content for an
// agent role. Resolution order is: explicit builtin selector, explicit file
// override, then the built-in default for the role when configuredValue is
// empty.
func LoadRoleSystemPrompt(roleName string, projectRoot string, configuredValue string) (string, string, error) {
	trimmed := strings.TrimSpace(configuredValue)
	if strings.HasPrefix(trimmed, "builtin:") {
		builtinRole := strings.TrimSpace(strings.TrimPrefix(trimmed, "builtin:"))
		prompt, ok := embeddedprompts.Get(builtinRole)
		if !ok {
			return "", "", fmt.Errorf("unknown built-in role system prompt %q", builtinRole)
		}
		return prompt, "embedded:" + builtinRole, nil
	}
	if trimmed != "" {
		cfg := &appconfig.Config{ProjectRoot: projectRoot}
		resolved := cfg.ResolveAgentRoleSystemPromptPath(trimmed)
		if _, err := os.Stat(resolved); err != nil {
			if os.IsNotExist(err) {
				return "", "", fmt.Errorf("missing role system prompt override %s", resolved)
			}
			return "", "", fmt.Errorf("stat role system prompt %s: %w", resolved, err)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return "", "", fmt.Errorf("read role system prompt %s: %w", resolved, err)
		}
		return string(data), "file:" + resolved, nil
	}
	prompt, ok := embeddedprompts.Get(roleName)
	if !ok {
		return "", "", fmt.Errorf("no built-in role system prompt for role %q", roleName)
	}
	return prompt, "embedded:" + strings.TrimSpace(roleName), nil
}

// ResolveModelContextLimit returns the context window size for a provider,
// either from explicit config or from built-in defaults per provider type.
func ResolveModelContextLimit(cfg *appconfig.Config, providerName string) (int, error) {
	return appconfig.ResolveModelContextLimit(cfg, providerName)
}
