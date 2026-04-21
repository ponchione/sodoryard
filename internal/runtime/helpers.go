// Package runtime provides shared runtime construction helpers used by
// cmd/yard and cmd/tidmouth. It exists because Go does not allow importing
// main packages across binaries.
package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/embeddedprompts"
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

// EnsureProjectRecord upserts the project row in the projects table so
// that downstream queries referencing project_id can join against it.
func EnsureProjectRecord(ctx context.Context, database *sql.DB, cfg *appconfig.Config) error {
	if ctx == nil {
		ctx = context.Background()
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
