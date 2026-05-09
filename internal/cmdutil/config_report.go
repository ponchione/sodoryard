package cmdutil

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/embeddedprompts"
	"github.com/ponchione/sodoryard/internal/modelcap"
	"github.com/ponchione/sodoryard/internal/promptmeta"
)

func RunConfig(out io.Writer, configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "config: valid")
	_, _ = fmt.Fprintf(out, "config_path: %s\n", configPath)
	_, _ = fmt.Fprintf(out, "project_root: %s\n", cfg.ProjectRoot)
	_, _ = fmt.Fprintf(out, "server_address: %s\n", cfg.ServerAddress())
	_, _ = fmt.Fprintf(out, "default_provider: %s\n", cfg.Routing.Default.Provider)
	_, _ = fmt.Fprintf(out, "default_model: %s\n", cfg.Routing.Default.Model)
	if provider, ok := cfg.Providers[cfg.Routing.Default.Provider]; ok {
		_, _ = fmt.Fprintf(out, "default_reasoning_effort: %s\n", valueOrDefault(provider.ReasoningEffort, "<unset>"))
	}
	if model, err := modelcap.ResolveConfiguredModel(cfg, "", ""); err == nil {
		_, _ = fmt.Fprintf(out, "default_context_window: %d\n", model.ContextWindow)
		_, _ = fmt.Fprintf(out, "default_model_capabilities: %s\n", valueOrDefault(strings.Join(modelcap.CapabilityLabels(model), ","), "<none>"))
		if model.MaxOutputTokens > 0 {
			_, _ = fmt.Fprintf(out, "default_max_output_tokens: %d\n", model.MaxOutputTokens)
		}
		if len(model.KnownQuirks) > 0 {
			_, _ = fmt.Fprintf(out, "default_model_quirks: %s\n", strings.Join(model.KnownQuirks, "; "))
		}
	}
	_, _ = fmt.Fprintf(out, "fallback_provider: %s\n", valueOrDefault(cfg.Routing.Fallback.Provider, "<unset>"))
	_, _ = fmt.Fprintf(out, "fallback_model: %s\n", valueOrDefault(cfg.Routing.Fallback.Model, "<unset>"))
	if cfg.Memory.Backend == "shunter" {
		_, _ = fmt.Fprintln(out, "database_path: <unused in shunter mode>")
	} else {
		_, _ = fmt.Fprintf(out, "database_path: %s\n", cfg.DatabasePath())
	}
	_, _ = fmt.Fprintf(out, "code_index_path: %s\n", cfg.CodeLanceDBPath())
	_, _ = fmt.Fprintf(out, "shunter_memory_path: %s\n", cfg.MemoryShunterDataDir())
	_, _ = fmt.Fprintf(out, "embedding_base_url: %s\n", cfg.Embedding.BaseURL)
	_, _ = fmt.Fprintf(out, "brain_enabled: %t\n", cfg.Brain.Enabled)
	_, _ = fmt.Fprintf(out, "local_services_enabled: %t\n", cfg.LocalServices.Enabled)
	_, _ = fmt.Fprintf(out, "local_services_mode: %s\n", cfg.LocalServices.Mode)
	_, _ = fmt.Fprintf(out, "local_services_compose_file: %s\n", cfg.LocalServices.ComposeFile)
	_, _ = fmt.Fprintf(out, "local_services_project_dir: %s\n", cfg.LocalServices.ProjectDir)
	for _, warning := range promptMetadataWarnings(cfg) {
		_, _ = fmt.Fprintf(out, "prompt_warning: %s\n", warning)
	}
	return nil
}

func promptMetadataWarnings(cfg *appconfig.Config) []string {
	if cfg == nil || len(cfg.AgentRoles) == 0 {
		return nil
	}
	roleNames := make([]string, 0, len(cfg.AgentRoles))
	for roleName := range cfg.AgentRoles {
		roleNames = append(roleNames, roleName)
	}
	sort.Strings(roleNames)
	var warnings []string
	for _, roleName := range roleNames {
		roleCfg := cfg.AgentRoles[roleName]
		content, source, ok, err := promptContentForMetadata(cfg, roleName, roleCfg.SystemPrompt)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %s", roleName, err))
			continue
		}
		if !ok {
			continue
		}
		parsed := promptmeta.Parse(content)
		for _, warning := range parsed.Warnings {
			warnings = append(warnings, fmt.Sprintf("%s %s: %s", roleName, source, warning))
		}
		if !parsed.HasFrontmatter {
			continue
		}
		for _, warning := range promptmeta.ValidateRoleRuntime(roleName, roleCfg.Tools, roleCfg.MaxTurns, parsed.Metadata) {
			warnings = append(warnings, fmt.Sprintf("%s %s: %s", roleName, source, warning))
		}
	}
	return warnings
}

func promptContentForMetadata(cfg *appconfig.Config, roleName string, configuredValue string) (string, string, bool, error) {
	if cfg == nil {
		return "", "", false, nil
	}
	trimmed := strings.TrimSpace(configuredValue)
	if strings.HasPrefix(trimmed, "builtin:") {
		builtinRole := strings.TrimSpace(strings.TrimPrefix(trimmed, "builtin:"))
		prompt, ok := embeddedprompts.Get(builtinRole)
		if !ok {
			return "", "", false, fmt.Errorf("unknown built-in role system prompt %q", builtinRole)
		}
		return prompt, "embedded:" + builtinRole, true, nil
	}
	if trimmed == "" {
		prompt, ok := embeddedprompts.Get(roleName)
		if !ok {
			return "", "", false, nil
		}
		return prompt, "embedded:" + roleName, true, nil
	}
	resolved := cfg.ResolveAgentRoleSystemPromptPath(trimmed)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", "", false, fmt.Errorf("read prompt metadata from %s: %w", resolved, err)
	}
	return string(data), "file:" + resolved, true, nil
}
