package receipt

import "time"

const SchemaVersion = "yard.receipt.v1"

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
	SchemaVersion   string    `yaml:"schema_version"`
	Agent           string    `yaml:"agent"`
	Role            string    `yaml:"role"`
	ChainID         string    `yaml:"chain_id"`
	Step            int       `yaml:"step"`
	StepID          string    `yaml:"step_id"`
	Verdict         Verdict   `yaml:"verdict"`
	Timestamp       time.Time `yaml:"timestamp"`
	TurnsUsed       int       `yaml:"turns_used"`
	TokensUsed      int       `yaml:"tokens_used"`
	DurationSeconds int       `yaml:"duration_seconds"`
	ChangedFiles    []string  `yaml:"changed_files"`
	Findings        []Finding `yaml:"findings"`
	Followups       []string  `yaml:"followups"`
	Metrics         Metrics   `yaml:"metrics"`
	SchemaWarnings  []string  `yaml:"-"`
	RawBody         string    `yaml:"-"`
}

type Metrics struct {
	Turns           int `yaml:"turns"`
	InputTokens     int `yaml:"input_tokens"`
	OutputTokens    int `yaml:"output_tokens"`
	Tokens          int `yaml:"tokens"`
	DurationSeconds int `yaml:"duration_seconds"`
}

type Finding struct {
	ID              string   `yaml:"id"`
	Status          string   `yaml:"status"`
	Severity        string   `yaml:"severity"`
	Category        string   `yaml:"category"`
	File            string   `yaml:"file"`
	Line            int      `yaml:"line"`
	Summary         string   `yaml:"summary"`
	Evidence        string   `yaml:"evidence"`
	Recommendation  string   `yaml:"recommendation"`
	RequiredFix     string   `yaml:"required_fix"`
	Resolution      string   `yaml:"resolution"`
	AddressedByStep string   `yaml:"addressed_by_step"`
	FilesChanged    []string `yaml:"files_changed"`
	Validation      []string `yaml:"validation"`
}

type StepValidation struct {
	Agent   string
	ChainID string
	Step    int
}

type UsageMetrics struct {
	TurnsUsed       int
	TokensUsed      int
	DurationSeconds int
}
