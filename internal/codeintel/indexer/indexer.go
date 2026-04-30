package indexer

import (
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// IndexConfig holds configuration for the indexing pipeline.
type IndexConfig struct {
	ProjectName     string
	ProjectRoot     string
	KnownFileHashes map[string]string
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
