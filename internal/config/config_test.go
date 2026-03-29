package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want text", cfg.LogFormat)
	}
	if cfg.Server.Port != defaultServerPort {
		t.Fatalf("Server.Port = %d, want %d", cfg.Server.Port, defaultServerPort)
	}
	if cfg.Server.Host != defaultServerHost {
		t.Fatalf("Server.Host = %q, want %q", cfg.Server.Host, defaultServerHost)
	}
	if cfg.Routing.Default.Provider != "anthropic" {
		t.Fatalf("Routing.Default.Provider = %q, want anthropic", cfg.Routing.Default.Provider)
	}
	if cfg.Agent.ShellTimeoutSeconds != 120 {
		t.Fatalf("Agent.ShellTimeoutSeconds = %d, want 120", cfg.Agent.ShellTimeoutSeconds)
	}
	if cfg.Context.CompressionThreshold != 0.50 {
		t.Fatalf("Context.CompressionThreshold = %v, want 0.50", cfg.Context.CompressionThreshold)
	}
	if !cfg.Brain.Enabled {
		t.Fatal("Brain.Enabled = false, want true")
	}
	if cfg.DatabasePath() == "" {
		t.Fatal("DatabasePath() returned empty string")
	}
}

func TestLoadPartialYAMLOverridesSpecifiedFields(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"log_level: debug\n" +
		"server:\n" +
		"  port: 9000\n" +
		"agent:\n" +
		"  shell_timeout_seconds: 60\n" +
		"brain:\n" +
		"  enabled: false\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ProjectRoot != projectRoot {
		t.Fatalf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectRoot)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.Server.Port != 9000 {
		t.Fatalf("Server.Port = %d, want 9000", cfg.Server.Port)
	}
	if cfg.Agent.ShellTimeoutSeconds != 60 {
		t.Fatalf("Agent.ShellTimeoutSeconds = %d, want 60", cfg.Agent.ShellTimeoutSeconds)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want default text", cfg.LogFormat)
	}
	if cfg.Server.Host != defaultServerHost {
		t.Fatalf("Server.Host = %q, want default %q", cfg.Server.Host, defaultServerHost)
	}
	if cfg.Brain.Enabled {
		t.Fatal("Brain.Enabled = true, want false")
	}
}

func TestLoadProvidesEmbeddingDefaults(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Embedding.BaseURL != "http://localhost:8081" {
		t.Fatalf("Embedding.BaseURL = %q, want http://localhost:8081", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.Model != "nomic-embed-code" {
		t.Fatalf("Embedding.Model = %q, want nomic-embed-code", cfg.Embedding.Model)
	}
	if cfg.Embedding.BatchSize != 32 {
		t.Fatalf("Embedding.BatchSize = %d, want 32", cfg.Embedding.BatchSize)
	}
	if cfg.Embedding.TimeoutSeconds != 30 {
		t.Fatalf("Embedding.TimeoutSeconds = %d, want 30", cfg.Embedding.TimeoutSeconds)
	}
	if cfg.Embedding.QueryPrefix != "Represent this query for searching relevant code: " {
		t.Fatalf("Embedding.QueryPrefix = %q, want default prefix", cfg.Embedding.QueryPrefix)
	}
}

func TestLoadAppliesPartialEmbeddingOverrides(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"embedding:\n" +
		"  model: custom-embed\n" +
		"  batch_size: 64\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Embedding.Model != "custom-embed" {
		t.Fatalf("Embedding.Model = %q, want custom-embed", cfg.Embedding.Model)
	}
	if cfg.Embedding.BatchSize != 64 {
		t.Fatalf("Embedding.BatchSize = %d, want 64", cfg.Embedding.BatchSize)
	}
	if cfg.Embedding.BaseURL != "http://localhost:8081" {
		t.Fatalf("Embedding.BaseURL = %q, want default http://localhost:8081", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.TimeoutSeconds != 30 {
		t.Fatalf("Embedding.TimeoutSeconds = %d, want default 30", cfg.Embedding.TimeoutSeconds)
	}
	if cfg.Embedding.QueryPrefix != "Represent this query for searching relevant code: " {
		t.Fatalf("Embedding.QueryPrefix = %q, want default query prefix", cfg.Embedding.QueryPrefix)
	}
}

func TestLoadRejectsInvalidEmbeddingConfig(t *testing.T) {
	projectRoot := t.TempDir()

	tests := []struct {
		name       string
		yaml       string
		wantSubstr string
	}{
		{
			name: "empty base url",
			yaml: "project_root: \"" + projectRoot + "\"\nembedding:\n  base_url: \"\"\n",
			wantSubstr: "embedding.base_url",
		},
		{
			name: "zero batch size",
			yaml: "project_root: \"" + projectRoot + "\"\nembedding:\n  batch_size: 0\n",
			wantSubstr: "embedding.batch_size=0",
		},
		{
			name: "zero timeout",
			yaml: "project_root: \"" + projectRoot + "\"\nembedding:\n  timeout_seconds: 0\n",
			wantSubstr: "embedding.timeout_seconds=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("WriteFile returned error: %v", err)
			}

			_, err := Load(configPath)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	projectRoot := t.TempDir()

	tests := []struct {
		name       string
		yaml       string
		wantSubstr string
	}{
		{
			name:       "bad port",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nserver:\n  port: 70000\n",
			wantSubstr: "server.port=70000",
		},
		{
			name:       "unknown provider type",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nproviders:\n  anthropic:\n    type: mystery\n",
			wantSubstr: "providers.anthropic.type=\"mystery\"",
		},
		{
			name:       "negative token budget",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\ncontext:\n  max_assembled_tokens: -1\n",
			wantSubstr: "context.max_assembled_tokens=-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("WriteFile returned error: %v", err)
			}

			_, err := Load(configPath)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestLoadAppliesEnvironmentVariableOverrides(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"log_level: info\n" +
		"brain:\n" +
		"  vault_path: \"" + projectRoot + "\"\n" +
		"providers:\n" +
		"  anthropic:\n" +
		"    type: anthropic\n" +
		"    api_key: yaml-anthropic\n" +
		"  openrouter:\n" +
		"    type: openai-compatible\n" +
		"    base_url: https://openrouter.ai/api/v1\n" +
		"    api_key: yaml-openrouter\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	t.Setenv("SIRTOPHAM_LOG_LEVEL", "error")
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic")
	t.Setenv("OPENROUTER_API_KEY", "env-openrouter")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.LogLevel != "error" {
		t.Fatalf("LogLevel = %q, want error", cfg.LogLevel)
	}
	if got := cfg.Providers["anthropic"].APIKey; got != "env-anthropic" {
		t.Fatalf("anthropic API key = %q, want env-anthropic", got)
	}
	if got := cfg.Providers["openrouter"].APIKey; got != "env-openrouter" {
		t.Fatalf("openrouter API key = %q, want env-openrouter", got)
	}
}
