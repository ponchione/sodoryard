package operator

import (
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
)

type RuntimeWarning struct {
	Message string
}

type RuntimeStatus struct {
	ProjectRoot         string
	ProjectName         string
	Provider            string
	Model               string
	AuthStatus          string
	CodeIndex           RuntimeIndexStatus
	BrainIndex          RuntimeIndexStatus
	LocalServicesStatus string
	ActiveChains        int
	Warnings            []RuntimeWarning
}

type RuntimeIndexStatus struct {
	Status            string
	LastIndexedAt     string
	LastIndexedCommit string
	StaleSince        string
	StaleReason       string
}

type ChatMessage struct {
	Role      string
	Content   string
	CreatedAt string
}

type ChatTurnRequest struct {
	ConversationID string
	Message        string
}

type ChatTurnResult struct {
	ConversationID string
	Provider       string
	Model          string
	Messages       []ChatMessage
	InputTokens    int
	OutputTokens   int
	StopReason     string
}

type StepSummary struct {
	ID          string
	SequenceNum int
	Role        string
	Status      string
	Verdict     string
	ReceiptPath string
	TokensUsed  int
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type ChainSummary struct {
	ID          string
	Status      string
	SourceTask  string
	SourceSpecs []string
	TotalSteps  int
	TotalTokens int
	StartedAt   time.Time
	UpdatedAt   time.Time
	CurrentStep *StepSummary
}

type ChainDetail struct {
	Chain        chain.Chain
	Steps        []chain.Step
	Receipts     []ReceiptSummary
	RecentEvents []chain.Event
}

type ChainMetricsReport struct {
	ChainID              string
	Status               string
	Health               string
	TotalSteps           int
	StepRows             int
	MaxSteps             int
	StepBudgetPct        float64
	CompletedSteps       int
	RunningSteps         int
	PendingSteps         int
	FailedSteps          int
	TotalTokens          int
	StepTokenTotal       int
	StepTurnTotal        int
	TokenBudget          int
	TokenBudgetPct       float64
	TotalDurationSecs    int
	StepDurationSecs     int
	MaxDurationSecs      int
	DurationBudgetPct    float64
	ResolverLoops        int
	MaxResolverLoops     int
	ResolverLoopPct      float64
	EventTotal           int
	OutputEvents         int
	StepFailedEvents     int
	SafetyLimitEvents    int
	ReindexStartedEvents int
	ReindexDoneEvents    int
	ProcessStartedEvents int
	ProcessExitedEvents  int
	Warnings             []RuntimeWarning
	Steps                []ChainStepMetric
}

type ChainStepMetric struct {
	SequenceNum  int
	Role         string
	Status       string
	Verdict      string
	ReceiptPath  string
	TokensUsed   int
	TurnsUsed    int
	DurationSecs int
	ExitCode     *int
	ErrorMessage string
}

type ReceiptSummary struct {
	Label string
	Step  string
	Path  string
}

type ReceiptView struct {
	ChainID string
	Step    string
	Path    string
	Content string
}

type ControlResult struct {
	ChainID        string
	PreviousStatus string
	TargetStatus   string
	Status         string
	EventType      chain.EventType
	Message        string
	Already        bool
	SignaledPIDs   []int
	Warnings       []RuntimeWarning
}

type AgentRoleSummary struct {
	Name string
}

type LaunchMode string

const (
	LaunchModeOrchestrator LaunchMode = "sir_topham_decides"
	LaunchModeConstrained  LaunchMode = "constrained_orchestration"
	LaunchModeOneStep      LaunchMode = "one_step_chain"
	LaunchModeManualRoster LaunchMode = "manual_roster"
)

type LaunchRequest struct {
	Mode             LaunchMode
	Role             string
	AllowedRoles     []string
	Roster           []string
	SourceTask       string
	SourceSpecs      []string
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
}

type LaunchPreview struct {
	Mode         LaunchMode
	Role         string
	AllowedRoles []string
	Roster       []string
	Summary      string
	CompiledTask string
	Warnings     []RuntimeWarning
}

type LaunchDraft struct {
	ID        string
	Request   LaunchRequest
	UpdatedAt string
}

type LaunchPreset struct {
	ID        string
	Name      string
	Request   LaunchRequest
	UpdatedAt string
}

type StartResult struct {
	ChainID string
	Status  string
	Preview LaunchPreview
}
