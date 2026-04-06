# Next session handoff

Date: 2026-04-06
Repo: /home/gernsback/source/sirtopham
Branch: main
State: several coherent low-risk slices are now complete, validated, and ready to commit. Checked-in default model still intentionally remains `gpt-5.4-mini`.

## Completed this session

1. Tool-output Stage 1 follow-through
- persisted tool-result references now use a structured marker/preview format
- persisted `shell` previews preserve tail/error context instead of losing the failure at the head-truncation boundary
- added an explicit `ToolOutputManager` seam so aggregate budgeting is no longer wired directly in the loop
- aggregate-budget persistence ordering is now deterministic:
  - `shell` first
  - other persistable tool classes next
  - `file_read` last
- persisted references keep a useful minimum preview budget instead of being immediately shrunk away
- added/reporting coverage for persisted-vs-inline budget outcomes

2. Conversation search / tombstone consumer hardening
- search snippets now sanitize assistant/tool JSON more reliably
- tombstone-bearing assistant/tool payloads collapse to compact summaries instead of leaking raw payload text
- title cleaning rejects tombstone markers so they do not become persisted conversation titles
- added focused manager/search/title tests for those cases

3. Codex visible-model alignment guardrail
- extracted the static visible model list into `internal/provider/codex/model_discovery.go`
- added `discoverVisibleModels(...)` against `codex app-server --listen stdio://`
- added tests for parsing the machine-readable `model/list` response
- added a non-short installed-Codex parity test to catch drift between the checked-in static list and the local visible runtime list
- production `Models()` still returns the static list; discovery is currently a validation/test helper, not runtime behavior

4. Router health hardening
- request-shape / client-side validation failures that surface as provider 400s no longer poison provider health
- auth failures still affect health and still return remediation for the default provider
- added router coverage for the non-poisoning validation case and auth/error behavior

5. Brain test determinism
- fake brain-search hits are now sorted deterministically in tests to avoid map-iteration flakes

## Validation run

Focused:
- `go test ./internal/agent -count=1`
- `go test -tags sqlite_fts5 ./internal/conversation ./internal/provider/codex ./internal/provider/router ./internal/tool -count=1`

Broad:
- `make test`

Everything passed.

## Recommended next step

Best next implementation seam is no longer more speculative Stage 1 tool-output polish.

Preferred order:
1. cancellation cleanup follow-through
2. file-edit error-contract / disambiguation hardening only if real runtime evidence shows weak model recovery
3. only then any further downstream tombstone/tool-output consumer work

## Guardrails

- do not spend the next session on generic prompt tuning
- do not wholesale port `sirtopham-handoff/` stubs
- keep checked-in default model at `gpt-5.4-mini` unless the user explicitly wants the launch switch flipped
- do not treat the Codex model-discovery helper as a mandate for runtime dynamic discovery unless drift appears again

## Local-only dirty state not suitable for commit

Keep out of the commit unless explicitly intended:
- `.brain/.obsidian/workspace.json`
- `sirtopham.yaml`

## Useful commands

- `go test ./internal/agent -count=1`
- `go test -tags sqlite_fts5 ./internal/conversation ./internal/provider/codex ./internal/provider/router ./internal/tool -count=1`
- `make test`

## Bottom line

This is a good stopping point. The current work set now includes: Stage 1 tool-output hardening with a real manager seam and deterministic persistence policy; search/title sanitation for tombstone-heavy transcripts; a Codex visible-model drift guardrail via `app-server` discovery tests; router health protection against bad request/model validation churn; and deterministic brain-tool tests. The next session should move to cancellation cleanup or evidence-driven file-edit recovery behavior rather than polishing these slices further.
