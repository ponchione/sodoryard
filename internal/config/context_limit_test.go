package config

import "testing"

func TestResolveModelContextLimitUsesConfiguredAndFallbackValues(t *testing.T) {
	cfg := &Config{Providers: map[string]ProviderConfig{
		"codex":         {Type: "codex", ContextLength: 111},
		"codex-default": {Type: "codex"},
		"local":         {Type: "openai-compatible"},
	}}
	if got, err := ResolveModelContextLimit(cfg, "codex"); err != nil || got != 111 {
		t.Fatalf("ResolveModelContextLimit(codex) = (%d, %v), want (111, nil)", got, err)
	}
	if got, err := ResolveModelContextLimit(cfg, "codex-default"); err != nil || got != 400000 {
		t.Fatalf("ResolveModelContextLimit(codex-default) = (%d, %v), want (400000, nil)", got, err)
	}
	if got, err := ResolveModelContextLimit(cfg, "local"); err != nil || got != 32768 {
		t.Fatalf("ResolveModelContextLimit(local) = (%d, %v), want (32768, nil)", got, err)
	}
}
