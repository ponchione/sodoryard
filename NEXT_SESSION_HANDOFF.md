Fresh-session handoff: Spec 11 (tool result normalization) fully complete

What was completed this session (1 commit, pushed)
- `a0a9407` — `feat(tool): add Phase 2 history compression pipeline (spec 11)`

Current state
- Spec 11 fully complete: Phase 1 (write-time normalization, b30af71) + Phase 2 (history compression, a0a9407)
- Layer 4 tool system fully complete (Epics 01-05 + wiring + normalization + compression)
- Layer 5 Epic 02 (Conversation Manager) fully complete
- Anthropic provider fully implemented (Complete, Stream, SSE parsing, retry, errors)
- 166 tests in internal/tool/ (was 137 before this session)
- All pushed to origin/main

Phase 2 history compression — what's implemented
- `internal/tool/historycompress.go`: HistoryCompressor with 4 transforms:
  1. Duplicate result elision — older reads of the same file → pointer to latest
  2. Stale result summarization — file_read/search_text older than N turns → one-liner
  3. Line-number stripping — file_read prefixes removed from historical results
  4. JSON re-minification — idempotent json.Compact on historical tool results
- `internal/agent/prompt.go`: buildMessages applies compression when CompressHistoricalResults=true
- `internal/config/config.go`: 4 new AgentConfig fields with defaults (all enabled, summarize after 10 turns)
- `internal/agent/loop.go`: all 3 PromptConfig constructions pass compression settings through

Key design decisions
- HistoryCompressor takes CurrentTurn and SummarizeAfterTurns — no dependency on config package
- Transforms operate on HistoryMessage (thin struct with Role, Content, ToolName, TurnNumber)
- Prompt builder converts db.Message ↔ HistoryMessage at the integration boundary
- Elision wins over summarization when both would apply (a file re-read 20 turns ago)
- Only file_read and git_diff are deduplicable (shell/search may vary between runs)
- Dedup tracking uses (toolName, filePath) keys extracted from file_read header format

Validation state
- `go test -race -tags sqlite_fts5 ./internal/tool/... ./internal/agent/... ./internal/config/... ./internal/provider/... ./internal/db/... ./internal/conversation/...` green
- `go vet ./internal/tool/... ./internal/agent/... ./internal/config/...` clean

What is NOT implemented
- Epic 06: Obsidian Client & Brain Tools (deferred — v0.2 scope per docs)
- Agent loop refactor for batch dispatch (adapter bridges the gap)
- Streaming shell output (future Layer 5/7 concern)
- Wire Manager/AgentLoop into actual startup (cmd/main.go serve command)
- REST API layer (internal/server/ is a stub)

Next natural slices
a. Wire Manager + AgentLoop into the serve command (cmd/main.go) — connect the built pieces
b. REST API for conversations (Layer 6 Epic 02) — expose CRUD + streaming
c. Agent loop refactor for native batch dispatch (replace adapter)
d. Layer 4 Epic 06: Obsidian Client & Brain Tools (if v0.1 scope)

Suggested commands
- `git log --oneline -15`
- `go test -race -tags sqlite_fts5 ./internal/tool/... ./internal/agent/... ./internal/config/...`
