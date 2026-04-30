package config

import "fmt"

// ResolveModelContextLimit returns the context window size for a provider,
// either from explicit config or from built-in defaults per provider type.
func ResolveModelContextLimit(cfg *Config, providerName string) (int, error) {
	if cfg == nil {
		return 0, fmt.Errorf("config is required")
	}
	if providerName == "" {
		return 0, fmt.Errorf("provider name is required")
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return 0, fmt.Errorf("unknown provider: %s", providerName)
	}
	if providerCfg.ContextLength > 0 {
		return providerCfg.ContextLength, nil
	}
	switch providerCfg.Type {
	case "anthropic":
		return 200000, nil
	case "codex":
		return 400000, nil
	case "openai-compatible":
		return 32768, nil
	default:
		return 0, fmt.Errorf("provider %s has no positive context_length configured", providerName)
	}
}
