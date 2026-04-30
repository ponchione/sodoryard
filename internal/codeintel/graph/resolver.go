package graph

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/pathglob"
)

// Resolver orchestrates language detection and dispatches to analyzers.
type Resolver struct {
	projectRoot string
	cfg         *AnalyzerConfig
	include     []string
	exclude     []string
}

// NewResolver creates a new language resolver.
func NewResolver(projectRoot string, cfg *AnalyzerConfig) *Resolver {
	return NewResolverWithIndexRules(projectRoot, cfg, nil, nil)
}

// NewResolverWithIndexRules creates a resolver that applies the configured
// index include/exclude globs to graph language detection and analyzers.
func NewResolverWithIndexRules(projectRoot string, cfg *AnalyzerConfig, include []string, exclude []string) *Resolver {
	return &Resolver{
		projectRoot: projectRoot,
		cfg:         cfg,
		include:     append([]string(nil), include...),
		exclude:     append([]string(nil), exclude...),
	}
}

type detectedLanguages struct {
	hasGo         bool
	hasTypeScript bool
	hasPython     bool
	tsconfigPath  string
}

// detectLanguages checks which languages are present in the project.
func (r *Resolver) detectLanguages() detectedLanguages {
	dl := detectedLanguages{}

	if _, err := os.Stat(filepath.Join(r.projectRoot, "go.mod")); err == nil {
		dl.hasGo = true
	}

	tsconfigPath := r.cfg.TypeScript.TsconfigPath
	if tsconfigPath == "" {
		tsconfigPath = "tsconfig.json"
	}
	if _, err := os.Stat(filepath.Join(r.projectRoot, tsconfigPath)); err == nil {
		dl.hasTypeScript = true
		dl.tsconfigPath = filepath.Join(r.projectRoot, tsconfigPath)
	}

	filepath.WalkDir(r.projectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if r.skipDir(path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(r.projectRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(path, ".py") && r.includesPath(rel) {
			dl.hasPython = true
			return filepath.SkipAll
		}
		return nil
	})

	return dl
}

func (r *Resolver) skipDir(path string, base string) bool {
	if base == "venv" || base == ".venv" || base == "node_modules" ||
		base == "__pycache__" || base == ".git" {
		return true
	}
	rel, err := filepath.Rel(r.projectRoot, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return false
	}
	return pathglob.MatchAny(r.exclude, rel+"/__dir__")
}

func (r *Resolver) includesPath(relPath string) bool {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if len(r.include) > 0 && !pathglob.MatchAny(r.include, relPath) {
		return false
	}
	return !pathglob.MatchAny(r.exclude, relPath)
}

// Analyze detects languages and dispatches to appropriate analyzers.
func (r *Resolver) Analyze() (*AnalysisResult, error) {
	dl := r.detectLanguages()
	result := &AnalysisResult{}

	slog.Info("detected languages",
		"go", dl.hasGo,
		"typescript", dl.hasTypeScript,
		"python", dl.hasPython,
	)

	if dl.hasGo && r.cfg.Go.Enabled {
		slog.Info("running Go analyzer")
		analyzer, err := NewGoAnalyzer(r.projectRoot)
		if err != nil {
			slog.Warn("Go analyzer init failed, skipping", "error", err)
		} else {
			analyzer.SetFileFilter(r.includesPath)
			goResult, err := analyzer.Analyze()
			if err != nil {
				slog.Warn("Go analysis failed", "error", err)
			} else {
				result.Merge(goResult)
			}
		}
	}

	if dl.hasTypeScript && r.cfg.TypeScript.Enabled {
		slog.Info("running TypeScript analyzer")
		analyzer, err := NewTSAnalyzer()
		if err != nil {
			slog.Warn("TS analyzer init failed, skipping", "error", err)
		} else {
			tsResult, err := analyzer.Analyze(r.projectRoot, dl.tsconfigPath)
			if err != nil {
				slog.Warn("TypeScript analysis failed", "error", err)
			} else {
				result.Merge(filterAnalysisResult(tsResult, r.includesPath))
			}
		}
	}

	if dl.hasPython && r.cfg.Python.Enabled {
		slog.Info("running Python analyzer")
		analyzer := NewPythonAnalyzer(r.projectRoot)
		analyzer.SetFileFilter(r.includesPath)
		pyResult, err := analyzer.Analyze()
		if err != nil {
			slog.Warn("Python analysis failed", "error", err)
		} else {
			result.Merge(pyResult)
		}
	}

	slog.Info("graph analysis complete",
		"total_symbols", len(result.Symbols),
		"total_edges", len(result.Edges),
		"total_boundary", len(result.BoundarySymbols),
	)

	return result, nil
}

func filterAnalysisResult(input *AnalysisResult, includePath func(string) bool) *AnalysisResult {
	if input == nil || includePath == nil {
		return input
	}
	output := &AnalysisResult{
		Symbols:         make([]Symbol, 0, len(input.Symbols)),
		Edges:           make([]Edge, 0, len(input.Edges)),
		BoundarySymbols: append([]BoundarySymbol(nil), input.BoundarySymbols...),
	}
	allowedSymbols := make(map[string]struct{}, len(input.Symbols))
	for _, symbol := range input.Symbols {
		if symbol.FilePath != "" && !includePath(symbol.FilePath) {
			continue
		}
		output.Symbols = append(output.Symbols, symbol)
		allowedSymbols[symbol.ID] = struct{}{}
	}

	boundarySymbols := make(map[string]struct{}, len(input.BoundarySymbols))
	for _, boundary := range input.BoundarySymbols {
		boundarySymbols[boundary.ID] = struct{}{}
	}
	for _, edge := range input.Edges {
		if _, ok := allowedSymbols[edge.SourceID]; !ok {
			continue
		}
		if _, ok := allowedSymbols[edge.TargetID]; ok {
			output.Edges = append(output.Edges, edge)
			continue
		}
		if _, ok := boundarySymbols[edge.TargetID]; ok {
			output.Edges = append(output.Edges, edge)
		}
	}
	return output
}
