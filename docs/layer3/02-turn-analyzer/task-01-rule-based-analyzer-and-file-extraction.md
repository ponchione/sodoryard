# Task 01: RuleBasedAnalyzer Struct and File Reference Extraction

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Create the `RuleBasedAnalyzer` struct that implements the `TurnAnalyzer` interface from Epic 01. Implement the first signal extraction rule: file reference detection. The analyzer uses regex to detect file paths with extensions (e.g., `internal/auth/middleware.go`, `./config.yaml`) and Go-convention directory references without extensions (e.g., `internal/auth/`, `pkg/server`). Detected paths populate `ContextNeeds.ExplicitFiles` and each match produces a `Signal{Type: "file_ref"}` trace entry.

## Acceptance Criteria

- [ ] `RuleBasedAnalyzer` struct defined, implementing the `TurnAnalyzer` interface
- [ ] `AnalyzeTurn(message string, recentHistory []Message) *ContextNeeds` method implemented with file reference extraction as the first rule
- [ ] Regex matches paths with file extensions: `internal/auth/middleware.go`, `./config.yaml`, `cmd/sirtopham/main.go`
- [ ] Regex matches Go-convention directory references: `internal/auth/`, `pkg/server`, `cmd/sirtopham`
- [ ] Matched paths populate `ContextNeeds.ExplicitFiles`
- [ ] Each match produces a `Signal{Type: "file_ref", Source: <matched text>, Value: <extracted path>}`
- [ ] Signal extraction runs in documented priority order (file references first)
- [ ] Package compiles with no errors
