**Status:** Draft v0.1 **Last Updated:** 2026-03-28 **Author:** Mitchell

---

## Overview

The tool system is Layer 4 in sodoryard's architecture. It provides the agent with structured capabilities to read, write, search, and execute within the project. The agent loop (Layer 5) dispatches tool calls against this layer; the provider layer (Layer 2) serializes tool definitions into the LLM request.

Every tool exists because it provides something the `shell` tool alone cannot — structured output for the web UI, safety guardrails, or token efficiency. Adding a new tool is: implement the interface, register it, done.

This document is the authoritative reference for the tool system. It consolidates tool definitions previously spread across [[05-agent-loop]] (core tools, dispatch model) and [[09-project-brain]] (brain tools), and adds the missing pieces: the Go interface contract, registry mechanics, JSON Schema definitions, and per-tool implementation specifications.

---

## Tool Interface

All tools implement a single Go interface. The agent loop dispatches against this interface exclusively — it has no knowledge of individual tool implementations.

```go
// Tool is the contract every tool must satisfy.
type Tool interface {
    // Name returns the tool's identifier as seen by the LLM.
    // Must be unique across the registry. Lowercase, underscored.
    Name() string

    // Description returns a one-line description for the LLM's tool definition.
    // This is what the LLM reads to decide whether to use this tool.
    Description() string

    // ParameterSchema returns the JSON Schema object describing the tool's
    // input parameters. Serialized directly into the LLM request's tools array.
    ParameterSchema() json.RawMessage

    // Purity declares whether the tool has side effects.
    // Pure tools run concurrently; mutating tools run sequentially.
    Purity() ToolPurity

    // Execute runs the tool with the given arguments.
    // args is the raw JSON object from the LLM's tool_use block.
    // Returns the tool result as a string (the content of the role=tool message).
    // Errors are returned as tool results, not Go errors — the LLM sees them.
    Execute(ctx context.Context, args json.RawMessage) (string, error)
}

type ToolPurity int

const (
    Pure     ToolPurity = iota // Read-only. No side effects. Safe for concurrent execution.
    Mutating                   // Writes to filesystem, executes commands. Sequential only.
)
```

### Design Decisions

**`Execute` returns `(string, error)`.** The `string` is the tool result content that gets appended to conversation history as a `role=tool` message. The `error` return is reserved for catastrophic failures (tool binary not found, internal panic) — not for tool-level errors like "file not found." Tool-level errors are returned as the string result so the LLM can see them and self-correct. This is Layer 1 of the agent loop's three-tier error recovery ([[05-agent-loop]]).

**`ParameterSchema` returns `json.RawMessage`.** Each tool defines its own JSON Schema as a static JSON blob. No reflection, no struct tag magic. The schema is the tool's contract with the LLM — it must be hand-authored and precise. The registry collects these schemas and serializes them into the provider request.

**`Purity` is declarative, not inferred.** Each tool states whether it's pure or mutating at registration time. The agent loop trusts this declaration for dispatch scheduling. There is no runtime analysis of side effects.

---

## Tool Registry

The registry is the agent loop's view of available tools. It's populated at startup and immutable for the lifetime of the process.

```go
type Registry struct {
    tools map[string]Tool
    order []string // insertion order, for deterministic schema serialization
}

func NewRegistry() *Registry { ... }

// Register adds a tool. Panics on duplicate name (programming error, not runtime).
func (r *Registry) Register(t Tool) { ... }

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool { ... }

// ToolDefinitions returns the JSON array of tool definitions for the LLM request.
// Format matches the Anthropic/OpenAI tool definition schema.
func (r *Registry) ToolDefinitions() json.RawMessage { ... }
```

### Registration at Startup

Tools are registered in the server's initialization code. The order is deterministic — tools are always serialized in the same order for prompt cache stability.

```go
registry := tools.NewRegistry()

// Core tools
registry.Register(tools.NewFileRead(projectRoot))
registry.Register(tools.NewFileWrite(projectRoot))
registry.Register(tools.NewFileEdit(projectRoot))
registry.Register(tools.NewSearchText(projectRoot))
registry.Register(tools.NewSearchSemantic(ragSearcher))
registry.Register(tools.NewGitStatus(projectRoot))
registry.Register(tools.NewGitDiff(projectRoot))
registry.Register(tools.NewShell(projectRoot, shellConfig))

// Brain tools (conditional on brain being enabled)
if brainConfig.Enabled {
    obsidianClient := brain.NewObsidianClient(brainConfig)
    registry.Register(brain.NewBrainRead(obsidianClient))
    registry.Register(brain.NewBrainWrite(obsidianClient, brainIndexer))
    registry.Register(brain.NewBrainSearch(obsidianClient, brainSearcher))
    registry.Register(brain.NewBrainUpdate(obsidianClient, brainIndexer))
}
```

Brain tools are conditionally registered. If the brain is disabled in config (`brain.enabled: false`) or Obsidian isn't running, the LLM never sees these tools in its definitions and cannot call them.

### Tool Definition Serialization

`ToolDefinitions()` produces the JSON array that gets embedded in every LLM request. The format is Anthropic-native (the hardest provider to satisfy — see [[03-provider-architecture]]):

```json
[
  {
    "name": "file_read",
    "description": "Read file contents, optionally with a line range. Returns content with line numbers.",
    "input_schema": {
      "type": "object",
      "properties": {
        "path": { "type": "string", "description": "File path relative to project root" },
        "line_start": { "type": "integer", "description": "First line to read (1-indexed, inclusive)" },
        "line_end": { "type": "integer", "description": "Last line to read (1-indexed, inclusive)" }
      },
      "required": ["path"]
    }
  },
  ...
]
```

The provider layer translates this format if needed. Anthropic uses `input_schema`; OpenAI uses `parameters` inside a `function` wrapper. This translation is the provider's responsibility ([[03-provider-architecture]]), not the tool system's. The tool system always produces Anthropic-format definitions.

### Input Validation

Before `Execute` is called, the agent loop validates the LLM's arguments against the tool's `ParameterSchema()`:

1. Parse the LLM's tool_use `input` field as JSON.
2. Validate against the tool's JSON Schema (required fields present, types correct).
3. If validation fails, return the validation error as the tool result — the LLM sees "Invalid arguments: missing required field 'path'" and self-corrects.
4. If validation succeeds, pass the raw JSON to `Execute`.

Validation uses a lightweight JSON Schema validator (e.g., `santhosh-tekuri/jsonschema` or `xeipuuv/gojsonschema`). The specific library is an implementation detail — any spec-compliant validator works.

---

## Execution Model

The agent loop owns tool dispatch. The tool system provides the interface and implementations; the agent loop decides execution order and concurrency. This section consolidates the dispatch logic from [[05-agent-loop]] for completeness — the agent loop doc remains the authority on iteration and error recovery.

### Purity-Based Dispatch

When the LLM returns a batch of tool calls:

1. **Partition** into pure and mutating calls based on each tool's `Purity()`.
2. **Execute all pure calls concurrently** — one goroutine per call. Collect results via a `sync.WaitGroup` or `errgroup.Group`.
3. **Execute mutating calls sequentially** in the order the LLM specified them. The LLM's order is intentional — a `file_write` followed by a `shell` (run tests) must execute in that sequence.
4. **Assemble results** in the original batch order (matching tool call IDs). The LLM expects results in the same order it issued the calls.

Pure calls execute concurrently with each other but complete before mutating calls begin. This is simple and correct — no interleaving of reads and writes within a single batch.

### Context Propagation

Every `Execute` call receives the turn's `context.Context`. This context carries:

- **Cancellation signal** — when the user cancels a turn, the context is cancelled. Pure tools (file reads, searches) abort immediately. The shell tool sends SIGTERM to the process group, escalating to SIGKILL after 5 seconds.
- **Timeout** — per-tool timeout (from config or tool-specific default). Implemented as a derived context with `context.WithTimeout`. The shell tool has its own timeout (default 120s) applied before the context timeout.

### Working Directory

All tools execute with the project root as the working directory. File paths in tool arguments are relative to the project root. The shell tool's working directory is the project root.

The project root is injected into each tool at construction time (see Registration at Startup). Tools resolve relative paths against it and refuse operations outside it (see Safety & Sandboxing).

---

## Output Handling

### Truncation

Tool results can be large — a 5000-line file read, a verbose build log, a broad ripgrep search. Each tool has a configurable output size limit. The global default is `tool_output_max_tokens: 50000` (from agent config). Individual tools can override this.

When a result exceeds the limit, it's truncated with a notice that helps the LLM recover:

```
[Output truncated — showing first 200 lines of 5847. Use file_read with line_start/line_end for specific sections.]
```

The truncation notice is tool-specific. `file_read` suggests line ranges. `search_text` suggests narrower patterns. `shell` suggests redirecting to a file. Each tool authors its own truncation guidance.

### Error Enrichment

Tool errors are tool results, not exceptions. When a tool encounters an error, it returns a descriptive error string as its result. The LLM sees the error and self-corrects.

Errors are enriched with context that helps the LLM recover on its next attempt:

Note: the brain-tool rows below were originally written for the pre-MCP Obsidian Local REST path. The current runtime brain backend is MCP-backed; remaining REST-specific wording is historical spec debt.

|Error|Enrichment|
|---|---|
|File not found|List available files in the parent directory|
|File outside project root|Show the project root path|
|Shell command not found|Suggest common alternatives or check PATH|
|Shell command timeout|Show partial output captured before timeout|
|Shell denylist match|Explain which pattern matched and why|
|Edit search string not found|Show the first few lines of the file so the LLM can see the actual content|
|Edit search string not unique|Show all match locations so the LLM can refine|
|Ripgrep no matches|Suggest alternative patterns or broader search|
|Brain document not found|List available documents in the vault|
|Brain Obsidian API unreachable|Explain that Obsidian must be running with the REST API plugin|

This enrichment is the single highest-leverage improvement for agent self-correction rates. A bare "file not found" forces the LLM to guess; "file not found, available files: middleware.go, service.go, types.go" lets it fix the path immediately.

### Result Format

Tool results are appended to conversation history as `role=tool` messages per the data model ([[08-data-model]]):

- `content`: plain text — the tool's output string
- `tool_use_id`: the ID from the LLM's `tool_use` block that triggered this execution
- `tool_name`: denormalized for queryability

The tool system produces the content string. The agent loop wraps it in the message structure and handles persistence.

---

## JSON Schema Tool Definitions

The following are the complete JSON Schema definitions for every tool. These schemas are what the LLM sees in every request. They must be precise — the LLM generates tool call arguments based solely on these definitions and descriptions.

### file_read

```json
{
  "name": "file_read",
  "description": "Read file contents, optionally with a line range. Returns content with line numbers for reference in file_edit.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File path relative to project root"
      },
      "line_start": {
        "type": "integer",
        "description": "First line to read (1-indexed, inclusive). Omit to start from beginning."
      },
      "line_end": {
        "type": "integer",
        "description": "Last line to read (1-indexed, inclusive). Omit to read to end of file."
      }
    },
    "required": ["path"]
  }
}
```

### file_write

```json
{
  "name": "file_write",
  "description": "Create or overwrite a file with the given content. Creates parent directories if needed. Returns a diff preview of the change.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File path relative to project root"
      },
      "content": {
        "type": "string",
        "description": "Complete file content to write"
      }
    },
    "required": ["path", "content"]
  }
}
```

### file_edit

```json
{
  "name": "file_edit",
  "description": "Apply a targeted edit to a file by searching for a unique string and replacing it. Much more token-efficient than rewriting an entire file. The search string must match exactly one location in the file.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File path relative to project root"
      },
      "old_str": {
        "type": "string",
        "description": "The exact string to find in the file. Must be unique — appears exactly once. Include enough surrounding context to ensure uniqueness."
      },
      "new_str": {
        "type": "string",
        "description": "The replacement string. Use empty string to delete the matched text."
      }
    },
    "required": ["path", "old_str", "new_str"]
  }
}
```

### search_text

```json
{
  "name": "search_text",
  "description": "Search for a text pattern across the project using ripgrep. Returns matches with surrounding context lines and file:line locations.",
  "input_schema": {
    "type": "object",
    "properties": {
      "pattern": {
        "type": "string",
        "description": "Search pattern (regex supported). Use literal strings for exact matches."
      },
      "path": {
        "type": "string",
        "description": "Directory or file to search within, relative to project root. Omit to search entire project."
      },
      "include": {
        "type": "string",
        "description": "Glob pattern to filter files (e.g., '*.go', '*.ts'). Omit to search all files."
      },
      "context_lines": {
        "type": "integer",
        "description": "Number of context lines around each match (default: 2)"
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum number of matches to return (default: 50)"
      }
    },
    "required": ["pattern"]
  }
}
```

### search_semantic

```json
{
  "name": "search_semantic",
  "description": "Semantic search against the codebase using RAG. Finds code by meaning, not just text. Returns relevant functions, types, and code chunks with file paths, descriptions, and similarity scores. Use when you need to find code related to a concept rather than a specific string.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Natural language description of what you're looking for (e.g., 'authentication middleware', 'database connection pooling')"
      },
      "language": {
        "type": "string",
        "description": "Filter by language (e.g., 'go', 'typescript', 'python'). Omit to search all languages."
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum number of results to return (default: 10)"
      }
    },
    "required": ["query"]
  }
}
```

### git_status

```json
{
  "name": "git_status",
  "description": "Get current git status: branch name, staged/unstaged/untracked files, and recent commit history.",
  "input_schema": {
    "type": "object",
    "properties": {
      "recent_commits": {
        "type": "integer",
        "description": "Number of recent commits to include (default: 5)"
      }
    },
    "required": []
  }
}
```

### git_diff

```json
{
  "name": "git_diff",
  "description": "Show git diff of working tree changes, staged changes, or between two refs.",
  "input_schema": {
    "type": "object",
    "properties": {
      "ref": {
        "type": "string",
        "description": "Git ref to diff against (e.g., 'HEAD', 'main', 'abc1234'). Omit for working tree diff."
      },
      "ref2": {
        "type": "string",
        "description": "Second git ref for comparing two refs (e.g., diff ref..ref2). Omit for working tree or staged."
      },
      "staged": {
        "type": "boolean",
        "description": "Show staged changes only (default: false)"
      },
      "path": {
        "type": "string",
        "description": "Limit diff to a specific file or directory path"
      }
    },
    "required": []
  }
}
```

### shell

```json
{
  "name": "shell",
  "description": "Execute a shell command in the project directory. Captures stdout, stderr, and exit code. Use for running tests, builds, linting, or any command-line operation not covered by other tools.",
  "input_schema": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "description": "The shell command to execute (passed to /bin/sh -c)"
      },
      "timeout": {
        "type": "integer",
        "description": "Timeout in seconds (default: 120). The command is killed if it exceeds this."
      }
    },
    "required": ["command"]
  }
}
```

### brain_read

```json
{
  "name": "brain_read",
  "description": "Read a document from the project brain (Obsidian vault). Returns the full markdown content including frontmatter. Optionally includes backlinks — other documents that link to this one.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Document path relative to vault root (e.g., 'architecture/provider-design.md')"
      },
      "include_backlinks": {
        "type": "boolean",
        "description": "Include a list of documents that link to this one (default: false)"
      }
    },
    "required": ["path"]
  }
}
```

### brain_write

```json
{
  "name": "brain_write",
  "description": "Create or overwrite a document in the project brain (Obsidian vault). Write Obsidian-native markdown with YAML frontmatter, [[wikilinks]], and #tags. Use for capturing durable project knowledge — architectural decisions, debugging journals, conventions, session summaries.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Document path relative to vault root (e.g., 'debugging/lancedb-nil-slice.md'). Creates parent directories if needed."
      },
      "content": {
        "type": "string",
        "description": "Full markdown content including YAML frontmatter block"
      }
    },
    "required": ["path", "content"]
  }
}
```

### brain_update

```json
{
  "name": "brain_update",
  "description": "Append to, prepend to, or replace a section in an existing brain document. More surgical than brain_write — use this to add information to a debugging journal or update a specific section without rewriting the whole file.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Document path relative to vault root"
      },
      "operation": {
        "type": "string",
        "enum": ["append", "prepend", "replace_section"],
        "description": "How to apply the edit: append to end, prepend to beginning, or replace a specific section"
      },
      "content": {
        "type": "string",
        "description": "Content to add or replace with"
      },
      "section": {
        "type": "string",
        "description": "Heading text to target for replace_section (e.g., '## Workaround'). Required when operation is replace_section."
      }
    },
    "required": ["path", "operation", "content"]
  }
}
```

### brain_search

```json
{
  "name": "brain_search",
  "description": "Search the project brain (Obsidian vault) for relevant documents. Supports keyword search (exact matches via Obsidian), semantic search (meaning-based via embeddings), or both. Returns document titles, paths, relevant snippets, and similarity scores.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Search query — natural language for semantic search, keywords/tags for keyword search"
      },
      "mode": {
        "type": "string",
        "enum": ["keyword", "semantic", "auto"],
        "description": "Search mode: 'keyword' for exact matches via Obsidian search, 'semantic' for meaning-based via embeddings, 'auto' runs both and merges results (default: auto)"
      },
      "tags": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Filter results by tags (e.g., ['debugging', 'convention'])"
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum results to return (default: 10)"
      }
    },
    "required": ["query"]
  }
}
```

---

## Individual Tool Specifications

### file_read

**Purpose:** Read file contents with optional line range. Provides structured output with line numbers that enable the UI to render syntax-highlighted code and that give the LLM precise references for subsequent `file_edit` calls.

**Purity:** Pure

**Why not just `shell cat`:** Structured output with line numbers. Token-efficient partial reads via line ranges. The UI can render the result as syntax-highlighted code with clickable line numbers. `cat` gives plain text with no metadata.

**Return format:**

```
File: internal/auth/middleware.go (89 lines)

 15 │ func AuthMiddleware(next http.Handler) http.Handler {
 16 │     return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
 17 │         token := r.Header.Get("Authorization")
 18 │         if token == "" {
 19 │             http.Error(w, "missing auth token", http.StatusUnauthorized)
 20 │             return
 21 │         }
    ...
 48 │     })
 49 │ }
```

Line numbers are left-padded and pipe-delimited. The header includes the file path and total line count. When a line range is specified, only that range is shown.

**Error cases:**

- File not found → "Error: file not found: {path}\nAvailable files in {parent_dir}/: {listing}"
- File outside project root → "Error: path '{path}' is outside the project root ({project_root})"
- Binary file detected → "Error: '{path}' appears to be a binary file. Use shell to inspect binary files."
- Output exceeds limit → Truncated with: "[Output truncated — showing first {N} lines of {total}. Use file_read with line_start/line_end for specific sections.]"

**Implementation notes:**

- Resolve path relative to project root. Reject any path that escapes via `..` traversal.
- Detect binary files by checking for null bytes in the first 8KB.
- Line numbering is 1-indexed, matching universal convention.
- When no line range is specified, read the entire file (subject to truncation).
- Per-tool truncation default: 500 lines. Configurable.

### file_write

**Purpose:** Create or overwrite a file. Returns a diff preview so the LLM (and the UI) can confirm the change. Creates parent directories automatically.

**Purity:** Mutating

**Why not just `shell echo/cat >file`:** Diff preview in structured output. Automatic directory creation. Project root enforcement. The UI renders the diff inline with syntax highlighting.

**Return format:**

For a new file:

```
Created: internal/auth/types.go (23 lines)
```

For an overwrite:

```
Updated: internal/auth/types.go (23 lines, was 18 lines)

Diff preview (first 20 lines):
--- a/internal/auth/types.go
+++ b/internal/auth/types.go
@@ -5,6 +5,11 @@
 type Claims struct {
     UserID string
+    Email  string
+    Role   string
 }
```

**Error cases:**

- Path outside project root → "Error: path '{path}' is outside the project root ({project_root})"
- Permission denied → "Error: permission denied writing to '{path}'"
- Disk full → "Error: disk full — could not write '{path}'"

**Implementation notes:**

- Create parent directories with `os.MkdirAll` (mode 0755).
- For overwrites, compute the diff before writing using a Go diff library (e.g., `sergi/go-diff`). Show the first 20 lines of the unified diff.
- Write to a temporary file first, then rename (atomic write). Prevents partial writes on crash.
- File permissions: 0644 for new files.

### file_edit

**Purpose:** Apply a targeted search-and-replace edit. Dramatically more token-efficient than `file_write` for small edits — the LLM sends only the search string and replacement, not the full file.

**Purity:** Mutating

**Why not just `file_write`:** Token efficiency. Editing a 500-line file to change 3 lines via `file_write` requires the LLM to output all 500 lines. With `file_edit`, it outputs only the old and new strings — typically under 20 lines total.

**Return format:**

```
Applied edit to: internal/auth/middleware.go

--- a/internal/auth/middleware.go
+++ b/internal/auth/middleware.go
@@ -17,3 +17,5 @@
         token := r.Header.Get("Authorization")
-        if token == "" {
+        token = strings.TrimPrefix(token, "Bearer ")
+        if token == "" || token == r.Header.Get("Authorization") {
             http.Error(w, "missing auth token", http.StatusUnauthorized)
```

**Error cases:**

- File not found → "Error: file not found: {path}\nAvailable files in {parent_dir}/: {listing}"
- Search string not found → "Error: old_str not found in {path}.\nFirst 30 lines of file:\n{preview}"
- Search string not unique → "Error: old_str found {N} times in {path} (must be unique).\nLocations: line {L1}, line {L2}, ..."
- Path outside project root → "Error: path '{path}' is outside the project root ({project_root})"

**Implementation notes:**

- Read the file, search for `old_str` as a literal substring (not regex).
- Require exactly one match. Zero matches or multiple matches are errors with enriched output.
- Replace the match, compute unified diff, write back atomically.
- The "not found" error shows the first 30 lines of the file so the LLM can see what the actual content looks like and adjust its search string.
- The "not unique" error shows all match locations so the LLM can include more surrounding context to disambiguate.

### search_text

**Purpose:** Ripgrep-based text search across the project. Provides structured file:line results that the UI renders as clickable links.

**Purity:** Pure

**Why not just `shell rg`:** Structured output with consistent formatting. Configurable defaults (context lines, max results). Automatic exclusion of gitignored and binary files. The UI can parse results into clickable file:line links.

**Return format:**

```
Found 7 matches for "ValidateToken" in 3 files:

internal/auth/service.go:
  22│ func (s *AuthService) ValidateToken(token string) (*Claims, error) {
  23│     claims, err := jwt.Parse(token, s.keyFunc)

internal/auth/middleware.go:
  25│         claims, err := authService.ValidateToken(token)

internal/auth/service_test.go:
  45│ func TestValidateToken_Expired(t *testing.T) {
  46│     claims, err := svc.ValidateToken(expiredToken)
  55│ func TestValidateToken_Invalid(t *testing.T) {
  56│     _, err := svc.ValidateToken("garbage")
  67│ func TestValidateToken_Valid(t *testing.T) {
  68│     claims, err := svc.ValidateToken(validToken)
```

**Error cases:**

- Pattern invalid (bad regex) → "Error: invalid regex pattern: {details}"
- No matches → "No matches found for '{pattern}' in {scope}.\nTry a broader pattern or check spelling."
- Output exceeds limit → Truncated with: "[Output truncated — showing first {N} matches of {total}. Narrow the search with a more specific pattern or use 'path' to limit scope.]"

**Implementation notes:**

- Shell out to `rg` (ripgrep) binary. Require ripgrep installed (document in prerequisites). Ripgrep is fast and handles gitignore, binary detection, and Unicode correctly.
- Default flags: `--line-number --with-filename --color never --max-count {max_results}`.
- Add `--glob {include}` when the `include` parameter is provided.
- Add `--context {context_lines}` (default 2).
- Working directory is project root. The `path` parameter is passed directly to ripgrep as the search path.
- Per-tool truncation default: 100 matches.

### search_semantic

**Purpose:** RAG-based semantic search against the code intelligence layer ([[04-code-intelligence-and-rag]]). This is the agent's on-demand access to the full codebase knowledge — the reactive complement to proactive context assembly ([[06-context-assembly]]).

**Purity:** Pure

**Why not just `search_text`:** Finds code by meaning, not text patterns. "Authentication middleware" matches functions that handle auth even if they don't contain the word "authentication." Understands descriptions, signatures, and relationships that the RAG pipeline captured during indexing.

**Return format:**

```
Found 5 relevant results for "authentication middleware":

1. internal/auth/middleware.go — AuthMiddleware (score: 0.87)
   Authentication middleware that validates JWT tokens on incoming requests.
   Lines 15-48 | Calls: ValidateToken, ExtractClaims | Called by: SetupRoutes

2. internal/auth/service.go — ValidateToken (score: 0.81)
   Validates a JWT token and returns the associated user claims.
   Lines 22-45 | Calls: jwt.Parse | Called by: AuthMiddleware, RefreshHandler

3. internal/server/routes.go — SetupRoutes (score: 0.72)
   Registers all HTTP routes with middleware chains.
   Lines 10-35 | Calls: AuthMiddleware, RateLimitMiddleware
```

**Error cases:**

- RAG index not built → "Error: code index not found for this project. Run 'yard index' to build it."
- No results above threshold → "No relevant results found for '{query}'. The codebase may not contain code related to this concept, or try rephrasing the query."
- Embedding service unavailable → "Error: embedding service unavailable (localhost:8081). Ensure the embeddings Docker container is running."

**Implementation notes:**

- Delegates to the `Searcher` from [[04-code-intelligence-and-rag]].
- Query is prefixed with the nomic-embed-code retrieval prefix: `"Represent this query for searching relevant code: "`.
- Uses multi-query expansion if the query has multiple concepts (determined by the searcher, not by this tool).
- One-hop call graph expansion is included — direct hits pull in their callers/callees per the searcher's budget allocation (60% direct, 40% hops).
- Results include: file path, function/type name, description, similarity score, line range, call relationships.
- Default `max_results: 10`. Applied after expansion and re-ranking.
- The `language` filter maps to a metadata filter on the LanceDB query.

### git_status

**Purpose:** Current git state — branch, staged/unstaged/untracked files, recent commits. Structured output that the UI renders as a status panel.

**Purity:** Pure

**Why not just `shell git status`:** Structured, consistent output. Combines `git status`, `git branch`, and `git log` into a single call. Parsed into a clean format.

**Return format:**

```
Branch: feature/auth-refactor
Tracking: origin/feature/auth-refactor (ahead 2, behind 0)

Staged:
  modified: internal/auth/middleware.go
  new file: internal/auth/claims.go

Unstaged:
  modified: internal/auth/service.go

Untracked:
  internal/auth/service_test.go

Recent commits (last 5):
  abc1234 Fix token expiry check in AuthMiddleware (2 hours ago)
  def5678 Add rate limiting to API gateway (5 hours ago)
  789abcd Update Go dependencies (1 day ago)
  bcd2345 Refactor provider interface (2 days ago)
  efg6789 Initial agent loop skeleton (3 days ago)
```

**Error cases:**

- Not a git repository → "Error: '{project_root}' is not a git repository."
- Git binary not found → "Error: git command not found. Ensure git is installed and on PATH."

**Implementation notes:**

- Shell out to the `git` binary (not go-git — per project conventions, see [[02-tech-stack-decisions]]).
- Three commands: `git status --porcelain=v2 --branch`, `git log --oneline -N`, `git rev-parse --abbrev-ref HEAD`.
- Parse porcelain v2 format for structured status output.
- The `recent_commits` parameter controls the `-N` flag on `git log` (default 5).

### git_diff

**Purpose:** Show diffs — working tree changes, staged changes, or between two refs.

**Purity:** Pure

**Why not just `shell git diff`:** Consistent flag handling and structured output. The UI renders diffs with syntax highlighting and file-level collapsing.

**Return format:**

```
Diff (working tree vs HEAD):

--- a/internal/auth/middleware.go
+++ b/internal/auth/middleware.go
@@ -17,3 +17,5 @@
         token := r.Header.Get("Authorization")
-        if token == "" {
+        token = strings.TrimPrefix(token, "Bearer ")
+        if token == "" || token == r.Header.Get("Authorization") {
             http.Error(w, "missing auth token", http.StatusUnauthorized)

--- a/internal/auth/service.go
+++ b/internal/auth/service.go
@@ -30,1 +30,1 @@
-    return claims, nil
+    return claims, s.validateExpiry(claims)

2 files changed, 3 insertions(+), 2 deletions(-)
```

**Error cases:**

- Invalid ref → "Error: unknown revision '{ref}'. Use 'git_status' to see available branches and recent commits."
- No diff (clean tree) → "No changes."
- Output exceeds limit → Truncated with: "[Diff truncated — showing first {N} lines. Use 'path' parameter to diff a specific file.]"

**Implementation notes:**

- Shell out to `git diff` with appropriate flags:
    - Working tree: `git diff`
    - Staged: `git diff --cached`
    - Against ref: `git diff {ref}`
    - Between refs: `git diff {ref}..{ref2}`
    - Path-limited: append `-- {path}`
- Add `--stat` summary at the end (files changed, insertions, deletions).
- Per-tool truncation default: 300 lines of diff output.

### shell

**Purpose:** Execute arbitrary shell commands. The escape hatch — anything the specialized tools don't cover (running tests, builds, linting, installing dependencies, complex git operations).

**Purity:** Mutating

**Why have specialized tools at all:** `shell` provides no structure, no guardrails, and no token efficiency. `file_edit` is 10x more token-efficient than `shell sed`. `search_text` handles gitignore and binary detection automatically. `git_status` combines three git commands with structured parsing. Specialized tools exist to make the common paths better.

**Return format:**

```
$ go test ./internal/auth/...
Exit code: 1

STDOUT:
--- FAIL: TestValidateToken_Expired (0.00s)
    service_test.go:48: expected ErrTokenExpired, got nil
FAIL    github.com/user/project/internal/auth   0.015s

STDERR:
(empty)
```

The output includes the command as invoked, the exit code, and clearly labeled stdout/stderr sections.

**Error cases:**

- Denylist match → "Error: command rejected by safety denylist.\nMatched pattern: 'rm -rf /'\nThis pattern is blocked to prevent catastrophic mistakes."
- Timeout → "Error: command timed out after {timeout}s.\nPartial output:\n{captured_output}\n\nConsider increasing the timeout or breaking the operation into smaller steps."
- Command not found → "Error: command not found: {cmd}\nCheck that the tool is installed and available on PATH."
- Cancelled → "Command cancelled.\nPartial output:\n{captured_output}"

**Implementation notes:**

- Execute via `/bin/sh -c "{command}"` (not direct exec — the LLM writes shell syntax).
- Capture stdout and stderr separately.
- Use `os/exec.CommandContext` with the turn's context for cancellation.
- Timeout: use the `timeout` parameter if provided, otherwise the tool-specific default (120s), capped by the context timeout.
- **Process group handling:** Start the process in its own process group (`Setpgid: true`). On cancellation, send SIGTERM to the process group (kills child processes). After 5 seconds, escalate to SIGKILL.
- **Denylist:** Before execution, check the command string against configured denylist patterns. Patterns are simple substring matches (not regex). The default denylist is intentionally minimal — this is a personal tool.
- Working directory: project root.
- Environment: inherit the parent process's environment. No sandboxing beyond the denylist.

### brain_read

**Purpose:** Read a document from the project brain (Obsidian vault). Returns full markdown content including frontmatter. Optionally includes backlinks — documents that reference the target.

**Purity:** Pure

**Return format:**

```
Document: architecture/provider-design.md
Title: Provider Architecture Decisions

---
created: 2026-03-27
author: agent
tags: [architecture, provider, oauth]
---

# Provider Architecture Decisions

The provider layer abstracts over three LLM backends...

[full document content]

Outgoing links: [[tech-stack-decisions]], [[error-handling]]
Backlinks: [[agent-loop-design]], [[context-assembly-decisions]]
```

When `include_backlinks` is false, the "Backlinks" line is omitted.

**Error cases:**

- Document not found → "Error: brain document not found: {path}\nAvailable documents in {parent_dir}/: {listing}"
- Historical REST-path error case → "Error: Obsidian REST API not reachable at {api_url}...". Current runtime brain tooling uses the MCP/vault backend instead.

**Implementation notes:**

- Historical note: the original plan delegated to `ObsidianClient` over the Local REST API. Current runtime brain reads use the MCP/vault backend.
- Backlinks are retrieved via a second API call or from the SQLite `brain_links` table (target path lookup).
- Outgoing wikilinks are parsed from the document content (regex for `[[...]]`).

### brain_write

**Purpose:** Create or overwrite a brain document. The agent writes Obsidian-native markdown — frontmatter, wikilinks, tags. Used for capturing durable project knowledge.

**Purity:** Mutating

**Return format:**

```
Created: debugging/lancedb-nil-slice.md (45 lines)
Tags: #debugging #cgo #lancedb
Links: [[tech-stack-decisions]], [[rag-pipeline-audit]], [[error-handling]]
```

For overwrites: "Updated: {path} ({N} lines, was {M} lines)"

**Error cases:**

- Obsidian API unreachable → same as brain_read
- Write failed → "Error: failed to write brain document: {details}"

**Implementation notes:**

- Delegates to `ObsidianClient` PUT request.
- Historical note: the original v0.1 plan wrote through the Obsidian REST API only. Current runtime note mutation is MCP/vault-backed; semantic/index-backed brain updates remain future work.
- Parses the written content to extract tags, wikilinks, and frontmatter for the confirmation output.
- Creates parent directories in the vault if needed.

### brain_update

**Purpose:** Surgically update an existing brain document — append, prepend, or replace a specific section. More efficient than full overwrite for incremental additions (e.g., adding an entry to a debugging journal).

**Purity:** Mutating

**Return format:**

```
Updated: debugging/lancedb-nil-slice.md
Operation: append
Added 12 lines (was 45, now 57)
```

For `replace_section`:

```
Updated: debugging/lancedb-nil-slice.md
Operation: replace_section "## Workaround"
Section: 8 lines replaced with 12 lines
```

**Error cases:**

- Document not found → same as brain_read
- Section not found (for replace_section) → "Error: section '## Workaround' not found in {path}.\nAvailable sections: ## Problem, ## Root Cause, ## Impact, ## Related"
- Obsidian API unreachable → same as brain_read

**Implementation notes:**

- Read the document via `ObsidianClient`, apply the operation in Go, write back.
- For `replace_section`: parse the document into heading-delimited sections. Find the target heading (exact match on heading text). Replace everything from that heading to the next heading of equal or higher level (or end of file).
- Historical note: the original v0.1 plan used an Obsidian REST read-modify-write flow. Current runtime note mutation is MCP/vault-backed; re-embedding and graph updates remain future work.

### brain_search

**Purpose:** Search the project brain for relevant documents. Historical v0.1 planning text below describes keyword search via Obsidian REST; current runtime uses MCP/vault-backed keyword search, while semantic/index-backed brain retrieval is still future work.

**Purity:** Pure

**Return format:**

```
Found 3 results for "authentication" (mode: keyword):

1. architecture/auth-architecture.md (keyword match)
   "The auth system uses JWT tokens validated by middleware..."
   Tags: #architecture #auth
   Links: [[provider-design]], [[error-handling]]

2. conventions/error-handling.md (keyword match)
   "Auth errors should return structured JSON, not plain text..."
   Tags: #convention

3. sessions/2026-03-28-auth-refactor.md (keyword match)
   "Refactored auth middleware to separate validation from refresh..."
   Tags: #session
```

**Error cases:**

- No results → "No brain documents found for '{query}'. The brain may not contain relevant knowledge yet."
- Historical REST-path error case → "Error: brain search unavailable...". Current runtime availability depends on the MCP/vault backend instead.
- `mode=semantic` or `mode=auto` → return a note that semantic search is not yet available in v0.1, then continue with keyword search

**Implementation notes:**

- Historical plan: delegate keyword mode to Obsidian's search endpoint. Current runtime performs keyword-backed search through the MCP/vault backend.
- **Semantic / auto modes in v0.1:** Return guidance that semantic search is planned for v0.2, then fall back to the keyword-search path.
- No internal brain vector index or wikilink-graph traversal is used in v0.1.

---

## Safety & Sandboxing

### Project Root Enforcement

All file tools (`file_read`, `file_write`, `file_edit`) resolve paths relative to the project root and reject any resolved path that escapes the project directory.

Implementation: `filepath.Join(projectRoot, path)` followed by a check that the result starts with `projectRoot + "/"`. This catches `..` traversal, absolute paths, and symlink escapes (resolve symlinks before the prefix check).

The `search_text` tool inherits this from ripgrep's working directory (set to project root) and any `path` parameter is relative.

The `shell` tool does **not** enforce project root — it executes arbitrary commands and could access any path the process has permissions for. This is intentional. The shell is the escape hatch; constraining it defeats its purpose. The denylist handles catastrophic mistakes; the developer handles everything else.

### Shell Denylist

Configurable patterns that reject shell commands before execution. Substring matching (case-sensitive).

Default denylist:

```yaml
shell_denylist:
  - "rm -rf /"
  - "git push --force"
```

This is intentionally minimal. sodoryard is a personal tool running as the developer's user. The agent should be able to run builds, tests, installs, and routine git operations without friction. The denylist catches catastrophic mistakes — commands that are essentially never intentional in a development context.

The developer can extend the denylist in config. No patterns are hardcoded beyond the defaults.

### Brain Vault Enforcement

Brain tools operate within the configured vault path only. The historical `ObsidianClient` path also enforced vault-root bounds. Current runtime should preserve the same vault-root safety invariant through the MCP/vault backend.

### No Network Sandboxing

sodoryard runs as the developer's user with full network access. There is no attempt to restrict the agent's network operations. The shell tool can `curl`, `wget`, `go get`, `npm install`, etc. This is a personal tool for a developer who understands what the agent is doing — the observability layer (tool execution logs, web UI tool call visualization) provides visibility, not restriction.

---

## Future Tools

The tool interface is designed so these fit naturally when added. Each would implement the same `Tool` interface, register in the same registry, and participate in the same purity-based dispatch.

### search_web

Web search for documentation, Stack Overflow, API references. Returns structured results with titles, snippets, and URLs. **Purity:** Pure. The implementation would likely wrap a search API (SearXNG self-hosted, or a commercial API).

### mcp_call

Invoke a tool from an MCP (Model Context Protocol) server. The MCP client discovers tools from connected servers; `mcp_call` bridges the agent to those tools. **Purity:** Depends on the target tool — the MCP tool definition includes metadata about side effects. The implementation requires MCP client infrastructure (connection management, tool discovery, protocol handling).

### delegate

Spawn a sub-agent for a focused sub-task. Inspired by Hermes's delegation pattern. The parent agent creates a child conversation with a scoped system prompt and a subset of tools. The child works on a focused objective and returns a summary. **Purity:** Mutating (child may execute mutating tools). This is architecturally significant — it requires the agent loop to support recursive invocation.

---

## Configuration

Tool-related configuration, consolidated from [[05-agent-loop]] and [[09-project-brain]]:

```yaml
agent:
  # Global tool settings
  tool_output_max_tokens: 50000       # Default truncation limit per tool result

  # Shell tool
  shell_timeout_seconds: 120          # Default shell command timeout
  shell_denylist:                     # Patterns rejected before execution
    - "rm -rf /"
    - "git push --force"

tools:
  file_read:
    max_lines: 500                    # Per-tool truncation (lines)
  file_write:
    diff_preview_lines: 20            # Lines of diff to show in result
  file_edit:
    not_found_preview_lines: 30       # Lines of file to show on "not found" error
  search_text:
    max_results: 50                   # Maximum ripgrep matches
    context_lines: 2                  # Default context lines around matches
  search_semantic:
    max_results: 10                   # Default RAG results
    include_call_graph: true          # One-hop call graph expansion
  git_status:
    recent_commits: 5                 # Default commit count
  git_diff:
    max_lines: 300                    # Diff output truncation
  shell:
    timeout_seconds: 120              # Override per-tool (can also be set per-call)

brain:
  enabled: true
  vault_path: ~/obsidian-vaults/sodoryard-brain
  obsidian_api_url: http://localhost:27124
  obsidian_api_key: "your-api-key-here"
  # v0.2+ smart-retrieval fields (reactive keyword tools are the v0.1 scope)
  max_brain_tokens: 8000
  brain_relevance_threshold: 0.30
  include_graph_hops: true
  graph_hop_depth: 1
```

---

## Epic Mapping

The tool system decomposes into 6 implementation epics. Each epic can be implemented independently with clear dependencies:

|Epic|Scope|Dependencies|
|---|---|---|
|01: Tool Types & Interface|`Tool` interface, `ToolPurity`, `Registry`, JSON Schema generation, input validation|None — this is the foundation|
|02: File Tools|`file_read`, `file_write`, `file_edit`. Project root enforcement, diff generation, output truncation, error enrichment|Epic 01|
|03: Search Tools|`search_text` (ripgrep wrapper), `search_semantic` (RAG searcher bridge)|Epic 01, [[04-code-intelligence-and-rag]] searcher|
|04: Git Tools|`git_status`, `git_diff`. Shell out to git binary, structured output parsing|Epic 01|
|05: Shell Tool|`shell`. Process group management, timeout, SIGTERM/SIGKILL escalation, denylist, stdout/stderr capture|Epic 01|
|06: Brain Tools & Legacy REST Plan|Historical `ObsidianClient` HTTP wrapper plus brain tools. Current runtime uses MCP/vault-backed keyword brain tooling; semantic/index-backed expansion remains deferred.|Epic 01, [[09-project-brain]] current brain contract|

Epic 01 ships first. Epics 02-06 can proceed in parallel once 01 is complete. Epic 03 (`search_semantic`) has an external dependency on the RAG searcher from [[04-code-intelligence-and-rag]]. Epic 06's Obsidian REST dependency is historical planning context, not the current runtime requirement.

---

## Dependencies

- [[05-agent-loop]] — Primary consumer. Dispatches tool calls using purity classification. Manages iteration, error recovery (three-tier), output truncation policy, and cancellation propagation. The agent loop owns the dispatch loop; the tool system owns the implementations.
- [[04-code-intelligence-and-rag]] — `search_semantic` delegates to the RAG searcher. Multi-query expansion, hit-count re-ranking, and one-hop call graph expansion are searcher capabilities, not tool capabilities.
- [[09-project-brain]] — Brain tool specifications originate there. Current runtime brain tooling and proactive retrieval are MCP/vault-backed; older ObsidianClient references in this spec are historical.
- [[08-data-model]] — `tool_executions` table records every tool dispatch (tool_name, input JSON, output_size, duration_ms, success/failure). `role=tool` messages store the full tool result content with `tool_use_id` linkage.
- [[03-provider-architecture]] — Tool definitions from `Registry.ToolDefinitions()` are serialized into the provider request's `tools` field. Provider-specific format translation (Anthropic `input_schema` vs OpenAI `parameters`) happens in the provider layer, not here.
- [[06-context-assembly]] — `search_semantic` is the reactive fallback when proactive context assembly is insufficient. The context assembly report tracks `AgentUsedSearchTool` as a quality signal.

---

## What Ports from topham

Very little. topham's pipeline model made single-shot LLM calls per phase with no tool calling — there is no tool system to port.

**Concepts carried forward:**

- Shell execution patterns (process group handling, timeout, signal escalation) from topham's shell runner
- Git command construction from topham's git integration
- Ripgrep invocation patterns from topham's text search

**Net-new for sodoryard:**

- The entire `Tool` interface and registry
- Purity classification and dispatch logic
- All 12 tool implementations
- JSON Schema definitions
- Input validation
- Error enrichment
- Output truncation with tool-specific guidance
- Brain tools and Obsidian client

---

## Open Questions

- **Validation library choice.** `santhosh-tekuri/jsonschema` (pure Go, spec-compliant) vs `xeipuuv/gojsonschema` (more mature, larger dependency). Either works. Decide at implementation time.
- **Diff library.** `sergi/go-diff` for unified diffs in `file_write` and `file_edit` results. Alternatively, shell out to `diff -u`. The Go library is cleaner; shelling out is simpler. Either works for v0.1.
- **Ripgrep as a hard dependency.** The `search_text` tool requires `rg` on PATH. This is documented in prerequisites. Alternative: use Go's `regexp` package with `filepath.Walk` — much slower but removes the dependency. Ripgrep is the right call for a tool targeting developers.
- **Tool definition caching for prompt stability.** Tool definitions are part of the LLM request and affect prompt caching. If the tool set changes between turns (e.g., brain tools become available mid-session because Obsidian started), the cached prefix is invalidated. Worth ensuring the tool set is stable for the session lifetime. Brain tool availability should be checked once at session start, not per-turn.
- **File edit conflict with concurrent shell edits.** If the LLM issues a `file_edit` and a `shell sed ...` in the same batch, the mutating sequential dispatch handles ordering. But if they target the same file, the second operation sees the first's changes. This is correct behavior — but worth noting that the LLM must be aware that mutating operations execute in order.

---

## References

- [[05-agent-loop]] — Tool dispatch, purity classification, error recovery tiers, output handling
- [[09-project-brain]] — Brain tool specifications, Obsidian REST API integration
- [[08-data-model]] — tool_executions table, role=tool message format
- [[04-code-intelligence-and-rag]] — RAG searcher that search_semantic delegates to
- [[03-provider-architecture]] — Tool definition serialization into provider requests
- [[06-context-assembly]] — search_semantic as reactive fallback, AgentUsedSearchTool quality metric
- Anthropic tool use: https://docs.anthropic.com/en/docs/build-with-claude/tool-use
- OpenAI function calling: https://platform.openai.com/docs/guides/function-calling
- Hermes Agent tools: `tools/` directory in hermes-agent repo (reference for tool patterns)
- Obsidian Local REST API: https://github.com/coddingtonbear/obsidian-local-rest-api
- ripgrep: https://github.com/BurntSushi/ripgrep