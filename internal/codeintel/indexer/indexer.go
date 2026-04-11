package indexer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// IndexConfig holds configuration for the indexing pipeline.
type IndexConfig struct {
	ProjectName string
	ProjectRoot string
	DataDir     string
	Include     []string
	Exclude     []string
}

// IndexOpts configures optional behavior for IndexRepo.
type IndexOpts struct {
	Force bool
}

// parsedFile holds the results of parsing a single file during pass 1.
type parsedFile struct {
	relPath  string
	absPath  string
	language string
	content  []byte
	fileHash string
	chunks   []codeintel.Chunk
}

// chunkRef identifies a chunk by its position within the parsed file list.
type chunkRef struct {
	fileIdx  int
	chunkIdx int
}

// IndexRepo walks the project directory and indexes all matching files using a
// three-pass pipeline:
//  1. Walk + parse: collect all chunks with forward call metadata
//  2. Reverse call graph: populate CalledBy on target chunks
//  3. Describe + embed + store: enrich with descriptions and embeddings, then upsert
func IndexRepo(
	ctx context.Context,
	cfg IndexConfig,
	parser codeintel.Parser,
	store codeintel.Store,
	embedder codeintel.Embedder,
	describer codeintel.Describer,
	opts IndexOpts,
) error {
	root := cfg.ProjectRoot
	if root == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	hashFile := filepath.Join(cfg.DataDir, "rag_file_hashes.json")
	fileHashes, err := loadFileHashes(hashFile)
	if err != nil {
		slog.Warn("could not load file hashes, re-indexing all", "error", err)
		fileHashes = make(map[string]string)
	}

	if opts.Force {
		slog.Info("force flag set, re-indexing all files")
		fileHashes = map[string]string{"__schema_version": codeintel.SchemaVersion}
	}

	if fileHashes["__schema_version"] != codeintel.SchemaVersion {
		slog.Info("schema version changed, forcing full re-index",
			"old", fileHashes["__schema_version"],
			"new", codeintel.SchemaVersion,
		)
		fileHashes = map[string]string{"__schema_version": codeintel.SchemaVersion}
	}

	parsed, filesVisited, walkErr := walkAndParse(absRoot, cfg, parser, fileHashes)
	if walkErr != nil {
		return walkErr
	}

	buildReverseCallGraph(parsed)

	filesIndexed, totalChunks := describeEmbedStore(ctx, parsed, store, embedder, describer, fileHashes)

	slog.Info("indexing complete",
		"files_visited", filesVisited,
		"files_indexed", filesIndexed,
		"total_chunks", totalChunks,
	)

	if saveErr := saveFileHashes(hashFile, fileHashes); saveErr != nil {
		slog.Warn("could not save file hashes", "error", saveErr)
	}

	return nil
}

// walkAndParse walks the project directory and parses all matching files (Pass 1).
func walkAndParse(
	absRoot string,
	cfg IndexConfig,
	parser codeintel.Parser,
	fileHashes map[string]string,
) ([]parsedFile, int, error) {
	var filesVisited int
	var parsed []parsedFile

	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		filesVisited++

		if len(cfg.Include) > 0 && !matchesAnyGlob(cfg.Include, relPath) {
			return nil
		}
		if matchesAnyGlob(cfg.Exclude, relPath) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read file", "path", relPath, "err", err)
			return nil
		}

		fHash := codeintel.ContentHash(string(content))
		if fileHashes[relPath] == fHash {
			return nil
		}

		lang := langFromExt(filepath.Ext(relPath))

		rawChunks, err := parser.Parse(path, content)
		if err != nil {
			slog.Warn("parse failed", "path", relPath, "err", err)
			return nil
		}
		if len(rawChunks) == 0 {
			return nil
		}

		chunks := make([]codeintel.Chunk, len(rawChunks))
		for i, raw := range rawChunks {
			chunks[i] = newChunk(raw, cfg.ProjectName, relPath, lang, "")
		}

		parsed = append(parsed, parsedFile{
			relPath:  relPath,
			absPath:  path,
			language: lang,
			content:  content,
			fileHash: fHash,
			chunks:   chunks,
		})

		return nil
	})

	return parsed, filesVisited, err
}

// buildReverseCallGraph populates CalledBy on target chunks (Pass 2).
func buildReverseCallGraph(parsed []parsedFile) {
	pkgIndex := make(map[string][]chunkRef)
	dirSet := make(map[string]bool)
	for fi, pf := range parsed {
		for ci, chunk := range pf.chunks {
			if chunk.Name == "" {
				continue
			}
			if pf.language == "go" {
				dir := filepath.Dir(pf.relPath)
				key := dir + "." + chunk.Name
				pkgIndex[key] = append(pkgIndex[key], chunkRef{fi, ci})
				dirSet[dir] = true
			}
		}
	}

	allDirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		allDirs = append(allDirs, d)
	}

	suffixToDir := make(map[string][]string)
	for d := range dirSet {
		slashDir := filepath.ToSlash(d)
		suffixToDir[slashDir] = append(suffixToDir[slashDir], d)
		suffixToDir[d] = append(suffixToDir[d], d)
		parts := strings.Split(slashDir, "/")
		for i := 1; i < len(parts); i++ {
			suffix := strings.Join(parts[i:], "/")
			suffixToDir[suffix] = append(suffixToDir[suffix], d)
		}
	}

	for fi, pf := range parsed {
		for ci, chunk := range pf.chunks {
			for _, call := range chunk.Calls {
				var targets []chunkRef

				if call.Package != "" {
					if dirs, ok := suffixToDir[call.Package]; ok {
						for _, d := range dirs {
							targets = append(targets, pkgIndex[d+"."+call.Name]...)
						}
					}
				}

				if call.Package == "" || strings.Contains(pf.relPath, call.Package) {
					for _, d := range allDirs {
						targets = append(targets, pkgIndex[d+"."+call.Name]...)
					}
				}

				callerRef := codeintel.FuncRef{
					Name:    chunk.Name,
					Package: call.Package,
				}

				seen := make(map[chunkRef]bool)
				for _, t := range targets {
					if seen[t] {
						continue
					}
					seen[t] = true
					if t.fileIdx == fi && t.chunkIdx == ci {
						continue
					}
					parsed[t.fileIdx].chunks[t.chunkIdx].CalledBy = append(
						parsed[t.fileIdx].chunks[t.chunkIdx].CalledBy,
						callerRef,
					)
				}
			}
		}
	}
}

// describeEmbedStore enriches chunks and upserts them (Pass 3).
func describeEmbedStore(
	ctx context.Context,
	parsed []parsedFile,
	store codeintel.Store,
	embedder codeintel.Embedder,
	describer codeintel.Describer,
	fileHashes map[string]string,
) (int, int) {
	var filesIndexed, totalChunks int

	for i := range parsed {
		pf := &parsed[i]

		descContent := string(pf.content)
		if relCtx := formatRelationshipContext(pf.chunks); relCtx != "" {
			descContent = descContent + "\n\n" + relCtx
		}

		descriptions, err := describer.DescribeFile(ctx, descContent, "")
		if err != nil {
			slog.Warn("describe failed", "path", pf.relPath, "err", err)
			descriptions = nil
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
			continue
		}

		for j := range pf.chunks {
			if j < len(embeddings) {
				pf.chunks[j].Embedding = embeddings[j]
			}
		}

		if err := store.Upsert(ctx, pf.chunks); err != nil {
			slog.Warn("upsert failed", "path", pf.relPath, "err", err)
			continue
		}

		fileHashes[pf.relPath] = pf.fileHash
		filesIndexed++
		totalChunks += len(pf.chunks)
		slog.Info("indexed file", "path", pf.relPath, "chunks", len(pf.chunks))
	}

	return filesIndexed, totalChunks
}
