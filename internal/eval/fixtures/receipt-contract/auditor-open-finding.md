---
schema_version: yard.receipt.v1
agent: correctness-auditor
role: correctness-auditor
chain_id: eval-receipt
step: 2
step_id: eval-receipt-step-002
verdict: fix_required
timestamp: 2026-05-08T00:01:00Z
turns_used: 1
tokens_used: 64
duration_seconds: 2
findings:
  - id: FIND-correctness-001
    status: open
    severity: high
    category: correctness
    file: internal/example.go
    line: 42
    summary: Missing nil guard.
    evidence: internal/example.go:42
    recommendation: Add a nil check before dereference.
---
## Summary

The implementation has one blocking correctness issue.

## Changes

No source changes.

## Validation

- rtk make test

## Concerns

The nil case still needs a fix.

## Next Steps

Resolver should address the finding.

## Findings

### FIND-correctness-001

Severity: high
Status: open
Evidence: internal/example.go:42
Summary: Missing nil guard.
Required fix: Add a nil check before dereference.
