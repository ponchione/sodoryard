# Task 06: Schema — Indexes

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04

---

## Description

Add all indexes to `schema.sql` as specified in doc 08. These support the key query patterns (conversation listing, history reconstruction, analytics). FTS5 virtual tables are self-indexed and do not require explicit indexes.

## Complete Index List (from doc 08)

```sql
-- conversations
CREATE INDEX idx_conversations_project ON conversations(project_id, updated_at DESC);

-- messages
CREATE INDEX idx_messages_conversation ON messages(conversation_id, is_compressed, sequence);
CREATE INDEX idx_messages_turn ON messages(conversation_id, turn_number);

-- tool_executions
CREATE INDEX idx_tool_exec_conversation ON tool_executions(conversation_id, turn_number);
CREATE INDEX idx_tool_exec_name ON tool_executions(tool_name);

-- sub_calls
CREATE INDEX idx_sub_calls_conversation ON sub_calls(conversation_id, turn_number);
CREATE INDEX idx_sub_calls_created ON sub_calls(created_at);
CREATE INDEX idx_sub_calls_purpose ON sub_calls(purpose);

-- context_reports
CREATE INDEX idx_context_reports_quality ON context_reports(agent_used_search_tool);

-- brain_documents
CREATE INDEX idx_brain_docs_project ON brain_documents(project_id);

-- brain_links
CREATE INDEX idx_brain_links_source ON brain_links(project_id, source_path);
CREATE INDEX idx_brain_links_target ON brain_links(project_id, target_path);

-- index_state
CREATE INDEX idx_index_state_project ON index_state(project_id);
```

## Acceptance Criteria

- [ ] `idx_conversations_project` on `conversations(project_id, updated_at DESC)`
- [ ] `idx_messages_conversation` on `messages(conversation_id, is_compressed, sequence)`
- [ ] `idx_messages_turn` on `messages(conversation_id, turn_number)`
- [ ] `idx_tool_exec_conversation` on `tool_executions(conversation_id, turn_number)`
- [ ] `idx_tool_exec_name` on `tool_executions(tool_name)`
- [ ] `idx_sub_calls_conversation` on `sub_calls(conversation_id, turn_number)`
- [ ] `idx_sub_calls_created` on `sub_calls(created_at)`
- [ ] `idx_sub_calls_purpose` on `sub_calls(purpose)`
- [ ] `idx_context_reports_quality` on `context_reports(agent_used_search_tool)`
- [ ] `idx_brain_docs_project` on `brain_documents(project_id)`
- [ ] `idx_brain_links_source` on `brain_links(project_id, source_path)`
- [ ] `idx_brain_links_target` on `brain_links(project_id, target_path)`
- [ ] `idx_index_state_project` on `index_state(project_id)`
- [ ] Schema still executes cleanly with all 13 indexes added
