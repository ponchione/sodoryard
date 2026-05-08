package modelcap

import (
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func TestResolveConfiguredModelUsesProviderDefaults(t *testing.T) {
	cfg := &appconfig.Config{
		Routing: appconfig.RoutingConfig{Default: appconfig.RouteConfig{Provider: "codex", Model: "gpt-5.5"}},
		Providers: map[string]appconfig.ProviderConfig{
			"codex": {Type: "codex", Model: "gpt-5.5"},
		},
	}

	model, err := ResolveConfiguredModel(cfg, "", "")
	if err != nil {
		t.Fatalf("ResolveConfiguredModel returned error: %v", err)
	}
	if model.ID != "gpt-5.5" || model.Provider != "codex" || model.ContextWindow != 400000 {
		t.Fatalf("model identity = %+v, want codex gpt-5.5 with 400000 context", model)
	}
	if !model.SupportsTools || !model.SupportsReasoningEffort || model.SupportsThinking {
		t.Fatalf("model capabilities = %+v, want tools+reasoning_effort and no thinking", model)
	}
	if len(model.KnownQuirks) != 1 {
		t.Fatalf("known quirks = %v, want codex quirk", model.KnownQuirks)
	}
}

func TestResolveConfiguredModelAppliesProviderOverrides(t *testing.T) {
	falseValue := false
	trueValue := true
	cfg := &appconfig.Config{
		Routing: appconfig.RoutingConfig{Default: appconfig.RouteConfig{Provider: "local", Model: "local-model"}},
		Providers: map[string]appconfig.ProviderConfig{
			"local": {
				Type:                     "openai-compatible",
				Model:                    "local-model",
				ContextLength:            8192,
				SupportsTools:            &falseValue,
				SupportsThinking:         &trueValue,
				SupportsStructuredOutput: &trueValue,
				MaxOutputTokens:          2048,
				KnownQuirks:              []string{"tool calls disabled for this endpoint"},
			},
		},
	}

	model, err := ResolveConfiguredModel(cfg, "local", "")
	if err != nil {
		t.Fatalf("ResolveConfiguredModel returned error: %v", err)
	}
	if model.ContextWindow != 8192 || model.MaxOutputTokens != 2048 {
		t.Fatalf("limits = context:%d output:%d, want 8192/2048", model.ContextWindow, model.MaxOutputTokens)
	}
	if model.SupportsTools || !model.SupportsThinking || !model.SupportsStructuredOutput {
		t.Fatalf("capabilities = %+v, want overrides applied", model)
	}
	if len(model.KnownQuirks) != 2 {
		t.Fatalf("known quirks = %v, want default plus configured quirk", model.KnownQuirks)
	}
}

func TestCapabilityLabels(t *testing.T) {
	cfg := &appconfig.Config{
		Routing: appconfig.RoutingConfig{Default: appconfig.RouteConfig{Provider: "anthropic", Model: "claude-sonnet-4-6-20250514"}},
		Providers: map[string]appconfig.ProviderConfig{
			"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-6-20250514"},
		},
	}
	model, err := ResolveConfiguredModel(cfg, "anthropic", "")
	if err != nil {
		t.Fatalf("ResolveConfiguredModel returned error: %v", err)
	}
	labels := CapabilityLabels(model)
	want := []string{"tools", "thinking", "prompt_cache"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels[%d] = %q, want %q (all=%v)", i, labels[i], want[i], labels)
		}
	}
}
