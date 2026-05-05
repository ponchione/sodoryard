package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/embeddedprompts"
	"github.com/ponchione/sodoryard/internal/toolgroup"
	"gopkg.in/yaml.v3"
)

const (
	defaultServerHost           = "localhost"
	defaultServerPort           = 8090
	defaultQwenCoderBaseURL     = "http://localhost:12434"
	defaultNomicEmbedBaseURL    = "http://localhost:12435"
	localServicesModeOff        = "off"
	localServicesModeManual     = "manual"
	localServicesModeAuto       = "auto"
	localServicesProviderDocker = "docker-compose"
	defaultLocalServicesCompose = "./ops/llm/docker-compose.yml"
	defaultLocalServicesProject = "./ops/llm"
	defaultLocalServicesNetwork = "llm-net"
	defaultLocalStartupTimeout  = 180
	defaultLocalHealthInterval  = 2
	memoryBackendShunter        = "shunter"
	brainBackendShunter         = "shunter"
	memoryRPCTransportUnix      = "unix"
)

// Canonical on-disk names for the yard state directory and its contents.
// These are hardcoded (not derived from project basename) so that every
// project writes yard state to the same relative path, simplifying tooling,
// dashboards, and docker-compose mounts.
const (
	StateDirName   = ".yard"
	StateDBName    = "yard.db"
	ConfigFilename = "yard.yaml"
)

var allowedProviderTypes = []string{"anthropic", "codex", "openai-compatible"}

type Config struct {
	ProjectRoot string `yaml:"project_root"`
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	ServerPort  int    `yaml:"server_port"`
	ServerHost  string `yaml:"server_host"`

	ConfiguredProviders []string `yaml:"-"`

	Server        ServerConfig               `yaml:"server"`
	Routing       RoutingConfig              `yaml:"routing"`
	Providers     map[string]ProviderConfig  `yaml:"providers"`
	Index         IndexConfig                `yaml:"index"`
	Embedding     Embedding                  `yaml:"embedding"`
	Agent         AgentConfig                `yaml:"agent"`
	AgentRoles    map[string]AgentRoleConfig `yaml:"agent_roles"`
	Context       ContextConfig              `yaml:"context"`
	Memory        MemoryConfig               `yaml:"memory"`
	Brain         BrainConfig                `yaml:"brain"`
	LocalServices LocalServicesConfig        `yaml:"local_services"`
}

type AgentRoleConfig struct {
	SystemPrompt    string   `yaml:"system_prompt"`
	Tools           []string `yaml:"tools"`
	CustomTools     []string `yaml:"custom_tools"`
	BrainWritePaths []string `yaml:"brain_write_paths"`
	BrainDenyPaths  []string `yaml:"brain_deny_paths"`
	MaxTurns        int      `yaml:"max_turns"`
	MaxTokens       int      `yaml:"max_tokens"`
	Timeout         Duration `yaml:"timeout"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || strings.TrimSpace(value.Value) == "" {
		*d = 0
		return nil
	}
	if value.Kind == yaml.ScalarNode {
		if parsed, err := time.ParseDuration(value.Value); err == nil {
			*d = Duration(parsed)
			return nil
		}
	}
	var raw int64
	if err := value.Decode(&raw); err == nil {
		*d = Duration(time.Duration(raw))
		return nil
	}
	return fmt.Errorf("invalid duration %q", value.Value)
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

type ServerConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	DevMode       bool   `yaml:"dev_mode"`
	OpenBrowser   bool   `yaml:"open_browser"`
	AllowExternal bool   `yaml:"allow_external"`
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

var requiredIndexExcludePatterns = []string{"**/.git/**", "**/node_modules/**", "**/vendor/**"}

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
	ToolResultStoreRoot      string   `yaml:"tool_result_store_root"`
	ShellTimeoutSeconds      int      `yaml:"shell_timeout_seconds"`
	ShellDenylist            []string `yaml:"shell_denylist"`
	ExtendedThinking         bool     `yaml:"extended_thinking"`
	CacheSystemPrompt        bool     `yaml:"cache_system_prompt"`
	CacheAssembledContext    bool     `yaml:"cache_assembled_context"`
	CacheConversationHistory bool     `yaml:"cache_conversation_history"`

	// Phase 2: History compression (spec 11).
	CompressHistoricalResults  bool `yaml:"compress_historical_results"`
	StripHistoricalLineNumbers bool `yaml:"strip_historical_line_numbers"`
	ElideDuplicateReads        bool `yaml:"elide_duplicate_reads"`
	HistorySummarizeAfterTurns int  `yaml:"history_summarize_after_turns"`
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
	Enabled                 bool     `yaml:"enabled"`
	Backend                 string   `yaml:"backend"`
	EmbeddingModel          string   `yaml:"embedding_model"`
	ChunkAtHeadings         bool     `yaml:"chunk_at_headings"`
	ReindexOnStartup        bool     `yaml:"reindex_on_startup"`
	MaxBrainTokens          int      `yaml:"max_brain_tokens"`
	BrainRelevanceThreshold float64  `yaml:"brain_relevance_threshold"`
	IncludeGraphHops        bool     `yaml:"include_graph_hops"`
	GraphHopDepth           int      `yaml:"graph_hop_depth"`
	LogBrainQueries         bool     `yaml:"log_brain_queries"`
	LogBrainOperations      bool     `yaml:"log_brain_operations"`
	LintStaleDays           int      `yaml:"lint_stale_days"`
	LintOrphanAllowlist     []string `yaml:"lint_orphan_allowlist"`
	BrainWritePaths         []string `yaml:"brain_write_paths"`
	BrainDenyPaths          []string `yaml:"brain_deny_paths"`

	MemoryBackend  string `yaml:"-"`
	ShunterDataDir string `yaml:"-"`
	DurableAck     bool   `yaml:"-"`
	RPCTransport   string `yaml:"-"`
	RPCPath        string `yaml:"-"`
}

type MemoryConfig struct {
	Backend        string          `yaml:"backend"`
	ShunterDataDir string          `yaml:"shunter_data_dir"`
	DurableAck     bool            `yaml:"durable_ack"`
	RPC            MemoryRPCConfig `yaml:"rpc"`
}

type MemoryRPCConfig struct {
	Transport string `yaml:"transport"`
	Path      string `yaml:"path"`
}

type LocalServicesConfig struct {
	Enabled                    bool                      `yaml:"enabled"`
	Mode                       string                    `yaml:"mode"`
	Provider                   string                    `yaml:"provider"`
	ComposeFile                string                    `yaml:"compose_file"`
	ProjectDir                 string                    `yaml:"project_dir"`
	RequiredNetworks           []string                  `yaml:"required_networks"`
	AutoCreateNetworks         bool                      `yaml:"auto_create_networks"`
	StartupTimeoutSeconds      int                       `yaml:"startup_timeout_seconds"`
	HealthcheckIntervalSeconds int                       `yaml:"healthcheck_interval_seconds"`
	Services                   map[string]ManagedService `yaml:"services"`
}

type ManagedService struct {
	BaseURL    string `yaml:"base_url"`
	HealthPath string `yaml:"health_path"`
	ModelsPath string `yaml:"models_path"`
	Required   bool   `yaml:"required"`
}

func defaultManagedServices() map[string]ManagedService {
	return map[string]ManagedService{
		"qwen-coder": {
			BaseURL:    defaultQwenCoderBaseURL,
			HealthPath: "/health",
			ModelsPath: "/v1/models",
			Required:   false,
		},
		"nomic-embed": {
			BaseURL:    defaultNomicEmbedBaseURL,
			HealthPath: "/health",
			ModelsPath: "/v1/models",
			Required:   true,
		},
	}
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
			Host:          defaultServerHost,
			Port:          defaultServerPort,
			DevMode:       false,
			OpenBrowser:   true,
			AllowExternal: false,
		},
		Routing: RoutingConfig{
			Default: RouteConfig{
				Provider: "codex",
				Model:    "gpt-5.5",
			},
		},
		Providers: map[string]ProviderConfig{
			"codex": {
				Type:          "codex",
				Model:         "gpt-5.5",
				ContextLength: 400000,
			},
		},
		Index: IndexConfig{
			Include:               []string{"**/*.go", "**/*.sql", "**/*.md"},
			Exclude:               []string{"**/.git/**", "**/vendor/**", "**/node_modules/**"},
			MaxRAGResults:         30,
			MaxTreeLines:          200,
			AutoReindex:           false,
			MaxFileSizeBytes:      51200,
			MaxTotalFileSizeBytes: 0,
		},
		Embedding: Embedding{
			BaseURL:        defaultNomicEmbedBaseURL,
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

			CompressHistoricalResults:  true,
			StripHistoricalLineNumbers: true,
			ElideDuplicateReads:        true,
			HistorySummarizeAfterTurns: 10,
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
		Memory: MemoryConfig{
			Backend:        memoryBackendShunter,
			ShunterDataDir: ".yard/shunter/project-memory",
			DurableAck:     true,
			RPC: MemoryRPCConfig{
				Transport: memoryRPCTransportUnix,
				Path:      ".yard/run/memory.sock",
			},
		},
		Brain: BrainConfig{
			Enabled:                 true,
			Backend:                 brainBackendShunter,
			EmbeddingModel:          "nomic-embed-code",
			ChunkAtHeadings:         true,
			ReindexOnStartup:        false,
			MaxBrainTokens:          8000,
			BrainRelevanceThreshold: 0.30,
			IncludeGraphHops:        true,
			GraphHopDepth:           1,
			LogBrainQueries:         true,
			LogBrainOperations:      true,
			LintStaleDays:           90,
			LintOrphanAllowlist:     nil,
		},
		LocalServices: LocalServicesConfig{
			Enabled:                    true,
			Mode:                       localServicesModeManual,
			Provider:                   localServicesProviderDocker,
			ComposeFile:                defaultLocalServicesCompose,
			ProjectDir:                 defaultLocalServicesProject,
			RequiredNetworks:           []string{defaultLocalServicesNetwork},
			AutoCreateNetworks:         true,
			StartupTimeoutSeconds:      defaultLocalStartupTimeout,
			HealthcheckIntervalSeconds: defaultLocalHealthInterval,
			Services:                   defaultManagedServices(),
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
		cfg.ConfiguredProviders = configuredProviderNamesFromYAML(data)
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

func configuredProviderNamesFromYAML(data []byte) []string {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var raw struct {
		Providers map[string]ProviderConfig `yaml:"providers"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil || len(raw.Providers) == 0 {
		return nil
	}
	names := make([]string, 0, len(raw.Providers))
	for name := range raw.Providers {
		names = append(names, name)
	}
	return names
}

func (c *Config) ProviderNamesForSurfaces() []string {
	if c == nil {
		return nil
	}
	if len(c.ConfiguredProviders) > 0 {
		return append([]string(nil), c.ConfiguredProviders...)
	}
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	return names
}

func (c *Config) ApplyEnvOverrides() {
	if c == nil {
		return
	}

	if value, ok := os.LookupEnv("SODORYARD_LOG_LEVEL"); ok {
		c.LogLevel = value
	}

	if value, ok := os.LookupEnv("ANTHROPIC_API_KEY"); ok {
		if provider, configured := c.Providers["anthropic"]; configured {
			provider.APIKey = value
			provider.APIKeyEnv = "ANTHROPIC_API_KEY"
			c.Providers["anthropic"] = provider
		}
	}

	if value, ok := os.LookupEnv("OPENROUTER_API_KEY"); ok {
		if provider, configured := c.Providers["openrouter"]; configured {
			provider.APIKey = value
			provider.APIKeyEnv = "OPENROUTER_API_KEY"
			c.Providers["openrouter"] = provider
		}
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
	if err := c.validateLocalServices(); err != nil {
		return err
	}
	if err := c.validateNumericFields(); err != nil {
		return err
	}
	if err := c.validateAgentRoles(); err != nil {
		return err
	}

	return nil
}

func DefaultProjectName(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		if wd, err := os.Getwd(); err == nil && wd != "" {
			root = wd
		} else {
			return "sodoryard"
		}
	}
	base := filepath.Base(filepath.Clean(root))
	base = strings.TrimSpace(base)
	base = strings.TrimPrefix(base, ".")
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "sodoryard"
	}
	return base
}

// DefaultConfigFilename returns the canonical yard config filename.
func DefaultConfigFilename() string {
	return ConfigFilename
}

func (c *Config) ProjectName() string {
	return DefaultProjectName(c.ProjectRoot)
}

// StateDir returns the absolute path to the yard state directory for this
// project. The name is hardcoded (.yard) and does NOT depend on the project
// directory basename. See ProjectName() for the basename-derived codeintel
// label, which is a separate concept.
func (c *Config) StateDir() string {
	root := c.ProjectRoot
	if root == "" {
		root = "."
	}
	return filepath.Join(root, StateDirName)
}

func (c *Config) DatabasePath() string {
	return filepath.Join(c.StateDir(), StateDBName)
}

// CodeLanceDBPath returns the directory for the code vectorstore.
func (c *Config) CodeLanceDBPath() string {
	return filepath.Join(c.StateDir(), "lancedb", "code")
}

// BrainLanceDBPath returns the directory for the brain vectorstore.
func (c *Config) BrainLanceDBPath() string {
	return filepath.Join(c.StateDir(), "lancedb", "brain")
}

// GraphDBPath returns the SQLite database path for the structural graph index.
func (c *Config) GraphDBPath() string {
	return filepath.Join(c.StateDir(), "graph.db")
}

// MemoryShunterDataDir returns the resolved Shunter project-memory directory.
func (c *Config) MemoryShunterDataDir() string {
	path := c.Memory.ShunterDataDir
	if path == "" {
		path = ".yard/shunter/project-memory"
	}
	return projectRelativePath(c.ProjectRoot, path)
}

// MemoryRPCPath returns the resolved local memory RPC socket path.
func (c *Config) MemoryRPCPath() string {
	path := c.Memory.RPC.Path
	if path == "" {
		path = ".yard/run/memory.sock"
	}
	return projectRelativePath(c.ProjectRoot, path)
}

func (c *Config) ResolveAgentRoleSystemPromptPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	root := c.ProjectRoot
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, trimmed)
}

func (c *Config) ServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) LocalService(name string) ManagedService {
	if c == nil {
		return ManagedService{}
	}
	return c.LocalServices.Services[name]
}

func (c *Config) QwenCoderBaseURL() string {
	return strings.TrimRight(c.LocalService("qwen-coder").BaseURL, "/")
}

func (c *Config) requiredIndexExcludePatterns() []string {
	patterns := append([]string(nil), requiredIndexExcludePatterns...)
	patterns = append(patterns, "**/"+StateDirName+"/**")
	return patterns
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

	c.Index.Exclude = appendMissingStrings(c.Index.Exclude, c.requiredIndexExcludePatterns()...)
	c.normalizeMemory()
	c.normalizeLocalServices()

	c.ServerHost = c.Server.Host
	c.ServerPort = c.Server.Port
}

func (c *Config) normalizeMemory() {
	if c.Memory.Backend == "" {
		c.Memory.Backend = memoryBackendShunter
	}
	if c.Memory.ShunterDataDir == "" {
		c.Memory.ShunterDataDir = ".yard/shunter/project-memory"
	}
	if c.Memory.RPC.Transport == "" {
		c.Memory.RPC.Transport = memoryRPCTransportUnix
	}
	if c.Memory.RPC.Path == "" {
		c.Memory.RPC.Path = ".yard/run/memory.sock"
	}
	if c.Brain.Backend == "" {
		c.Brain.Backend = brainBackendShunter
	}
}

func (c *Config) normalizeLocalServices() {
	if c.LocalServices.Mode == "" {
		c.LocalServices.Mode = localServicesModeManual
	}
	if c.LocalServices.Provider == "" {
		c.LocalServices.Provider = localServicesProviderDocker
	}
	if c.LocalServices.ComposeFile == "" {
		c.LocalServices.ComposeFile = defaultLocalServicesCompose
	}
	if c.LocalServices.ProjectDir == "" {
		c.LocalServices.ProjectDir = defaultLocalServicesProject
	}
	if c.LocalServices.StartupTimeoutSeconds == 0 {
		c.LocalServices.StartupTimeoutSeconds = defaultLocalStartupTimeout
	}
	if c.LocalServices.HealthcheckIntervalSeconds == 0 {
		c.LocalServices.HealthcheckIntervalSeconds = defaultLocalHealthInterval
	}
	if len(c.LocalServices.RequiredNetworks) == 0 {
		c.LocalServices.RequiredNetworks = []string{defaultLocalServicesNetwork}
	}
	defaults := defaultManagedServices()
	if c.LocalServices.Services == nil {
		c.LocalServices.Services = defaults
		return
	}
	for name, svc := range defaults {
		current, ok := c.LocalServices.Services[name]
		if !ok {
			c.LocalServices.Services[name] = svc
			continue
		}
		if strings.TrimSpace(current.BaseURL) == "" {
			current.BaseURL = svc.BaseURL
		}
		if strings.TrimSpace(current.HealthPath) == "" {
			current.HealthPath = svc.HealthPath
		}
		if strings.TrimSpace(current.ModelsPath) == "" {
			current.ModelsPath = svc.ModelsPath
		}
		c.LocalServices.Services[name] = current
	}
}

func (c *Config) resolveLocalServicePaths() error {
	if !c.LocalServices.Enabled {
		return nil
	}
	if strings.TrimSpace(c.LocalServices.ComposeFile) != "" {
		resolved, err := resolveProjectRelativePath(c.ProjectRoot, c.LocalServices.ComposeFile)
		if err != nil {
			return fmt.Errorf("invalid field local_services.compose_file=%q: %w", c.LocalServices.ComposeFile, err)
		}
		c.LocalServices.ComposeFile = resolved
	}
	if strings.TrimSpace(c.LocalServices.ProjectDir) != "" {
		resolved, err := resolveProjectRelativePath(c.ProjectRoot, c.LocalServices.ProjectDir)
		if err != nil {
			return fmt.Errorf("invalid field local_services.project_dir=%q: %w", c.LocalServices.ProjectDir, err)
		}
		c.LocalServices.ProjectDir = resolved
	}
	return nil
}

func appendMissingStrings(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := append([]string(nil), existing...)
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		out = append(out, value)
		seen[value] = struct{}{}
	}
	return out
}

func validateEnumValue(field, raw, expected string, allowed ...string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, value := range allowed {
		if normalized == value {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("invalid field %s=%q (expected %s)", field, raw, expected)
}

func projectRelativePath(projectRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	root := projectRoot
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, path)
}

func resolveProjectRelativePath(projectRoot, path string) (string, error) {
	return expandPath(projectRelativePath(projectRoot, path))
}

func (c *Config) validateLogLevel() error {
	_, err := validateEnumValue("log_level", c.LogLevel, "debug, info, warn, or error", "debug", "info", "warn", "error")
	return err
}

func (c *Config) validateLogFormat() error {
	_, err := validateEnumValue("log_format", c.LogFormat, "json or text", "json", "text")
	return err
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

	if strings.TrimSpace(c.Agent.ToolResultStoreRoot) != "" {
		toolResultStoreRoot, err := expandPath(c.Agent.ToolResultStoreRoot)
		if err != nil {
			return fmt.Errorf("invalid field agent.tool_result_store_root=%q: %w", c.Agent.ToolResultStoreRoot, err)
		}
		c.Agent.ToolResultStoreRoot = toolResultStoreRoot
	}

	if err := c.resolveLocalServicePaths(); err != nil {
		return err
	}
	if err := c.resolveMemoryPaths(); err != nil {
		return err
	}
	c.syncBrainRuntimeMemoryFields()

	if !c.Brain.Enabled {
		return nil
	}
	brainBackend, err := validateEnumValue("brain.backend", c.Brain.Backend, "shunter", brainBackendShunter)
	if err != nil {
		return err
	}
	c.Brain.Backend = brainBackend
	if c.Memory.Backend != memoryBackendShunter {
		return fmt.Errorf("invalid field brain.backend=%q (requires memory.backend: shunter)", c.Brain.Backend)
	}
	return nil
}

func (c *Config) resolveMemoryPaths() error {
	memoryBackend, err := validateEnumValue("memory.backend", c.Memory.Backend, "shunter", memoryBackendShunter)
	if err != nil {
		return err
	}
	c.Memory.Backend = memoryBackend
	rpcTransport, err := validateEnumValue("memory.rpc.transport", c.Memory.RPC.Transport, "unix", memoryRPCTransportUnix)
	if err != nil {
		return err
	}
	c.Memory.RPC.Transport = rpcTransport
	if strings.TrimSpace(c.Memory.ShunterDataDir) == "" {
		return errors.New("invalid field memory.shunter_data_dir=\"\" (must be set when memory.backend is shunter)")
	}
	dataDir, err := resolveProjectRelativePath(c.ProjectRoot, c.Memory.ShunterDataDir)
	if err != nil {
		return fmt.Errorf("invalid field memory.shunter_data_dir=%q: %w", c.Memory.ShunterDataDir, err)
	}
	c.Memory.ShunterDataDir = dataDir
	if strings.TrimSpace(c.Memory.RPC.Path) == "" {
		return errors.New("invalid field memory.rpc.path=\"\" (must be set when memory.backend is shunter)")
	}
	rpcPath, err := resolveProjectRelativePath(c.ProjectRoot, c.Memory.RPC.Path)
	if err != nil {
		return fmt.Errorf("invalid field memory.rpc.path=%q: %w", c.Memory.RPC.Path, err)
	}
	c.Memory.RPC.Path = rpcPath
	return nil
}

func (c *Config) syncBrainRuntimeMemoryFields() {
	c.Brain.MemoryBackend = c.Memory.Backend
	c.Brain.ShunterDataDir = c.Memory.ShunterDataDir
	c.Brain.DurableAck = c.Memory.DurableAck
	c.Brain.RPCTransport = c.Memory.RPC.Transport
	c.Brain.RPCPath = c.Memory.RPC.Path
}

func (c *Config) validateRouting() error {
	if c.Routing.Default.Provider == "" {
		return errors.New("invalid field routing.default.provider=\"\" (must name a configured provider)")
	}
	if _, ok := c.Providers[c.Routing.Default.Provider]; !ok {
		return fmt.Errorf("invalid field routing.default.provider=%q (provider not configured)", c.Routing.Default.Provider)
	}
	if c.Routing.Fallback.Provider == "" && c.Routing.Fallback.Model == "" {
		return nil
	}
	if c.Routing.Fallback.Provider == "" {
		return errors.New("invalid field routing.fallback.provider=\"\" (must be set when routing.fallback.model is configured)")
	}
	if c.Routing.Fallback.Model == "" {
		return errors.New("invalid field routing.fallback.model=\"\" (must be set when routing.fallback.provider is configured)")
	}
	if _, ok := c.Providers[c.Routing.Fallback.Provider]; !ok {
		return fmt.Errorf("invalid field routing.fallback.provider=%q (provider not configured)", c.Routing.Fallback.Provider)
	}
	return nil
}

func (c *Config) validateProviders() error {
	for name, provider := range c.Providers {
		if _, err := validateEnumValue("providers."+name+".type", provider.Type, "anthropic, codex, or openai-compatible", allowedProviderTypes...); err != nil {
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

func (c *Config) validateLocalServices() error {
	if !c.LocalServices.Enabled {
		return nil
	}
	mode, err := validateEnumValue("local_services.mode", c.LocalServices.Mode, "off, manual, or auto", localServicesModeOff, localServicesModeManual, localServicesModeAuto)
	if err != nil {
		return err
	}
	c.LocalServices.Mode = mode
	providerName := strings.ToLower(strings.TrimSpace(c.LocalServices.Provider))
	if providerName == "" {
		return errors.New("invalid field local_services.provider=\"\" (must not be empty)")
	}
	if providerName != localServicesProviderDocker {
		return fmt.Errorf("invalid field local_services.provider=%q (expected %s)", c.LocalServices.Provider, localServicesProviderDocker)
	}
	c.LocalServices.Provider = providerName
	if c.LocalServices.StartupTimeoutSeconds <= 0 {
		return fmt.Errorf("invalid field local_services.startup_timeout_seconds=%d (must be > 0)", c.LocalServices.StartupTimeoutSeconds)
	}
	if c.LocalServices.HealthcheckIntervalSeconds <= 0 {
		return fmt.Errorf("invalid field local_services.healthcheck_interval_seconds=%d (must be > 0)", c.LocalServices.HealthcheckIntervalSeconds)
	}
	if len(c.LocalServices.Services) == 0 {
		return errors.New("invalid field local_services.services (must configure at least one managed service)")
	}
	for name, svc := range c.LocalServices.Services {
		if strings.TrimSpace(svc.BaseURL) == "" {
			return fmt.Errorf("invalid field local_services.services.%s.base_url=\"\" (must not be empty)", name)
		}
	}
	return nil
}

func (c *Config) validateNumericFields() error {
	for field, value := range map[string]int{
		"index.max_rag_results":               c.Index.MaxRAGResults,
		"index.max_tree_lines":                c.Index.MaxTreeLines,
		"index.max_file_size_bytes":           c.Index.MaxFileSizeBytes,
		"index.max_total_file_size_bytes":     c.Index.MaxTotalFileSizeBytes,
		"agent.max_iterations_per_turn":       c.Agent.MaxIterationsPerTurn,
		"agent.loop_detection_threshold":      c.Agent.LoopDetectionThreshold,
		"agent.tool_output_max_tokens":        c.Agent.ToolOutputMaxTokens,
		"agent.shell_timeout_seconds":         c.Agent.ShellTimeoutSeconds,
		"context.max_assembled_tokens":        c.Context.MaxAssembledTokens,
		"context.max_chunks":                  c.Context.MaxChunks,
		"context.max_explicit_files":          c.Context.MaxExplicitFiles,
		"context.convention_budget_tokens":    c.Context.ConventionBudgetTokens,
		"context.git_context_budget_tokens":   c.Context.GitContextBudgetTokens,
		"context.structural_hop_depth":        c.Context.StructuralHopDepth,
		"context.structural_hop_budget":       c.Context.StructuralHopBudget,
		"context.momentum_lookback_turns":     c.Context.MomentumLookbackTurns,
		"context.compression_head_preserve":   c.Context.CompressionHeadPreserve,
		"context.compression_tail_preserve":   c.Context.CompressionTailPreserve,
		"brain.max_brain_tokens":              c.Brain.MaxBrainTokens,
		"brain.graph_hop_depth":               c.Brain.GraphHopDepth,
		"brain.lint_stale_days":               c.Brain.LintStaleDays,
		"agent.history_summarize_after_turns": c.Agent.HistorySummarizeAfterTurns,
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

func (c *Config) validateAgentRoles() error {
	for name, role := range c.AgentRoles {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return errors.New("invalid field agent_roles (role names must be non-empty)")
		}
		trimmedPrompt := strings.TrimSpace(role.SystemPrompt)
		if trimmedPrompt == "" {
			if !embeddedprompts.Has(trimmedName) {
				return fmt.Errorf("invalid field agent_roles.%s.system_prompt=\"\" (must not be empty unless the role has a built-in default)", name)
			}
		} else if strings.HasPrefix(trimmedPrompt, "builtin:") {
			builtinRole := strings.TrimSpace(strings.TrimPrefix(trimmedPrompt, "builtin:"))
			if !embeddedprompts.Has(builtinRole) {
				return fmt.Errorf("invalid field agent_roles.%s.system_prompt=%q (unknown built-in role system prompt)", name, role.SystemPrompt)
			}
		}
		for _, group := range role.Tools {
			if !toolgroup.IsKnown(strings.TrimSpace(group)) {
				return fmt.Errorf("invalid field agent_roles.%s.tools (unsupported tool group %q; expected %s)", name, group, toolgroup.Message())
			}
		}
		if role.MaxTurns <= 0 && role.MaxTurns != 0 {
			return fmt.Errorf("invalid field agent_roles.%s.max_turns=%d (must be > 0 when specified)", name, role.MaxTurns)
		}
		if role.MaxTokens <= 0 && role.MaxTokens != 0 {
			return fmt.Errorf("invalid field agent_roles.%s.max_tokens=%d (must be > 0 when specified)", name, role.MaxTokens)
		}
		if role.Timeout < 0 {
			return fmt.Errorf("invalid field agent_roles.%s.timeout=%s (must be > 0 when specified)", name, role.Timeout.Duration())
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
