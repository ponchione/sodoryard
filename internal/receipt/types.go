package receipt

import "time"

type Verdict string

const (
	VerdictCompleted             Verdict = "completed"
	VerdictCompletedWithConcerns Verdict = "completed_with_concerns"
	VerdictCompletedNoReceipt    Verdict = "completed_no_receipt"
	VerdictFixRequired           Verdict = "fix_required"
	VerdictBlocked               Verdict = "blocked"
	VerdictEscalate              Verdict = "escalate"
	VerdictSafetyLimit           Verdict = "safety_limit"
)

type Receipt struct {
	Agent           string    `yaml:"agent"`
	ChainID         string    `yaml:"chain_id"`
	Step            int       `yaml:"step"`
	Verdict         Verdict   `yaml:"verdict"`
	Timestamp       time.Time `yaml:"timestamp"`
	TurnsUsed       int       `yaml:"turns_used"`
	TokensUsed      int       `yaml:"tokens_used"`
	DurationSeconds int       `yaml:"duration_seconds"`
	RawBody         string    `yaml:"-"`
}

type UsageMetrics struct {
	TurnsUsed       int
	TokensUsed      int
	DurationSeconds int
}
