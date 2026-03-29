package graph

// AnalyzerConfig controls which language analyzers the Resolver runs.
type AnalyzerConfig struct {
	Go         GoAnalyzerConfig
	TypeScript TSAnalyzerConfig
	Python     PythonAnalyzerConfig
}

// GoAnalyzerConfig controls the Go analyzer.
type GoAnalyzerConfig struct {
	Enabled bool
}

// TSAnalyzerConfig controls the TypeScript analyzer.
type TSAnalyzerConfig struct {
	Enabled      bool
	TsconfigPath string // relative to project root; defaults to "tsconfig.json"
}

// PythonAnalyzerConfig controls the Python analyzer.
type PythonAnalyzerConfig struct {
	Enabled bool
	Include []string // glob patterns for files to include
	Exclude []string // glob patterns for files to exclude
}

// DefaultAnalyzerConfig returns a config with all analyzers enabled.
func DefaultAnalyzerConfig() AnalyzerConfig {
	return AnalyzerConfig{
		Go:         GoAnalyzerConfig{Enabled: true},
		TypeScript: TSAnalyzerConfig{Enabled: true},
		Python:     PythonAnalyzerConfig{Enabled: true},
	}
}
