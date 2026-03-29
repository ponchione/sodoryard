# Task 07: Parser Interface Implementation and ParseFile Method

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03, Task 04, Task 05, L1-E01 (Parser interface)

---

## Description

Wire together all extraction methods into the public `ParseFile` method that satisfies the `Parser` interface from L1-E01 (or a richer variant). This is the main entry point: given a file path and its source content, it looks up the pre-loaded package, walks the AST, and returns `RawChunk`s along with relationship metadata (calls, types used, interface implementations, imports) for the indexer to merge into `Chunk` objects.

## Acceptance Criteria

- [ ] `GoParser` implements the `codeintel.Parser` interface: `Parse(filePath string, content []byte) ([]codeintel.RawChunk, error)`
- [ ] Additionally exposes a richer method: `ParseWithRelationships(filePath string, content []byte) ([]codeintel.RawChunk, *FileRelationships, error)`
- [ ] `FileRelationships` struct defined in `internal/codeintel/goparser/`:
  ```go
  type FileRelationships struct {
      Imports []string                    // import paths for the file
      ChunkRelationships map[string]*ChunkRelationships  // keyed by chunk name
  }
  type ChunkRelationships struct {
      Calls           []string  // qualified names of called functions
      TypesUsed       []string  // qualified names of referenced types
      ImplementsIfaces []string // qualified names of implemented interfaces
  }
  ```
- [ ] The `Parse` method delegates to `ParseWithRelationships` and discards the relationships (for callers that only need chunks)
- [ ] File lookup: uses `pkgsByFile` to find the `*packages.Package` for the given `filePath`. If the file is not found in any loaded package, returns an error (the caller can fall back to tree-sitter)
- [ ] For each declaration extracted by `extractDeclarations`:
  - Calls `extractCalls` for function/method declarations
  - Calls `extractTypesUsed` for type declarations
  - Calls `getImplementedIfaces` for type declarations
- [ ] Calls `extractImports` once per file and attaches the result to `FileRelationships.Imports`
- [ ] The relationship data for each chunk is keyed by the chunk's `Name` field (same name as in the `RawChunk`)
- [ ] If a file parses successfully but contains zero declarations (e.g., a file with only a package clause and imports), returns an empty `[]codeintel.RawChunk` and populated `FileRelationships.Imports` — this is not an error
- [ ] Errors during individual declaration extraction (e.g., position resolution failure) are logged and that declaration is skipped — the method does not fail the entire file for one bad declaration

### Import Tracking

- [ ] Method on `GoParser`: `extractImports(file *ast.File) []string`
- [ ] Iterates `file.Imports` (the `[]*ast.ImportSpec` slice)
- [ ] Each import spec's `Path.Value` is unquoted and recorded (e.g., the string `"fmt"`, not `"\"fmt\""`)
- [ ] Named imports (e.g., `import myfmt "fmt"`) record the path, not the alias — the result is `"fmt"`
- [ ] Dot imports (e.g., `import . "fmt"`) record the path `"fmt"`
- [ ] Blank imports (e.g., `import _ "net/http/pprof"`) record the path `"net/http/pprof"`
- [ ] Standard library and third-party imports are both included (no filtering)
- [ ] The returned `[]string` is sorted alphabetically
- [ ] A file with no imports returns an empty (non-nil) slice `[]string{}`
