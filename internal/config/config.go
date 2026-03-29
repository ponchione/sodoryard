package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultServerHost = "localhost"
	defaultServerPort = 8090
)

var allowedProviderTypes = map[string]struct{}{
	"anthropic":         {},
	"codex":             {},
	"openai-compatible": {},
}

type Config struct {
	ProjectRoot string `yaml:"project_root"`
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	ServerPort  int    `yaml:"server_port"`
	ServerHost  string `yaml:"server_host"`

	Server    ServerConfig              `yaml:"server"`
	Routing   RoutingConfig             `yaml:"routing"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Index     IndexConfig               `yaml:"index"`
	Embedding Embedding                 `yaml:"embedding"`
	Agent     AgentConfig               `yaml:"agent"`
	Context   ContextConfig             `yaml:"context"`
	Brain     BrainConfig               `yaml:"brain"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	DevMode     bool   `yaml:"dev_mode"`
	OpenBrowser bool   `yaml:"open_browser"`
}

type RoutingConfig struct {
	Default  RouteConfig `yaml:"default"`
	Fallback RouteConfig `yaml:"fallback"`
}

type RouteConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

type ProviderConfig struct {
	Type          string `yaml:"type"`
	BaseURL       string `yaml:"base_url"`
	Model         string `yaml:"model"`
	APIKey        string `yaml:"api_key"`
	APIKeyEnv     string `yaml:"api_key_env"`
	ContextLength int    `yaml:"context_length"`
}

type IndexConfig struct {
	Include               []string `yaml:"include"`
	Exclude               []string `yaml:"exclude"`
	MaxRAGResults         int      `yaml:"max_rag_results"`
	MaxTreeLines          int      `yaml:"max_tree_lines"`
	AutoReindex           bool     `yaml:"auto_reindex"`
	MaxFileSizeBytes      int      `yaml:"max_file_size_bytes"`
	MaxTotalFileSizeBytes int      `yaml:"max_total_file_size_bytes"`
}

// Embedding configures the local embedding service used for semantic search.
type Embedding struct {
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	BatchSize      int    `yaml:"batch_size"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	QueryPrefix    string `yaml:"query_prefix"`
}

type AgentConfig struct {
	MaxIterationsPerTurn     int      `yaml:"max_iterations_per_turn"`
	LoopDetectionThreshold   int      `yaml:"loop_detection_threshold"`
	ToolOutputMaxTokens      int      `yaml:"tool_output_max_tokens"`
	ShellTimeoutSeconds      int      `yaml:"shell_timeout_seconds"`
	ShellDenylist            []string `yaml:"shell_denylist"`
	ExtendedThinking         bool     `yaml:"extended_thinking"`
	CacheSystemPrompt        bool     `yaml:"cache_system_prompt"`
	CacheAssembledContext    bool     `yaml:"cache_assembled_context"`
	CacheConversationHistory bool     `yaml:"cache_conversation_history"`
}

type ContextConfig struct {
	MaxAssembledTokens      int     `yaml:"max_assembled_tokens"`
	MaxChunks               int     `yaml:"max_chunks"`
	MaxExplicitFiles        int     `yaml:"max_explicit_files"`
	ConventionBudgetTokens  int     `yaml:"convention_budget_tokens"`
	GitContextBudgetTokens  int     `yaml:"git_context_budget_tokens"`
	RelevanceThreshold      float64 `yaml:"relevance_threshold"`
	StructuralHopDepth      int     `yaml:"structural_hop_depth"`
	StructuralHopBudget     int     `yaml:"structural_hop_budget"`
	MomentumLookbackTurns   int     `yaml:"momentum_lookback_turns"`
	CompressionThreshold    float64 `yaml:"compression_threshold"`
	CompressionHeadPreserve int     `yaml:"compression_head_preserve"`
	CompressionTailPreserve int     `yaml:"compression_tail_preserve"`
	CompressionModel        string  `yaml:"compression_model"`
	EmitContextDebug        bool    `yaml:"emit_context_debug"`
	StoreAssemblyReports    bool    `yaml:"store_assembly_reports"`
}

type BrainConfig struct {
	Enabled                 bool    `yaml:"enabled"`
	VaultPath               string  `yaml:"vault_path"`
	ObsidianAPIURL          string  `yaml:"obsidian_api_url"`
	ObsidianAPIKey          string  `yaml:"obsidian_api_key"`
	EmbeddingModel          string  `yaml:"embedding_model"`
	ChunkAtHeadings         bool    `yaml:"chunk_at_headings"`
	ReindexOnStartup        bool    `yaml:"reindex_on_startup"`
	MaxBrainTokens          int     `yaml:"max_brain_tokens"`
	BrainRelevanceThreshold float64 `yaml:"brain_relevance_threshold"`
	IncludeGraphHops        bool    `yaml:"include_graph_hops"`
	GraphHopDepth           int     `yaml:"graph_hop_depth"`
	LogBrainQueries         bool    `yaml:"log_brain_queries"`
}

func Default() *Config {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		wd = "."
	}

	cfg := &Config{
		ProjectRoot: wd,
		LogLevel:    "info",
		LogFormat:   "text",
		ServerPort:  defaultServerPort,
		ServerHost:  defaultServerHost,
		Server: ServerConfig{
			Host:        defaultServerHost,
			Port:        defaultServerPort,
			DevMode:     false,
			OpenBrowser: true,
		},
		Routing: RoutingConfig{
			Default: RouteConfig{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-6",
			},
			Fallback: RouteConfig{
				Provider: "local",
				Model:    "qwen2.5-coder-7b",
			},
		},
		Providers: map[string]ProviderConfig{
			"anthropic": {
				Type:          "anthropic",
				Model:         "claude-sonnet-4-6",
				APIKeyEnv:     "ANTHROPIC_API_KEY",
				ContextLength: 200000,
			},
			"local": {
				Type:          "openai-compatible",
				BaseURL:       "http://localhost:8080/v1",
				Model:         "qwen2.5-coder-7b",
				ContextLength: 32768,
			},
			"openrouter": {
				Type:          "openai-compatible",
				BaseURL:       "https://openrouter.ai/api/v1",
				Model:         "anthropic/claude-sonnet-4",
				APIKeyEnv:     "OPENROUTER_API_KEY",
				ContextLength: 200000,
			},
		},
		Index: IndexConfig{
			Include:               []string{"**/*.go", "**/*.sql", "**/*.md"},
			Exclude:               []string{"**/.git/**", "**/vendor/**", "**/node_modules/**"},
			MaxRAGResults:         30,
			MaxTreeLines:          200,
			AutoReindex:           true,
			MaxFileSizeBytes:      51200,
			MaxTotalFileSizeBytes: 524288,
		},
		Embedding: Embedding{
			BaseURL:        "http://localhost:8081",
			Model:          "nomic-embed-code",
			BatchSize:      32,
			TimeoutSeconds: 30,
			QueryPrefix:    "Represent this query for searching relevant code: ",
		},
		Agent: AgentConfig{
			MaxIterationsPerTurn:     50,
			LoopDetectionThreshold:   3,
			ToolOutputMaxTokens:      50000,
			ShellTimeoutSeconds:      120,
			ShellDenylist:            []string{"rm -rf /", "git push --force"},
			ExtendedThinking:         true,
			CacheSystemPrompt:        true,
			CacheAssembledContext:    true,
			CacheConversationHistory: true,
		},
		Context: ContextConfig{
			MaxAssembledTokens:      30000,
			MaxChunks:               25,
			MaxExplicitFiles:        5,
			ConventionBudgetTokens:  3000,
			GitContextBudgetTokens:  2000,
			RelevanceThreshold:      0.35,
			StructuralHopDepth:      1,
			StructuralHopBudget:     10,
			MomentumLookbackTurns:   2,
			CompressionThreshold:    0.50,
			CompressionHeadPreserve: 3,
			CompressionTailPreserve: 4,
			CompressionModel:        "local",
			EmitContextDebug:        true,
			StoreAssemblyReports:    true,
		},
		Brain: BrainConfig{
			Enabled:                 true,
			VaultPath:               wd,
			ObsidianAPIURL:          "http://localhost:27124",
			ObsidianAPIKey:          "",
			EmbeddingModel:          "nomic-embed-code",
			ChunkAtHeadings:         true,
			ReindexOnStartup:        true,
			MaxBrainTokens:          8000,
			BrainRelevanceThreshold: 0.30,
			IncludeGraphHops:        true,
			GraphHopDepth:           1,
			LogBrainQueries:         true,
		},
	}

	cfg.normalize()
	return cfg
}

func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	cfg.normalize()
	cfg.ApplyEnvOverrides()
	cfg.normalize()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) ApplyEnvOverrides() {
	if c == nil {
		return
	}

	if value, ok := os.LookupEnv("SIRTOPHAM_LOG_LEVEL"); ok {
		c.LogLevel = value
	}

	if value, ok := os.LookupEnv("ANTHROPIC_API_KEY"); ok {
		provider := c.Providers["anthropic"]
		provider.APIKey = value
		provider.APIKeyEnv = "ANTHROPIC_API_KEY"
		c.Providers["anthropic"] = provider
	}

	if value, ok := os.LookupEnv("OPENROUTER_API_KEY"); ok {
		provider := c.Providers["openrouter"]
		provider.APIKey = value
		provider.APIKeyEnv = "OPENROUTER_API_KEY"
		c.Providers["openrouter"] = provider
	}
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}

	if err := c.validateLogLevel(); err != nil {
		return err
	}
	if err := c.validateLogFormat(); err != nil {
		return err
	}
	if err := c.validatePort(); err != nil {
		return err
	}
	if err := c.validatePaths(); err != nil {
		return err
	}
	if err := c.validateRouting(); err != nil {
		return err
	}
	if err := c.validateProviders(); err != nil {
		return err
	}
	if err := c.validateEmbedding(); err != nil {
		return err
	}
	if err := c.validateNumericFields(); err != nil {
		return err
	}

	return nil
}

func (c *Config) DatabasePath() string {
	root := c.ProjectRoot
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ".sirtopham", "sirtopham.db")
}

func (c *Config) ServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) normalize() {
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}

	if c.Server.Host == "" {
		c.Server.Host = c.ServerHost
	}
	if c.Server.Port == 0 {
		c.Server.Port = c.ServerPort
	}
	if c.Server.Host == "" {
		c.Server.Host = defaultServerHost
	}
	if c.Server.Port == 0 {
		c.Server.Port = defaultServerPort
	}

	c.ServerHost = c.Server.Host
	c.ServerPort = c.Server.Port
}

func (c *Config) validateLogLevel() error {
	switch strings.ToLower(strings.TrimSpace(c.LogLevel)) {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid field log_level=%q (expected debug, info, warn, or error)", c.LogLevel)
	}
}

func (c *Config) validateLogFormat() error {
	switch strings.ToLower(strings.TrimSpace(c.LogFormat)) {
	case "json", "text":
		return nil
	default:
		return fmt.Errorf("invalid field log_format=%q (expected json or text)", c.LogFormat)
	}
}

func (c *Config) validatePort() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid field server.port=%d (must be between 1 and 65535)", c.Server.Port)
	}
	return nil
}

func (c *Config) validatePaths() error {
	projectRoot, err := expandPath(c.ProjectRoot)
	if err != nil {
		return fmt.Errorf("invalid field project_root=%q: %w", c.ProjectRoot, err)
	}
	info, err := os.Stat(projectRoot)
	if err != nil {
		return fmt.Errorf("invalid field project_root=%q: %w", c.ProjectRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid field project_root=%q (must be an existing directory)", c.ProjectRoot)
	}
	c.ProjectRoot = projectRoot

	if !c.Brain.Enabled {
		return nil
	}

	vaultPath, err := expandPath(c.Brain.VaultPath)
	if err != nil {
		return fmt.Errorf("invalid field brain.vault_path=%q: %w", c.Brain.VaultPath, err)
	}
	info, err = os.Stat(vaultPath)
	if err != nil {
		return fmt.Errorf("invalid field brain.vault_path=%q: %w", c.Brain.VaultPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid field brain.vault_path=%q (must be an existing directory)", c.Brain.VaultPath)
	}
	c.Brain.VaultPath = vaultPath

	return nil
}

func (c *Config) validateRouting() error {
	if c.Routing.Default.Provider == "" {
		return errors.New("invalid field routing.default.provider=\"\" (must name a configured provider)")
	}
	if _, ok := c.Providers[c.Routing.Default.Provider]; !ok {
		return fmt.Errorf("invalid field routing.default.provider=%q (provider not configured)", c.Routing.Default.Provider)
	}
	if c.Routing.Fallback.Provider == "" {
		return errors.New("invalid field routing.fallback.provider=\"\" (must name a configured provider)")
	}
	if _, ok := c.Providers[c.Routing.Fallback.Provider]; !ok {
		return fmt.Errorf("invalid field routing.fallback.provider=%q (provider not configured)", c.Routing.Fallback.Provider)
	}
	return nil
}

func (c *Config) validateProviders() error {
	for name, provider := range c.Providers {
		providerType := strings.ToLower(strings.TrimSpace(provider.Type))
		if _, ok := allowedProviderTypes[providerType]; !ok {
			return fmt.Errorf("invalid field providers.%s.type=%q (expected anthropic, codex, or openai-compatible)", name, provider.Type)
		}
	}
	return nil
}

func (c *Config) validateEmbedding() error {
	if strings.TrimSpace(c.Embedding.BaseURL) == "" {
		return errors.New("invalid field embedding.base_url=\"\" (must not be empty)")
	}
	if c.Embedding.BatchSize <= 0 {
		return fmt.Errorf("invalid field embedding.batch_size=%d (must be > 0)", c.Embedding.BatchSize)
	}
	if c.Embedding.TimeoutSeconds <= 0 {
		return fmt.Errorf("invalid field embedding.timeout_seconds=%d (must be > 0)", c.Embedding.TimeoutSeconds)
	}
	return nil
}

func (c *Config) validateNumericFields() error {
	for field, value := range map[string]int{
		"index.max_rag_results":             c.Index.MaxRAGResults,
		"index.max_tree_lines":              c.Index.MaxTreeLines,
		"index.max_file_size_bytes":         c.Index.MaxFileSizeBytes,
		"index.max_total_file_size_bytes":   c.Index.MaxTotalFileSizeBytes,
		"agent.max_iterations_per_turn":     c.Agent.MaxIterationsPerTurn,
		"agent.loop_detection_threshold":    c.Agent.LoopDetectionThreshold,
		"agent.tool_output_max_tokens":      c.Agent.ToolOutputMaxTokens,
		"agent.shell_timeout_seconds":       c.Agent.ShellTimeoutSeconds,
		"context.max_assembled_tokens":      c.Context.MaxAssembledTokens,
		"context.max_chunks":                c.Context.MaxChunks,
		"context.max_explicit_files":        c.Context.MaxExplicitFiles,
		"context.convention_budget_tokens":  c.Context.ConventionBudgetTokens,
		"context.git_context_budget_tokens": c.Context.GitContextBudgetTokens,
		"context.structural_hop_depth":      c.Context.StructuralHopDepth,
		"context.structural_hop_budget":     c.Context.StructuralHopBudget,
		"context.momentum_lookback_turns":   c.Context.MomentumLookbackTurns,
		"context.compression_head_preserve": c.Context.CompressionHeadPreserve,
		"context.compression_tail_preserve": c.Context.CompressionTailPreserve,
		"brain.max_brain_tokens":            c.Brain.MaxBrainTokens,
		"brain.graph_hop_depth":             c.Brain.GraphHopDepth,
	} {
		if value < 0 {
			return fmt.Errorf("invalid field %s=%d (must be >= 0)", field, value)
		}
	}

	for field, value := range map[string]float64{
		"context.relevance_threshold":     c.Context.RelevanceThreshold,
		"context.compression_threshold":   c.Context.CompressionThreshold,
		"brain.brain_relevance_threshold": c.Brain.BrainRelevanceThreshold,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("invalid field %s=%v (must be between 0 and 1)", field, value)
		}
	}

	return nil
}

func expandPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}

	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if trimmed == "~" {
			trimmed = home
		} else {
			trimmed = filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return abs, nil
}
