# Task 01: ConversationManager CRUD Operations

**Epic:** 02 — Conversation Manager
**Status:** ⬚ Not started
**Dependencies:** L5-E01, Layer 0 Epic 04 (SQLite), Layer 0 Epic 05 (UUIDv7), Layer 0 Epic 06 (schema/sqlc)

---

## Description

Implement the `ConversationManager` struct and its core CRUD operations in `internal/conversation/manager.go`. This provides the basic lifecycle management for conversations: creating new conversations with UUIDv7 IDs, loading conversation metadata, listing conversations for a project, deleting conversations with cascade, and setting titles. All operations use sqlc-generated query functions against the `conversations` SQLite table.

## Acceptance Criteria

- [ ] `Conversation` struct defined with fields: `ID string`, `ProjectID string`, `Title *string` (nullable), `CreatedAt time.Time`, `UpdatedAt time.Time`
- [ ] `ConversationManager` struct with dependencies: sqlc queries interface, UUIDv7 generator, logger
- [ ] `NewConversationManager(queries DB, logger *slog.Logger) *ConversationManager` constructor
- [ ] `Create(projectID string, opts ...CreateOption) (*Conversation, error)` — generates UUIDv7 ID, inserts into `conversations` table with `created_at` and `updated_at` set to `now()`. `CreateOption` functional options for optional initial title
- [ ] `Get(conversationID string) (*Conversation, error)` — loads a single conversation by ID. Returns a well-typed error if not found
- [ ] `List(projectID string, limit, offset int) ([]Conversation, error)` — returns conversations ordered by `updated_at DESC` with pagination via limit/offset
- [ ] `Delete(conversationID string) error` — cascade deletes the conversation and all related records (messages, tool_executions, sub_calls, context_reports). Uses SQLite foreign key CASCADE or explicit DELETEs in a transaction
- [ ] `SetTitle(conversationID, title string) error` — updates the `title` column and `updated_at` timestamp
- [ ] All methods update `updated_at` on the conversation record when modifying it
- [ ] All database operations use sqlc-generated code, no raw SQL strings
- [ ] Package compiles with `go build ./internal/conversation/...`
