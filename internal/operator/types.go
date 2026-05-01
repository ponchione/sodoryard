package operator

import (
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
)

type RuntimeWarning struct {
	Message string
}

type RuntimeStatus struct {
	ProjectRoot  string
	ProjectName  string
	Provider     string
	Model        string
	ActiveChains int
	Warnings     []RuntimeWarning
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
	RecentEvents []chain.Event
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
	LaunchModeOneStep      LaunchMode = "one_step_chain"
)

type LaunchRequest struct {
	Mode             LaunchMode
	Role             string
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
	Summary      string
	CompiledTask string
	Warnings     []RuntimeWarning
}

type StartResult struct {
	ChainID string
	Status  string
	Preview LaunchPreview
}
