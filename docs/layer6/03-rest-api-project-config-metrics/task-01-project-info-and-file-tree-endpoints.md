# Task 01: Project Info and File Tree Endpoints

**Epic:** 03 — REST API Project/Config/Metrics
**Status:** ⬚ Not started
**Dependencies:** Epic 01 (HTTP Server Foundation), Layer 0 Epic 03 (config), Layer 0 Epic 06 (schema & sqlc)

---

## Description

Implement the `GET /api/project`, `GET /api/project/tree`, and `GET /api/project/file` endpoints. The project info endpoint returns metadata about the current project from the database. The file tree endpoint walks the project directory and returns a nested JSON tree structure, respecting include/exclude globs from config. The file contents endpoint returns the raw text of a single file with metadata, with path traversal protection to prevent reading files outside the project root.

## Acceptance Criteria

- [ ] `GET /api/project` returns `{id, name, root_path, language, last_indexed_at, last_indexed_commit}` from the `projects` table
- [ ] Returns HTTP 404 with `{"error": "project not found"}` if no project is registered (user needs to run `sirtopham init`)
- [ ] `GET /api/project/tree` returns a nested JSON structure: `{name: string, type: "dir"|"file", children?: [...]}` representing the project directory tree
- [ ] Tree depth is limited (default 3 levels) and controllable via `depth` query parameter (max 10)
- [ ] Tree respects the same include/exclude glob patterns from config that the indexer uses (e.g., excludes `node_modules/`, `.git/`, `vendor/`)
- [ ] `GET /api/project/file?path=<relative_path>` returns `{path, content, language, line_count}` where `content` is the raw file text
- [ ] File endpoint rejects paths containing `..` or absolute paths — returns HTTP 400 with `{"error": "invalid path"}` for path traversal attempts
- [ ] File endpoint returns HTTP 404 if the file does not exist within the project root
- [ ] Large files (>1MB) return HTTP 413 with `{"error": "file too large"}` or are truncated with a flag indicating truncation
- [ ] Language detection is based on file extension (`.go` -> `go`, `.ts` -> `typescript`, etc.) — no content-based detection needed
- [ ] All three endpoints are registered on the server's router under `/api/project`
