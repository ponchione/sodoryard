package modelcap

import (
	"fmt"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

func ResolveConfiguredModel(cfg *appconfig.Config, providerName string, modelName string) (provider.Model, error) {
	if cfg == nil {
		return provider.Model{}, fmt.Errorf("config is required")
	}
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		providerName = strings.TrimSpace(cfg.Routing.Default.Provider)
	}
	if providerName == "" {
		return provider.Model{}, fmt.Errorf("provider name is required")
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return provider.Model{}, fmt.Errorf("unknown provider: %s", providerName)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = strings.TrimSpace(providerCfg.Model)
	}
	if modelName == "" && providerName == strings.TrimSpace(cfg.Routing.Default.Provider) {
		modelName = strings.TrimSpace(cfg.Routing.Default.Model)
	}
	if modelName == "" {
		return provider.Model{}, fmt.Errorf("model name is required")
	}

	model := defaultModelForProviderType(providerCfg.Type)
	model.ID = modelName
	model.Name = modelName
	model.Provider = providerName
	if contextWindow, err := appconfig.ResolveModelContextLimit(cfg, providerName); err == nil {
		model.ContextWindow = contextWindow
	} else if providerCfg.ContextLength > 0 {
		model.ContextWindow = providerCfg.ContextLength
	}
	applyProviderOverrides(&model, providerCfg)
	return model, nil
}

func defaultModelForProviderType(providerType string) provider.Model {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "codex":
		return provider.Model{
			ContextWindow:           400000,
			SupportsTools:           true,
			SupportsReasoningEffort: true,
			KnownQuirks:             []string{"runtime requests are currently pinned to gpt-5.5 by the Codex adapter"},
		}
	case "anthropic":
		return provider.Model{
			ContextWindow:       200000,
			SupportsTools:       true,
			SupportsThinking:    true,
			SupportsPromptCache: true,
		}
	case "openai-compatible":
		return provider.Model{
			ContextWindow: 32768,
			SupportsTools: true,
			KnownQuirks:   []string{"capabilities depend on the configured OpenAI-compatible endpoint"},
		}
	default:
		return provider.Model{}
	}
}

func applyProviderOverrides(model *provider.Model, cfg appconfig.ProviderConfig) {
	if model == nil {
		return
	}
	if cfg.SupportsTools != nil {
		model.SupportsTools = *cfg.SupportsTools
	}
	if cfg.SupportsThinking != nil {
		model.SupportsThinking = *cfg.SupportsThinking
	}
	if cfg.SupportsReasoningEffort != nil {
		model.SupportsReasoningEffort = *cfg.SupportsReasoningEffort
	}
	if cfg.SupportsStructuredOutput != nil {
		model.SupportsStructuredOutput = *cfg.SupportsStructuredOutput
	}
	if cfg.SupportsPromptCache != nil {
		model.SupportsPromptCache = *cfg.SupportsPromptCache
	}
	if cfg.SupportsImages != nil {
		model.SupportsImages = *cfg.SupportsImages
	}
	if cfg.SupportsToolChoice != nil {
		model.SupportsToolChoice = *cfg.SupportsToolChoice
	}
	if cfg.MaxOutputTokens > 0 {
		model.MaxOutputTokens = cfg.MaxOutputTokens
	}
	if len(cfg.KnownQuirks) > 0 {
		model.KnownQuirks = append(model.KnownQuirks, compactStrings(cfg.KnownQuirks)...)
	}
}

func CapabilityLabels(model provider.Model) []string {
	labels := make([]string, 0, 7)
	if model.SupportsTools {
		labels = append(labels, "tools")
	}
	if model.SupportsThinking {
		labels = append(labels, "thinking")
	}
	if model.SupportsReasoningEffort {
		labels = append(labels, "reasoning_effort")
	}
	if model.SupportsStructuredOutput {
		labels = append(labels, "structured_output")
	}
	if model.SupportsPromptCache {
		labels = append(labels, "prompt_cache")
	}
	if model.SupportsImages {
		labels = append(labels, "images")
	}
	if model.SupportsToolChoice {
		labels = append(labels, "tool_choice")
	}
	return labels
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
