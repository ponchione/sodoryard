package receipt

import "time"

type Verdict string

const (
	VerdictCompleted          Verdict = "completed"
	VerdictCompletedNoReceipt Verdict = "completed_no_receipt"
	VerdictBlocked            Verdict = "blocked"
	VerdictEscalate           Verdict = "escalate"
	VerdictSafetyLimit        Verdict = "safety_limit"
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
