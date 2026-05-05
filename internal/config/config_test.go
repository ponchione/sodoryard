package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) returned error: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory to %q: %v", wd, err)
		}
	})
}

func ensureDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned error: %v", dir, err)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	withWorkingDir(t, projectRoot)
	missing := filepath.Join(projectRoot, "does-not-exist.yaml")

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
	if cfg.Server.AllowExternal {
		t.Fatal("Server.AllowExternal = true, want false")
	}
	if cfg.Routing.Default.Provider != "codex" {
		t.Fatalf("Routing.Default.Provider = %q, want codex", cfg.Routing.Default.Provider)
	}
	if cfg.Routing.Default.Model != "gpt-5.5" {
		t.Fatalf("Routing.Default.Model = %q, want gpt-5.5", cfg.Routing.Default.Model)
	}
	if provider := cfg.Providers["codex"]; provider.Type != "codex" || provider.Model != "gpt-5.5" || provider.ContextLength != 400000 {
		t.Fatalf("Providers[codex] = %#v, want codex/gpt-5.5/400000", provider)
	}
	if cfg.Agent.ShellTimeoutSeconds != 120 {
		t.Fatalf("Agent.ShellTimeoutSeconds = %d, want 120", cfg.Agent.ShellTimeoutSeconds)
	}
	if cfg.Index.AutoReindex {
		t.Fatal("Index.AutoReindex = true, want false")
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
	if !cfg.Brain.LogBrainOperations {
		t.Fatal("Brain.LogBrainOperations = false, want true")
	}
	if cfg.Brain.ReindexOnStartup {
		t.Fatal("Brain.ReindexOnStartup = true, want false")
	}
	if cfg.Brain.LintStaleDays != 90 {
		t.Fatalf("Brain.LintStaleDays = %d, want 90", cfg.Brain.LintStaleDays)
	}
	if cfg.Memory.Backend != "legacy" {
		t.Fatalf("Memory.Backend = %q, want legacy", cfg.Memory.Backend)
	}
	if cfg.Brain.Backend != "vault" {
		t.Fatalf("Brain.Backend = %q, want vault", cfg.Brain.Backend)
	}
}

func TestLoadTracksExplicitProviderNames(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"providers:\n" +
		"  codex:\n" +
		"    type: codex\n" +
		"    model: gpt-5.4-mini\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !slices.Equal(cfg.ConfiguredProviders, []string{"codex"}) {
		t.Fatalf("ConfiguredProviders = %#v, want [codex]", cfg.ConfiguredProviders)
	}
	if _, ok := cfg.Providers["codex"]; !ok {
		t.Fatalf("expected codex provider to remain configured, got %#v", cfg.Providers)
	}
}

func TestLoadPartialYAMLOverridesSpecifiedFields(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"log_level: debug\n" +
		"server:\n" +
		"  port: 9000\n" +
		"  allow_external: true\n" +
		"agent:\n" +
		"  shell_timeout_seconds: 60\n" +
		"  tool_result_store_root: \"" + filepath.Join(projectRoot, ".artifacts", "tool-results") + "\"\n" +
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
	wantToolResultStoreRoot := filepath.Join(projectRoot, ".artifacts", "tool-results")
	if got := cfg.Agent.ToolResultStoreRoot; got != wantToolResultStoreRoot {
		t.Fatalf("Agent.ToolResultStoreRoot = %q, want %q", got, wantToolResultStoreRoot)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want default text", cfg.LogFormat)
	}
	if cfg.Server.Host != defaultServerHost {
		t.Fatalf("Server.Host = %q, want default %q", cfg.Server.Host, defaultServerHost)
	}
	if !cfg.Server.AllowExternal {
		t.Fatal("Server.AllowExternal = false, want true")
	}
	if cfg.Brain.Enabled {
		t.Fatal("Brain.Enabled = true, want false")
	}
	if cfg.Brain.LintStaleDays != 90 {
		t.Fatalf("Brain.LintStaleDays = %d, want default 90", cfg.Brain.LintStaleDays)
	}
}

func TestLoadAllowsConfiguredFallback(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"routing:\n" +
		"  default:\n" +
		"    provider: anthropic\n" +
		"    model: claude-sonnet-4-6-20250514\n" +
		"  fallback:\n" +
		"    provider: openrouter\n" +
		"    model: anthropic/claude-sonnet-4\n" +
		"providers:\n" +
		"  anthropic:\n" +
		"    type: anthropic\n" +
		"  openrouter:\n" +
		"    type: openai-compatible\n" +
		"    base_url: https://openrouter.ai/api/v1\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.Routing.Fallback.Provider; got != "openrouter" {
		t.Fatalf("Routing.Fallback.Provider = %q, want openrouter", got)
	}
	if got := cfg.Routing.Fallback.Model; got != "anthropic/claude-sonnet-4" {
		t.Fatalf("Routing.Fallback.Model = %q, want anthropic/claude-sonnet-4", got)
	}
}

func TestLoadAppendsRequiredIndexExcludesWhenCustomListOmitsThem(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"index:\n" +
		"  include:\n" +
		"    - \"**/*.go\"\n" +
		"  exclude:\n" +
		"    - \"**/.git/**\"\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	wantPatterns := []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/.yard/**"}
	for _, pattern := range wantPatterns {
		if !slices.Contains(cfg.Index.Exclude, pattern) {
			t.Fatalf("Index.Exclude = %#v, want to contain %q", cfg.Index.Exclude, pattern)
		}
	}
}

func TestLoadProvidesEmbeddingDefaults(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	withWorkingDir(t, projectRoot)
	missing := filepath.Join(projectRoot, "does-not-exist.yaml")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Embedding.BaseURL != "http://localhost:12435" {
		t.Fatalf("Embedding.BaseURL = %q, want http://localhost:12435", cfg.Embedding.BaseURL)
	}
	if cfg.LocalServices.Mode != "manual" {
		t.Fatalf("LocalServices.Mode = %q, want manual", cfg.LocalServices.Mode)
	}
	if got := cfg.LocalServices.Services["qwen-coder"].BaseURL; got != "http://localhost:12434" {
		t.Fatalf("LocalServices.Services[qwen-coder].BaseURL = %q, want http://localhost:12434", got)
	}
	if cfg.LocalServices.Services["qwen-coder"].Required {
		t.Fatal("LocalServices.Services[qwen-coder].Required = true, want false")
	}
	if !cfg.LocalServices.Services["nomic-embed"].Required {
		t.Fatal("LocalServices.Services[nomic-embed].Required = false, want true")
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

func TestLoadShunterMemoryResolvesPathsAndDoesNotRequireVault(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"memory:\n" +
		"  backend: shunter\n" +
		"  shunter_data_dir: .yard/shunter/project-memory\n" +
		"  durable_ack: true\n" +
		"  rpc:\n" +
		"    transport: unix\n" +
		"    path: .yard/run/memory.sock\n" +
		"brain:\n" +
		"  enabled: true\n" +
		"  backend: shunter\n" +
		"  vault_path: .brain\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Memory.Backend != "shunter" {
		t.Fatalf("Memory.Backend = %q, want shunter", cfg.Memory.Backend)
	}
	if cfg.Brain.Backend != "shunter" {
		t.Fatalf("Brain.Backend = %q, want shunter", cfg.Brain.Backend)
	}
	if want := filepath.Join(projectRoot, ".yard", "shunter", "project-memory"); cfg.Memory.ShunterDataDir != want {
		t.Fatalf("Memory.ShunterDataDir = %q, want %q", cfg.Memory.ShunterDataDir, want)
	}
	if want := filepath.Join(projectRoot, ".yard", "run", "memory.sock"); cfg.Memory.RPC.Path != want {
		t.Fatalf("Memory.RPC.Path = %q, want %q", cfg.Memory.RPC.Path, want)
	}
	if cfg.Brain.ShunterDataDir != cfg.Memory.ShunterDataDir {
		t.Fatalf("Brain.ShunterDataDir = %q, want %q", cfg.Brain.ShunterDataDir, cfg.Memory.ShunterDataDir)
	}
}

func TestLoadRejectsShunterBrainWithoutShunterMemory(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n" +
		"  enabled: true\n" +
		"  backend: shunter\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "requires memory.backend: shunter") {
		t.Fatalf("Load error = %v, want memory.backend validation", err)
	}
}

func TestLoadAppliesPartialEmbeddingOverrides(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
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
	if cfg.Embedding.BaseURL != "http://localhost:12435" {
		t.Fatalf("Embedding.BaseURL = %q, want default http://localhost:12435", cfg.Embedding.BaseURL)
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
	ensureDir(t, filepath.Join(projectRoot, ".brain"))

	tests := []struct {
		name       string
		yaml       string
		wantSubstr string
	}{
		{
			name:       "empty base url",
			yaml:       "project_root: \"" + projectRoot + "\"\nembedding:\n  base_url: \"\"\n",
			wantSubstr: "embedding.base_url",
		},
		{
			name:       "zero batch size",
			yaml:       "project_root: \"" + projectRoot + "\"\nembedding:\n  batch_size: 0\n",
			wantSubstr: "embedding.batch_size=0",
		},
		{
			name:       "zero timeout",
			yaml:       "project_root: \"" + projectRoot + "\"\nembedding:\n  timeout_seconds: 0\n",
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
			name:       "fallback provider without model",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nrouting:\n  default:\n    provider: codex\n    model: gpt-5.5\n  fallback:\n    provider: openrouter\n",
			wantSubstr: "routing.fallback.model",
		},
		{
			name:       "fallback model without provider",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nrouting:\n  default:\n    provider: codex\n    model: gpt-5.5\n  fallback:\n    model: anthropic/claude-sonnet-4\n",
			wantSubstr: "routing.fallback.provider",
		},
		{
			name:       "fallback provider must be configured",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nrouting:\n  default:\n    provider: codex\n    model: gpt-5.5\n  fallback:\n    provider: missing\n    model: foo\n",
			wantSubstr: "routing.fallback.provider",
		},
		{
			name:       "negative token budget",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\ncontext:\n  max_assembled_tokens: -1\n",
			wantSubstr: "context.max_assembled_tokens=-1",
		},
		{
			name:       "negative history_summarize_after_turns",
			yaml:       "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\nagent:\n  history_summarize_after_turns: -5\n",
			wantSubstr: "agent.history_summarize_after_turns=-5",
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

func TestProjectPathsUseCanonicalYardDirectory(t *testing.T) {
	cfg := &Config{ProjectRoot: filepath.Join(string(filepath.Separator), "tmp", "eyebox")}

	// ProjectName is still derived from the basename because codeintel
	// and brain indexers use it as a label on chunks in the vector store.
	if got := cfg.ProjectName(); got != "eyebox" {
		t.Fatalf("ProjectName() = %q, want eyebox", got)
	}
	// Every other derived path is hardcoded to the canonical .yard name
	// regardless of basename.
	if got := DefaultConfigFilename(); got != "yard.yaml" {
		t.Fatalf("DefaultConfigFilename() = %q, want yard.yaml", got)
	}
	if got := cfg.StateDir(); got != filepath.Join(cfg.ProjectRoot, ".yard") {
		t.Fatalf("StateDir() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard"))
	}
	if got := cfg.DatabasePath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "yard.db") {
		t.Fatalf("DatabasePath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "yard.db"))
	}
	if got := cfg.CodeLanceDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "code") {
		t.Fatalf("CodeLanceDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "code"))
	}
	if got := cfg.BrainLanceDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "brain") {
		t.Fatalf("BrainLanceDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "brain"))
	}
	if got := cfg.GraphDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "graph.db") {
		t.Fatalf("GraphDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "graph.db"))
	}
}

func TestNormalizeAddsDerivedStateDirExcludePattern(t *testing.T) {
	cfg := Default()
	cfg.ProjectRoot = filepath.Join(string(filepath.Separator), "tmp", "sodoryard")
	cfg.Index.Exclude = []string{"**/.git/**"}

	cfg.normalize()

	want := "**/.yard/**"
	if !slices.Contains(cfg.Index.Exclude, want) {
		t.Fatalf("Index.Exclude = %#v, want %q", cfg.Index.Exclude, want)
	}
}

func TestNormalizeKeepsUniversalRequiredExcludes(t *testing.T) {
	cfg := Default()
	cfg.ProjectRoot = filepath.Join(string(filepath.Separator), "tmp", "eyebox")
	cfg.Index.Exclude = nil

	cfg.normalize()

	for _, want := range []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/.yard/**"} {
		if !slices.Contains(cfg.Index.Exclude, want) {
			t.Fatalf("Index.Exclude = %#v, want %q", cfg.Index.Exclude, want)
		}
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

	t.Setenv("SODORYARD_LOG_LEVEL", "error")
	t.Setenv("SIRTOPHAM_LOG_LEVEL", "debug")
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

func TestLoadSupportsLegacyLogLevelEnvironmentVariable(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := "project_root: \"" + projectRoot + "\"\nbrain:\n  vault_path: \"" + projectRoot + "\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	t.Setenv("SIRTOPHAM_LOG_LEVEL", "warn")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want warn", cfg.LogLevel)
	}
}

func TestApplyEnvOverridesDoesNotCreateUnconfiguredAPIKeyProviders(t *testing.T) {
	cfg := Default()
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic")
	t.Setenv("OPENROUTER_API_KEY", "env-openrouter")

	cfg.ApplyEnvOverrides()

	if _, ok := cfg.Providers["anthropic"]; ok {
		t.Fatalf("anthropic provider was created by env override: %#v", cfg.Providers["anthropic"])
	}
	if _, ok := cfg.Providers["openrouter"]; ok {
		t.Fatalf("openrouter provider was created by env override: %#v", cfg.Providers["openrouter"])
	}
}

func TestLoadParsesAgentRolesAndBrainWritePolicies(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n" +
		"  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
		"agent_roles:\n" +
		"  reviewer:\n" +
		"    system_prompt: prompts/reviewer.md\n" +
		"    tools:\n" +
		"      - file\n" +
		"      - git\n" +
		"    custom_tools:\n" +
		"      - external.reviewer\n" +
		"    brain_write_paths:\n" +
		"      - receipts/**\n" +
		"    brain_deny_paths:\n" +
		"      - secrets/**\n" +
		"    max_turns: 3\n" +
		"    max_tokens: 1200\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	role, ok := cfg.AgentRoles["reviewer"]
	if !ok {
		t.Fatalf("AgentRoles = %#v, want reviewer role", cfg.AgentRoles)
	}
	if role.SystemPrompt != "prompts/reviewer.md" {
		t.Fatalf("role.SystemPrompt = %q, want prompts/reviewer.md", role.SystemPrompt)
	}
	if !slices.Equal(role.Tools, []string{"file", "git"}) {
		t.Fatalf("role.Tools = %#v, want [file git]", role.Tools)
	}
	if !slices.Equal(role.CustomTools, []string{"external.reviewer"}) {
		t.Fatalf("role.CustomTools = %#v, want [external.reviewer]", role.CustomTools)
	}
	if !slices.Equal(role.BrainWritePaths, []string{"receipts/**"}) {
		t.Fatalf("role.BrainWritePaths = %#v, want [receipts/**]", role.BrainWritePaths)
	}
	if !slices.Equal(role.BrainDenyPaths, []string{"secrets/**"}) {
		t.Fatalf("role.BrainDenyPaths = %#v, want [secrets/**]", role.BrainDenyPaths)
	}
	if role.MaxTurns != 3 {
		t.Fatalf("role.MaxTurns = %d, want 3", role.MaxTurns)
	}
	if role.MaxTokens != 1200 {
		t.Fatalf("role.MaxTokens = %d, want 1200", role.MaxTokens)
	}
}

func TestLoadAcceptsFileReadAgentRoleToolGroup(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n" +
		"  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
		"agent_roles:\n" +
		"  auditor:\n" +
		"    system_prompt: prompts/auditor.md\n" +
		"    tools:\n" +
		"      - brain\n" +
		"      - file:read\n" +
		"      - git\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	role, ok := cfg.AgentRoles["auditor"]
	if !ok {
		t.Fatalf("AgentRoles = %#v, want auditor role", cfg.AgentRoles)
	}
	if !slices.Equal(role.Tools, []string{"brain", "file:read", "git"}) {
		t.Fatalf("role.Tools = %#v, want [brain file:read git]", role.Tools)
	}
}

func TestLoadAcceptsUtilityAgentRoleToolGroups(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n" +
		"  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
		"agent_roles:\n" +
		"  utility:\n" +
		"    system_prompt: prompts/utility.md\n" +
		"    tools:\n" +
		"      - directory\n" +
		"      - test\n" +
		"      - sqlc\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	role, ok := cfg.AgentRoles["utility"]
	if !ok {
		t.Fatalf("AgentRoles = %#v, want utility role", cfg.AgentRoles)
	}
	if !slices.Equal(role.Tools, []string{"directory", "test", "sqlc"}) {
		t.Fatalf("role.Tools = %#v, want [directory test sqlc]", role.Tools)
	}
}

func TestLoadParsesReadOnlyFileRoleAndCustomTools(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n" +
		"  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
		"agent_roles:\n" +
		"  correctness-auditor:\n" +
		"    system_prompt: agents/correctness-auditor.md\n" +
		"    tools:\n" +
		"      - brain\n" +
		"      - file:read\n" +
		"      - git\n" +
		"    brain_write_paths:\n" +
		"      - receipts/correctness/**\n" +
		"    brain_deny_paths:\n" +
		"      - plans/**\n" +
		"  orchestrator:\n" +
		"    system_prompt: agents/orchestrator.md\n" +
		"    tools:\n" +
		"      - brain\n" +
		"    custom_tools:\n" +
		"      - spawn_agent\n" +
		"      - chain_complete\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	auditorRole, ok := cfg.AgentRoles["correctness-auditor"]
	if !ok {
		t.Fatalf("AgentRoles = %#v, want correctness-auditor role", cfg.AgentRoles)
	}
	if !slices.Equal(auditorRole.Tools, []string{"brain", "file:read", "git"}) {
		t.Fatalf("auditorRole.Tools = %#v, want [brain file:read git]", auditorRole.Tools)
	}
	if !slices.Equal(auditorRole.BrainWritePaths, []string{"receipts/correctness/**"}) {
		t.Fatalf("auditorRole.BrainWritePaths = %#v, want [receipts/correctness/**]", auditorRole.BrainWritePaths)
	}
	if !slices.Equal(auditorRole.BrainDenyPaths, []string{"plans/**"}) {
		t.Fatalf("auditorRole.BrainDenyPaths = %#v, want [plans/**]", auditorRole.BrainDenyPaths)
	}

	orchestratorRole, ok := cfg.AgentRoles["orchestrator"]
	if !ok {
		t.Fatalf("AgentRoles = %#v, want orchestrator role", cfg.AgentRoles)
	}
	if !slices.Equal(orchestratorRole.CustomTools, []string{"spawn_agent", "chain_complete"}) {
		t.Fatalf("orchestratorRole.CustomTools = %#v, want [spawn_agent chain_complete]", orchestratorRole.CustomTools)
	}
}

func TestLoadRejectsInvalidAgentRoles(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))

	tests := []struct {
		name       string
		yaml       string
		wantSubstr string
	}{
		{
			name: "empty system prompt",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: \"\"\n",
			wantSubstr: "agent_roles.reviewer.system_prompt",
		},
		{
			name: "invalid tool group",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: prompts/reviewer.md\n    tools:\n      - browser\n",
			wantSubstr: "unsupported tool group \"browser\"; expected brain, file, file:read, git, shell, search, directory, test, or sqlc",
		},
		{
			name: "negative max turns",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: prompts/reviewer.md\n    max_turns: -1\n",
			wantSubstr: "agent_roles.reviewer.max_turns=-1",
		},
		{
			name: "negative max tokens",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: prompts/reviewer.md\n    max_tokens: -1\n",
			wantSubstr: "agent_roles.reviewer.max_tokens=-1",
		},
		{
			name: "negative timeout",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: prompts/reviewer.md\n    timeout: -1s\n",
			wantSubstr: "agent_roles.reviewer.timeout=-1s",
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

func TestLoadParsesAgentRoleTimeout(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := "project_root: \"" + projectRoot + "\"\n" +
		"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
		"agent_roles:\n" +
		"  coder:\n" +
		"    system_prompt: builtin:coder\n" +
		"    timeout: 45m\n"

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := cfg.AgentRoles["coder"].Timeout.Duration(); got != 45*time.Minute {
		t.Fatalf("AgentRoles[coder].Timeout = %s, want 45m", got)
	}
}

func TestResolveAgentRoleAcceptsConfigKeyAndPersonaAlias(t *testing.T) {
	cfg := &Config{AgentRoles: map[string]AgentRoleConfig{
		"coder":   {SystemPrompt: "builtin:coder"},
		"planner": {SystemPrompt: "builtin:planner"},
	}}

	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "coder", want: "coder"},
		{input: "Thomas", want: "coder"},
		{input: "thomas", want: "coder"},
		{input: "gordon", want: "planner"},
	} {
		got, _, err := cfg.ResolveAgentRole(tc.input)
		if err != nil {
			t.Fatalf("ResolveAgentRole(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ResolveAgentRole(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveAgentRoleUsesExactConfigKeyBeforePersonaAlias(t *testing.T) {
	cfg := &Config{AgentRoles: map[string]AgentRoleConfig{
		"coder":  {SystemPrompt: "builtin:coder"},
		"thomas": {SystemPrompt: "prompts/custom.md"},
	}}

	got, _, err := cfg.ResolveAgentRole("Thomas")
	if err != nil {
		t.Fatalf("ResolveAgentRole returned error: %v", err)
	}
	if got != "thomas" {
		t.Fatalf("ResolveAgentRole(Thomas) = %q, want config key thomas", got)
	}
}

func TestResolveAgentRoleReportsAmbiguousPersonaAlias(t *testing.T) {
	cfg := &Config{AgentRoles: map[string]AgentRoleConfig{
		"coder":    {SystemPrompt: "builtin:coder"},
		"reviewer": {SystemPrompt: "builtin:coder"},
	}}

	_, _, err := cfg.ResolveAgentRole("thomas")
	if err == nil {
		t.Fatal("expected ambiguous role error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "coder, reviewer") {
		t.Fatalf("ResolveAgentRole(thomas) error = %v, want ambiguity with both roles", err)
	}
}

func TestResolveAgentRoleSystemPromptPathUsesProjectRoot(t *testing.T) {
	projectRoot := filepath.Join(string(filepath.Separator), "tmp", "sirtopham-project")
	cfg := &Config{ProjectRoot: projectRoot}

	if got := cfg.ResolveAgentRoleSystemPromptPath("prompts/reviewer.md"); got != filepath.Join(projectRoot, "prompts", "reviewer.md") {
		t.Fatalf("ResolveAgentRoleSystemPromptPath(relative) = %q", got)
	}
	if got := cfg.ResolveAgentRoleSystemPromptPath(filepath.Join(string(filepath.Separator), "abs", "prompt.md")); got != filepath.Join(string(filepath.Separator), "abs", "prompt.md") {
		t.Fatalf("ResolveAgentRoleSystemPromptPath(abs) = %q", got)
	}
}

func TestLoadAllowsBuiltInPromptDefaultsAndSelectors(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))

	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "empty builtin role prompt uses default",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  coder:\n    system_prompt: \"\"\n",
		},
		{
			name: "explicit builtin selector",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: builtin:coder\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("WriteFile returned error: %v", err)
			}

			if _, err := Load(configPath); err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
		})
	}
}

func TestLoadRejectsUnknownBuiltInSelectorsAndEmptyUnknownRolePrompt(t *testing.T) {
	projectRoot := t.TempDir()
	ensureDir(t, filepath.Join(projectRoot, ".brain"))

	tests := []struct {
		name       string
		yaml       string
		wantSubstr string
	}{
		{
			name: "empty unknown role prompt",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: \"\"\n",
			wantSubstr: "agent_roles.reviewer.system_prompt",
		},
		{
			name: "unknown builtin selector",
			yaml: "project_root: \"" + projectRoot + "\"\n" +
				"brain:\n  vault_path: \"" + filepath.Join(projectRoot, ".brain") + "\"\n" +
				"agent_roles:\n  reviewer:\n    system_prompt: builtin:not-a-role\n",
			wantSubstr: "unknown built-in role system prompt",
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
