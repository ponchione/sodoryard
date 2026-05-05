package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	brainparser "github.com/ponchione/sodoryard/internal/brain/parser"
	"github.com/ponchione/sodoryard/internal/codeintel"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

const (
	defaultBrainSearchTopK     = 8
	defaultBrainGraphHopDepth  = 1
	maxBrainGraphHopDepth      = 3
	brainGraphScoreDecayFactor = 0.85
)

type graphFrontierNode struct {
	Path  string
	Score float64
}

type brainGraph struct {
	incoming map[string][]string
	outgoing map[string][]string
}

type HybridBrainSearcher struct {
	keywordBackend brain.Backend
	semanticStore  codeintel.Store
	embedder       codeintel.Embedder
	queries        *appdb.Queries
	projectID      string
}

func NewHybridBrainSearcher(keywordBackend brain.Backend, semanticStore codeintel.Store, embedder codeintel.Embedder, queries *appdb.Queries, projectID string) *HybridBrainSearcher {
	return &HybridBrainSearcher{
		keywordBackend: keywordBackend,
		semanticStore:  semanticStore,
		embedder:       embedder,
		queries:        queries,
		projectID:      strings.TrimSpace(projectID),
	}
}

func (s *HybridBrainSearcher) Search(ctx stdctx.Context, request BrainSearchRequest) ([]BrainSearchResult, error) {
	if s == nil {
		return nil, nil
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = "auto"
	}
	topK := request.MaxResults
	if topK <= 0 {
		topK = defaultBrainSearchTopK
	}
	results := map[string]*BrainSearchResult{}

	if mode == "auto" || mode == "keyword" {
		if err := s.addKeywordResults(ctx, query, topK, results); err != nil {
			return nil, err
		}
	}
	if mode == "auto" || mode == "semantic" {
		if err := s.addSemanticResults(ctx, query, topK, results); err != nil {
			return nil, err
		}
	}
	if request.IncludeGraphHops {
		s.expandGraphResults(ctx, results, brainGraphHopDepth(request.GraphHopDepth))
	}

	flattened := make([]BrainSearchResult, 0, len(results))
	for _, result := range results {
		s.enrichMetadata(ctx, result)
		result.MatchSources = uniqueSortedStrings(result.MatchSources)
		result.MatchMode = brainMatchMode(*result)
		result.FinalScore = maxFloat(result.FinalScore, result.LexicalScore, result.SemanticScore)
		if strings.TrimSpace(result.Title) == "" {
			result.Title = filepath.Base(result.DocumentPath)
		}
		flattened = append(flattened, *result)
	}
	sort.SliceStable(flattened, func(i, j int) bool {
		if flattened[i].FinalScore != flattened[j].FinalScore {
			return flattened[i].FinalScore > flattened[j].FinalScore
		}
		if flattened[i].SemanticScore != flattened[j].SemanticScore {
			return flattened[i].SemanticScore > flattened[j].SemanticScore
		}
		if flattened[i].LexicalScore != flattened[j].LexicalScore {
			return flattened[i].LexicalScore > flattened[j].LexicalScore
		}
		return flattened[i].DocumentPath < flattened[j].DocumentPath
	})
	if len(flattened) > topK {
		flattened = flattened[:topK]
	}
	return flattened, nil
}

func (s *HybridBrainSearcher) addKeywordResults(ctx stdctx.Context, query string, topK int, results map[string]*BrainSearchResult) error {
	if s.keywordBackend == nil {
		return nil
	}
	var (
		hits []brain.SearchHit
		err  error
	)
	if limited, ok := s.keywordBackend.(brain.LimitedKeywordSearcher); ok {
		hits, err = limited.SearchKeywordLimit(ctx, query, topK)
	} else {
		hits, err = s.keywordBackend.SearchKeyword(ctx, query)
	}
	if err != nil {
		return err
	}
	for _, hit := range hits {
		path := strings.TrimSpace(hit.Path)
		if path == "" || brain.IsOperationalDocument(path) {
			continue
		}
		entry := ensureBrainSearchResult(results, path)
		prevLexical := entry.LexicalScore
		entry.LexicalScore = maxFloat(entry.LexicalScore, hit.Score)
		entry.FinalScore = maxFloat(entry.FinalScore, hit.Score)
		if strings.TrimSpace(entry.Snippet) == "" || hit.Score > prevLexical {
			entry.Snippet = strings.TrimSpace(hit.Snippet)
		}
		entry.MatchSources = append(entry.MatchSources, "keyword")
	}
	return nil
}

func (s *HybridBrainSearcher) addSemanticResults(ctx stdctx.Context, query string, topK int, results map[string]*BrainSearchResult) error {
	if s.semanticStore == nil || s.embedder == nil {
		return nil
	}
	embedding, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return fmt.Errorf("embed brain query: %w", err)
	}
	matches, err := s.semanticStore.VectorSearch(ctx, embedding, topK, codeintel.Filter{Language: "markdown"})
	if err != nil {
		return fmt.Errorf("semantic brain vector search: %w", err)
	}
	for _, match := range matches {
		path := strings.TrimSpace(match.Chunk.FilePath)
		if path == "" || brain.IsOperationalDocument(path) {
			continue
		}
		entry := ensureBrainSearchResult(results, path)
		prevSemantic := entry.SemanticScore
		entry.SemanticScore = maxFloat(entry.SemanticScore, match.Score)
		entry.FinalScore = maxFloat(entry.FinalScore, match.Score)
		if strings.TrimSpace(entry.Snippet) == "" || match.Score > prevSemantic {
			entry.Snippet = strings.TrimSpace(match.Chunk.Body)
		}
		if match.Chunk.ChunkType == codeintel.ChunkTypeSection {
			entry.SectionHeading = strings.TrimSpace(match.Chunk.Name)
		}
		entry.MatchSources = append(entry.MatchSources, "semantic")
	}
	return nil
}

func (s *HybridBrainSearcher) expandGraphResults(ctx stdctx.Context, results map[string]*BrainSearchResult, maxDepth int) {
	if s == nil || len(results) == 0 || maxDepth <= 0 {
		return
	}
	if s.queries == nil || strings.TrimSpace(s.projectID) == "" {
		s.expandBackendGraphResults(ctx, results, maxDepth)
		return
	}
	frontier := make([]graphFrontierNode, 0, len(results))
	bestDepth := make(map[string]int, len(results))
	for path, result := range results {
		path = strings.TrimSpace(path)
		if path == "" || brain.IsOperationalDocument(path) {
			continue
		}
		frontier = append(frontier, graphFrontierNode{Path: path, Score: result.FinalScore})
		bestDepth[path] = 0
	}
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		nextByPath := map[string]graphFrontierNode{}
		for _, current := range frontier {
			incoming, err := s.queries.ListBrainLinksByTarget(ctx, appdb.ListBrainLinksByTargetParams{ProjectID: s.projectID, TargetPath: current.Path})
			if err == nil {
				for _, link := range incoming {
					s.applyGraphLink(results, nextByPath, bestDepth, current, strings.TrimSpace(link.SourcePath), depth, "backlink")
				}
			}
			outgoing, err := s.queries.ListBrainLinksBySource(ctx, appdb.ListBrainLinksBySourceParams{ProjectID: s.projectID, SourcePath: current.Path})
			if err == nil {
				for _, link := range outgoing {
					s.applyGraphLink(results, nextByPath, bestDepth, current, strings.TrimSpace(link.TargetPath), depth, "graph")
				}
			}
		}
		frontier = make([]graphFrontierNode, 0, len(nextByPath))
		for _, node := range nextByPath {
			frontier = append(frontier, node)
		}
		sort.SliceStable(frontier, func(i, j int) bool {
			if frontier[i].Score != frontier[j].Score {
				return frontier[i].Score > frontier[j].Score
			}
			return frontier[i].Path < frontier[j].Path
		})
	}
}

func (s *HybridBrainSearcher) expandBackendGraphResults(ctx stdctx.Context, results map[string]*BrainSearchResult, maxDepth int) {
	graph, err := s.loadBackendBrainGraph(ctx)
	if err != nil || graph == nil {
		return
	}
	frontier := make([]graphFrontierNode, 0, len(results))
	bestDepth := make(map[string]int, len(results))
	for resultPath, result := range results {
		resultPath = strings.TrimSpace(resultPath)
		if resultPath == "" || brain.IsOperationalDocument(resultPath) {
			continue
		}
		frontier = append(frontier, graphFrontierNode{Path: resultPath, Score: result.FinalScore})
		bestDepth[resultPath] = 0
	}
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		nextByPath := map[string]graphFrontierNode{}
		for _, current := range frontier {
			for _, sourcePath := range graph.incoming[current.Path] {
				s.applyGraphLink(results, nextByPath, bestDepth, current, sourcePath, depth, "backlink")
			}
			for _, targetPath := range graph.outgoing[current.Path] {
				s.applyGraphLink(results, nextByPath, bestDepth, current, targetPath, depth, "graph")
			}
		}
		frontier = make([]graphFrontierNode, 0, len(nextByPath))
		for _, node := range nextByPath {
			frontier = append(frontier, node)
		}
		sort.SliceStable(frontier, func(i, j int) bool {
			if frontier[i].Score != frontier[j].Score {
				return frontier[i].Score > frontier[j].Score
			}
			return frontier[i].Path < frontier[j].Path
		})
	}
}

func (s *HybridBrainSearcher) applyGraphLink(results map[string]*BrainSearchResult, nextByPath map[string]graphFrontierNode, bestDepth map[string]int, parent graphFrontierNode, candidatePath string, depth int, source string) {
	if candidatePath == "" || candidatePath == parent.Path || brain.IsOperationalDocument(candidatePath) {
		return
	}
	seenDepth, seen := bestDepth[candidatePath]
	if seen && seenDepth < depth && !(seenDepth == 0 && depth == 1) {
		return
	}
	if seen && seenDepth == 0 && depth == 1 && source == "backlink" {
		return
	}
	entry := ensureBrainSearchResult(results, candidatePath)
	graphScore := graphExpandedScore(parent.Score, depth)
	entry.FinalScore = maxFloat(entry.FinalScore, graphScore)
	if strings.TrimSpace(entry.Snippet) == "" {
		entry.Snippet = graphExpansionSnippet(parent.Path, source, depth)
	}
	if entry.GraphHopDepth == 0 || depth < entry.GraphHopDepth || (depth == entry.GraphHopDepth && strings.TrimSpace(entry.GraphSourcePath) == "") {
		entry.GraphSourcePath = parent.Path
		entry.GraphHopDepth = depth
	}
	entry.MatchSources = append(entry.MatchSources, source)
	if seen && seenDepth < depth {
		return
	}
	bestDepth[candidatePath] = depth
	if existing, ok := nextByPath[candidatePath]; ok {
		existing.Score = maxFloat(existing.Score, graphScore)
		nextByPath[candidatePath] = existing
		return
	}
	nextByPath[candidatePath] = graphFrontierNode{Path: candidatePath, Score: graphScore}
}

func brainGraphHopDepth(depth int) int {
	if depth <= 0 {
		return defaultBrainGraphHopDepth
	}
	if depth > maxBrainGraphHopDepth {
		return maxBrainGraphHopDepth
	}
	return depth
}

func graphExpandedScore(parentScore float64, depth int) float64 {
	if parentScore <= 0 {
		parentScore = 0.5
	}
	score := parentScore
	for hop := 0; hop < depth; hop++ {
		score *= brainGraphScoreDecayFactor
	}
	return score
}

func graphExpansionSnippet(parentPath string, source string, depth int) string {
	label := strings.TrimSpace(filepath.Base(parentPath))
	if label == "" {
		label = strings.TrimSpace(parentPath)
	}
	prefix := "Graph-related to"
	if source == "backlink" {
		prefix = "Backlinks to"
	}
	if depth > 1 {
		return fmt.Sprintf("%s %s via %d graph hops.", prefix, label, depth)
	}
	return fmt.Sprintf("%s %s.", prefix, label)
}

func (s *HybridBrainSearcher) enrichMetadata(ctx stdctx.Context, result *BrainSearchResult) {
	if s == nil || result == nil || strings.TrimSpace(result.DocumentPath) == "" {
		return
	}
	if s.queries != nil && strings.TrimSpace(s.projectID) != "" {
		doc, err := s.queries.GetBrainDocumentByPath(ctx, appdb.GetBrainDocumentByPathParams{ProjectID: s.projectID, Path: result.DocumentPath})
		if err == nil {
			if strings.TrimSpace(result.Title) == "" && doc.Title.Valid {
				result.Title = strings.TrimSpace(doc.Title.String)
			}
			if len(result.Tags) == 0 && doc.Tags.Valid && strings.TrimSpace(doc.Tags.String) != "" {
				var tags []string
				if err := json.Unmarshal([]byte(doc.Tags.String), &tags); err == nil {
					result.Tags = uniqueSortedStrings(tags)
				}
			}
		} else if err != sql.ErrNoRows {
			return
		}
	}
	if strings.TrimSpace(result.Title) != "" && len(result.Tags) > 0 {
		return
	}
	doc, err := s.parseBackendDocument(ctx, result.DocumentPath)
	if err != nil {
		return
	}
	if strings.TrimSpace(result.Title) == "" {
		result.Title = strings.TrimSpace(doc.Title)
	}
	if len(result.Tags) == 0 {
		result.Tags = uniqueSortedStrings(doc.Tags)
	}
}

func (s *HybridBrainSearcher) parseBackendDocument(ctx stdctx.Context, docPath string) (brainparser.Document, error) {
	if s == nil || s.keywordBackend == nil {
		return brainparser.Document{}, fmt.Errorf("brain backend unavailable")
	}
	docPath = strings.TrimSpace(docPath)
	if docPath == "" {
		return brainparser.Document{}, fmt.Errorf("brain document path is required")
	}
	content, err := s.keywordBackend.ReadDocument(ctx, docPath)
	if err != nil {
		return brainparser.Document{}, err
	}
	return brainparser.ParseDocument(docPath, content)
}

func (s *HybridBrainSearcher) loadBackendBrainGraph(ctx stdctx.Context) (*brainGraph, error) {
	if s == nil || s.keywordBackend == nil {
		return nil, nil
	}
	paths, err := s.keywordBackend.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(paths))
	for _, docPath := range paths {
		docPath = strings.TrimSpace(docPath)
		if docPath == "" || brain.IsOperationalDocument(docPath) {
			continue
		}
		known[docPath] = struct{}{}
	}
	graph := &brainGraph{
		incoming: map[string][]string{},
		outgoing: map[string][]string{},
	}
	for _, docPath := range paths {
		docPath = strings.TrimSpace(docPath)
		if docPath == "" || brain.IsOperationalDocument(docPath) {
			continue
		}
		doc, err := s.parseBackendDocument(ctx, docPath)
		if err != nil {
			continue
		}
		seenTargets := map[string]struct{}{}
		for _, link := range doc.Wikilinks {
			targetPath := resolveBrainLinkTarget(docPath, link.Target, known)
			if targetPath == "" || targetPath == docPath || brain.IsOperationalDocument(targetPath) {
				continue
			}
			if _, ok := seenTargets[targetPath]; ok {
				continue
			}
			seenTargets[targetPath] = struct{}{}
			graph.outgoing[docPath] = append(graph.outgoing[docPath], targetPath)
			graph.incoming[targetPath] = append(graph.incoming[targetPath], docPath)
		}
	}
	sortBrainGraph(graph)
	return graph, nil
}

func resolveBrainLinkTarget(sourcePath string, target string, known map[string]struct{}) string {
	target = strings.Trim(path.Clean(strings.ReplaceAll(strings.TrimSpace(target), `\`, "/")), "/")
	if target == "" || target == "." {
		return ""
	}
	candidates := []string{target}
	if !strings.HasSuffix(strings.ToLower(target), ".md") {
		candidates = append(candidates, target+".md")
	}
	if !strings.Contains(target, "/") {
		sourceDir := path.Dir(strings.ReplaceAll(strings.TrimSpace(sourcePath), `\`, "/"))
		if sourceDir != "." && sourceDir != "" {
			relative := path.Clean(path.Join(sourceDir, target))
			candidates = append(candidates, relative)
			if !strings.HasSuffix(strings.ToLower(relative), ".md") {
				candidates = append(candidates, relative+".md")
			}
		}
	}
	for _, candidate := range candidates {
		if _, ok := known[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func sortBrainGraph(graph *brainGraph) {
	if graph == nil {
		return
	}
	for key := range graph.incoming {
		graph.incoming[key] = uniqueSortedStrings(graph.incoming[key])
	}
	for key := range graph.outgoing {
		graph.outgoing[key] = uniqueSortedStrings(graph.outgoing[key])
	}
}

func ensureBrainSearchResult(results map[string]*BrainSearchResult, path string) *BrainSearchResult {
	if existing, ok := results[path]; ok {
		return existing
	}
	created := &BrainSearchResult{DocumentPath: path}
	results[path] = created
	return created
}

func brainMatchMode(result BrainSearchResult) string {
	sources := uniqueSortedStrings(result.MatchSources)
	if len(sources) == 0 {
		return ""
	}
	hasKeyword := stringSliceContains(sources, "keyword")
	hasSemantic := stringSliceContains(sources, "semantic")
	hasBacklink := stringSliceContains(sources, "backlink")
	hasGraph := stringSliceContains(sources, "graph")
	hasLexicalSemantic := hasKeyword || hasSemantic
	hasStructural := hasBacklink || hasGraph

	switch {
	case hasLexicalSemantic && hasStructural:
		if structuralMatchLabel(result) == "backlink" {
			return "hybrid-backlink"
		}
		return "hybrid-graph"
	case hasStructural:
		return structuralMatchLabel(result)
	case len(sources) == 1:
		return sources[0]
	default:
		return "hybrid"
	}
}

func structuralMatchLabel(result BrainSearchResult) string {
	if result.GraphHopDepth > 1 || stringSliceContains(result.MatchSources, "graph") {
		return "graph"
	}
	if stringSliceContains(result.MatchSources, "backlink") {
		return "backlink"
	}
	return "graph"
}

func stringSliceContains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
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
	sort.Strings(out)
	return out
}

func maxFloat(values ...float64) float64 {
	best := 0.0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}
