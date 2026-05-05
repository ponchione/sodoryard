package receipt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const happyPathReceipt = `---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---

Receipt created per request.
`

func TestParseHappyPath(t *testing.T) {
	receipt, err := Parse([]byte(happyPathReceipt))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	wantTime := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	if receipt.Agent != "correctness-auditor" || receipt.ChainID != "smoke-test-p5a" || receipt.Step != 1 || receipt.Verdict != VerdictCompleted || !receipt.Timestamp.Equal(wantTime) || receipt.TurnsUsed != 2 || receipt.TokensUsed != 0 || receipt.DurationSeconds != 0 || receipt.RawBody != "\nReceipt created per request.\n" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	_, err := Parse([]byte("no frontmatter here"))
	if !errors.Is(err, ErrMissingFrontmatter) {
		t.Fatalf("Parse error = %v, want ErrMissingFrontmatter", err)
	}
}

func TestParseMalformedYAML(t *testing.T) {
	_, err := Parse([]byte("---\nagent: [\n---\nbody"))
	if err == nil || !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("Parse error = %v, want yaml error", err)
	}
}

func TestParseMissingRequiredField(t *testing.T) {
	_, err := Parse([]byte(`---
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---
`))
	if !errors.Is(err, ErrMissingField) || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("Parse error = %v, want missing agent field", err)
	}
}

func TestParseNegativeStep(t *testing.T) {
	_, err := Parse([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: -1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---
`))
	if !errors.Is(err, ErrInvalidField) || !strings.Contains(err.Error(), "step") {
		t.Fatalf("Parse error = %v, want invalid step field", err)
	}
}

func TestParseUnknownVerdict(t *testing.T) {
	_, err := Parse([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: mystery
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---
`))
	if !errors.Is(err, ErrInvalidVerdict) || !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("Parse error = %v, want invalid verdict", err)
	}
}

func TestParseAcceptsAllSpecVerdicts(t *testing.T) {
	for _, verdict := range []Verdict{
		VerdictCompleted,
		VerdictCompletedWithConcerns,
		VerdictCompletedNoReceipt,
		VerdictFixRequired,
		VerdictBlocked,
		VerdictEscalate,
		VerdictSafetyLimit,
	} {
		t.Run(string(verdict), func(t *testing.T) {
			receipt, err := Parse([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: ` + string(verdict) + `
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---
body
`))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if receipt.Verdict != verdict {
				t.Fatalf("Verdict = %q, want %q", receipt.Verdict, verdict)
			}
		})
	}
}

func TestParseMissingBodyIsAllowed(t *testing.T) {
	receipt, err := Parse([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if receipt.RawBody != "" {
		t.Fatalf("RawBody = %q, want empty string", receipt.RawBody)
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	receipt, err := Parse([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
extra_field: ignored
---
body
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if receipt.Agent != "correctness-auditor" {
		t.Fatalf("Agent = %q, want correctness-auditor", receipt.Agent)
	}
}

func TestRewriteUsageMetricsPreservesBody(t *testing.T) {
	updated, parsed, changed, err := RewriteUsageMetrics([]byte(`---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 0
tokens_used: 0
duration_seconds: 0
extra_field: keep-me
---

Receipt created per request.
`), UsageMetrics{TurnsUsed: 3, TokensUsed: 99, DurationSeconds: 7})
	if err != nil {
		t.Fatalf("RewriteUsageMetrics returned error: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if parsed.TurnsUsed != 3 || parsed.TokensUsed != 99 || parsed.DurationSeconds != 7 {
		t.Fatalf("parsed usage = %+v, want updated usage", parsed)
	}
	text := string(updated)
	for _, want := range []string{"turns_used: 3", "tokens_used: 99", "duration_seconds: 7", "extra_field: keep-me", "\nReceipt created per request.\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated receipt = %q, want %q", text, want)
		}
	}
}
