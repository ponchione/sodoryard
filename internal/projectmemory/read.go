package projectmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/types"
)

func (r *Runtime) ReadDocument(ctx context.Context, path string) (Document, string, error) {
	normalized, err := normalizeDocumentPath(path)
	if err != nil {
		return Document{}, "", err
	}
	var doc Document
	var content string
	var found bool
	err = r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableDocuments, indexDocumentsPrimary, types.NewString(normalized)) {
			doc = decodeDocumentRow(row)
			found = true
			break
		}
		if !found || doc.Deleted {
			return nil
		}
		var chunks []documentChunk
		for _, row := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(normalized)) {
			chunks = append(chunks, decodeChunkRow(row))
		}
		content = joinDocumentChunks(chunks)
		return nil
	})
	if err != nil {
		return Document{}, "", err
	}
	if !found || doc.Deleted {
		return Document{}, "", fmt.Errorf("Document not found: %s", normalized)
	}
	return doc, content, nil
}

func (r *Runtime) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	prefix := strings.Trim(filepathSlash(directory), "/")
	if prefix != "" {
		prefix += "/"
	}
	var paths []string
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			if prefix != "" && !strings.HasPrefix(doc.Path, prefix) {
				continue
			}
			paths = append(paths, doc.Path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *Runtime) SearchDocuments(ctx context.Context, query string, maxResults int) ([]SearchHit, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	normalizedQuery := normalizeKeyword(query)
	if normalizedQuery == "" {
		return nil, nil
	}
	var hits []SearchHit
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			var chunks []documentChunk
			for _, chunkRow := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(doc.Path)) {
				chunks = append(chunks, decodeChunkRow(chunkRow))
			}
			content := joinDocumentChunks(chunks)
			score := keywordScore(normalizedQuery, doc, content)
			if score == 0 {
				continue
			}
			hits = append(hits, SearchHit{
				Path:    doc.Path,
				Snippet: snippetAround(content, normalizedQuery),
				Score:   score,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits, nil
}

func (r *Runtime) ReadBrainIndexState(ctx context.Context) (BrainIndexState, bool, error) {
	var state BrainIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableBrainIndexState, indexBrainIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeBrainIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return BrainIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ReadCodeIndexState(ctx context.Context) (CodeIndexState, bool, error) {
	var state CodeIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeIndexState, indexCodeIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeCodeIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return CodeIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ListCodeFileIndexStates(ctx context.Context) ([]CodeFileIndexState, error) {
	var states []CodeFileIndexState
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeFileIndexState, indexCodeFileIndexStateProject, types.NewString(DefaultProjectID)) {
			states = append(states, decodeCodeFileIndexStateRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].FilePath < states[j].FilePath
	})
	return states, nil
}

type SearchHit struct {
	Path    string
	Snippet string
	Score   float64
}

func filepathSlash(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
}

func normalizeKeyword(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func keywordScore(query string, doc Document, content string) float64 {
	score := 0.0
	if strings.Contains(normalizeKeyword(doc.Path), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.Title), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.TagsJSON), query) {
		score += 1
	}
	score += float64(strings.Count(normalizeKeyword(content), query))
	return score
}

func snippetAround(content string, normalizedQuery string) string {
	if content == "" {
		return ""
	}
	normalizedContent := normalizeKeyword(content)
	idx := strings.Index(normalizedContent, normalizedQuery)
	if idx < 0 {
		runes := []rune(content)
		if len(runes) > 160 {
			return string(runes[:160]) + "..."
		}
		return content
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(normalizedQuery) + 80
	if end > len(content) {
		end = len(content)
	}
	return strings.TrimSpace(content[start:end])
}
