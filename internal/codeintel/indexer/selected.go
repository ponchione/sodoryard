package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// FileIndexResult captures the indexing outcome for one file.
type FileIndexResult struct {
	Path       string
	FileHash   string
	ChunkCount int
}

// IndexResult captures the outcome of indexing a selected file set.
type IndexResult struct {
	Files       []FileIndexResult
	TotalChunks int
	FailedFiles []string
}

// IndexFiles indexes the provided relative file paths.
//
// Files that parse to zero chunks are still included in the returned Files slice
// with ChunkCount=0 so callers can persist file-hash state and avoid reprocessing
// the same unchanged zero-chunk files forever.
func IndexFiles(
	ctx context.Context,
	cfg IndexConfig,
	parser codeintel.Parser,
	store codeintel.Store,
	embedder codeintel.Embedder,
	describer codeintel.Describer,
	relPaths []string,
) (*IndexResult, error) {
	root := cfg.ProjectRoot
	if root == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	sorted := append([]string(nil), relPaths...)
	sort.Strings(sorted)

	result := &IndexResult{Files: make([]FileIndexResult, 0, len(sorted))}
	parsed := make([]parsedFile, 0, len(sorted))
	seen := make(map[string]struct{}, len(sorted))

	for _, relPath := range sorted {
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." || relPath == "" {
			continue
		}
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}

		absPath := filepath.Join(absRoot, filepath.FromSlash(relPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			result.FailedFiles = append(result.FailedFiles, relPath)
			slog.Warn("failed to read selected file", "path", relPath, "err", err)
			continue
		}
		if !utf8.Valid(content) {
			slog.Warn("skipping selected file with invalid utf-8", "path", relPath)
			continue
		}

		fileHash := codeintel.ContentHash(string(content))
		result.Files = append(result.Files, FileIndexResult{
			Path:     relPath,
			FileHash: fileHash,
		})

		rawChunks, err := parser.Parse(absPath, content)
		if err != nil {
			result.FailedFiles = append(result.FailedFiles, relPath)
			slog.Warn("parse failed", "path", relPath, "err", err)
			continue
		}
		if len(rawChunks) == 0 {
			continue
		}

		lang := langFromExt(filepath.Ext(relPath))
		chunks := make([]codeintel.Chunk, len(rawChunks))
		for i, raw := range rawChunks {
			chunks[i] = newChunk(raw, cfg.ProjectName, relPath, lang, "")
		}

		parsed = append(parsed, parsedFile{
			relPath:  relPath,
			absPath:  absPath,
			language: lang,
			content:  content,
			fileHash: fileHash,
			chunks:   chunks,
		})
	}

	buildReverseCallGraph(parsed)

	chunkCounts, failedDuringStore, totalChunks := describeEmbedStoreDetailed(ctx, parsed, store, embedder, describer)
	for i := range result.Files {
		result.Files[i].ChunkCount = chunkCounts[result.Files[i].Path]
	}
	result.TotalChunks = totalChunks
	result.FailedFiles = append(result.FailedFiles, failedDuringStore...)

	if len(result.FailedFiles) > 0 {
		sort.Strings(result.FailedFiles)
		result.FailedFiles = uniqueStrings(result.FailedFiles)
		return result, fmt.Errorf("index selected files: %d files failed", len(result.FailedFiles))
	}

	return result, nil
}

func describeEmbedStoreDetailed(
	ctx context.Context,
	parsed []parsedFile,
	store codeintel.Store,
	embedder codeintel.Embedder,
	describer codeintel.Describer,
) (map[string]int, []string, int) {
	chunkCounts := make(map[string]int, len(parsed))
	failed := make([]string, 0)
	var totalChunks int

	for i := range parsed {
		pf := &parsed[i]

		descContent := string(pf.content)
		if relCtx := formatRelationshipContext(pf.chunks); relCtx != "" {
			descContent = descContent + "\n\n" + relCtx
		}

		descriptions, err := describer.DescribeFile(ctx, descContent, "")
		if err != nil {
			slog.Warn("describe failed", "path", pf.relPath, "err", err)
			failed = append(failed, pf.relPath)
			continue
		}

		descMap := make(map[string]string, len(descriptions))
		for _, d := range descriptions {
			descMap[d.Name] = d.Description
		}

		embedTexts := make([]string, len(pf.chunks))
		for j := range pf.chunks {
			desc := descMap[pf.chunks[j].Name]
			pf.chunks[j].Description = desc
			embedTexts[j] = pf.chunks[j].Signature + "\n" + desc
		}

		embeddings, err := embedder.EmbedTexts(ctx, embedTexts)
		if err != nil {
			slog.Warn("embed failed", "path", pf.relPath, "err", err)
			failed = append(failed, pf.relPath)
			continue
		}

		for j := range pf.chunks {
			if j < len(embeddings) {
				pf.chunks[j].Embedding = embeddings[j]
			}
		}

		if err := store.Upsert(ctx, pf.chunks); err != nil {
			slog.Warn("upsert failed", "path", pf.relPath, "err", err)
			failed = append(failed, pf.relPath)
			continue
		}

		chunkCounts[pf.relPath] = len(pf.chunks)
		totalChunks += len(pf.chunks)
		slog.Info("indexed selected file", "path", pf.relPath, "chunks", len(pf.chunks))
	}

	return chunkCounts, failed, totalChunks
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
