# Layer 4 — Epic 02: File Tools

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 4 Epic 01: Tool Interface, Registry & Executor

**Architecture Refs:**
- [[05-agent-loop]] §Tool Set — file_read, file_write, file_edit definitions, purity classifications
- [[05-agent-loop]] §Error Recovery — enriched error messages (e.g., listing available files on "not found")

---

## What This Epic Covers

Three file system tools that implement the `Tool` interface from Epic 01:

**`file_read` (Pure):** Read file contents with optional line range (`line_start`, `line_end`). Returns content with line numbers prepended. The structured output enables the UI to show syntax-highlighted code at exact file/line locations. Handles edge cases: file not found (list available files in the directory), binary file detection, empty files.

**`file_write` (Mutating):** Write or overwrite a file with provided content. Creates parent directories if they don't exist. Returns confirmation and a diff preview (first N lines of the unified diff against the previous version, or "[new file]" for creation). Refuses writes outside the project root — path traversal with `..` is rejected.

**`file_edit` (Mutating):** Search-and-replace within a file. Takes `path`, `old_str` (unique string to find), and `new_str` (replacement). Validates that `old_str` appears exactly once in the file — fails with guidance if zero matches (typo?) or multiple matches (not unique enough). Returns the unified diff of the change. Dramatically more token-efficient than rewriting entire files.

---

## Definition of Done

### file_read
- [ ] Implements `Tool` interface with purity `Pure`
- [ ] Parameters: `path` (required), `line_start` (optional), `line_end` (optional)
- [ ] Returns file content with line numbers (e.g., `   15\t    func ValidateToken(...)`)
- [ ] Line range support: `line_start` and `line_end` are 1-indexed, inclusive
- [ ] If `line_start` omitted, starts from line 1; if `line_end` omitted, reads to end of file
- [ ] File not found error includes listing of files in the same directory (enriched error per [[05-agent-loop]] §Error Recovery)
- [ ] Path traversal outside project root rejected with clear error
- [ ] Binary file detection: if file contains null bytes in the first 8KB, return "Binary file, cannot display" instead of raw content
- [ ] JSON Schema accurately describes all parameters with types and descriptions
- [ ] Unit tests: normal read, line range, file not found with directory listing, path traversal rejection, binary detection

### file_write
- [ ] Implements `Tool` interface with purity `Mutating`
- [ ] Parameters: `path` (required), `content` (required)
- [ ] Creates parent directories (equivalent to `mkdir -p`) if they don't exist
- [ ] On overwrite: generates unified diff against previous content, returns first 50 lines of diff
- [ ] On new file creation: returns "[new file created]" with byte count
- [ ] Path traversal outside project root rejected with clear error
- [ ] JSON Schema accurately describes all parameters
- [ ] Unit tests: new file creation with nested directories, overwrite with diff, path traversal rejection

### file_edit
- [ ] Implements `Tool` interface with purity `Mutating`
- [ ] Parameters: `path` (required), `old_str` (required), `new_str` (required)
- [ ] Validates `old_str` appears exactly once in the file
- [ ] Zero matches: error with "String not found in file. Check for typos or whitespace differences."
- [ ] Multiple matches: error with "String appears N times. Provide a longer, more unique search string."
- [ ] Performs the replacement and writes the file
- [ ] Returns unified diff of the change
- [ ] File not found error includes directory listing (enriched error)
- [ ] Path traversal rejection
- [ ] JSON Schema accurately describes all parameters
- [ ] Unit tests: successful edit with diff, zero matches, multiple matches, file not found, path traversal

### All three tools
- [ ] Registered in the registry (registration happens in a central `RegisterAll` or init function)
- [ ] All paths resolved relative to `projectRoot` passed by the executor

---

## Key Design Notes

**Diff generation:** Use Go's standard diff library or a lightweight unified diff implementation. The diff is for the LLM and UI — it doesn't need to be `git diff` compatible, just human-readable.

**Line numbering format:** Match the format commonly used by Claude Code and similar tools: right-aligned line numbers with tab separator. This helps the LLM reference specific lines in subsequent tool calls.

**Path resolution:** All paths are resolved relative to `projectRoot`. Absolute paths are rejected. Paths containing `..` that would escape the project root are rejected. This is the tool-level sandboxing mentioned in Epic 01.

---

## Consumed By

- [[layer4-epic01-tool-interface]] — registered in the tool registry
- Layer 5 (Agent Loop) — dispatched via the executor
