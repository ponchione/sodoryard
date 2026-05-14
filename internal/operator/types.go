package operator

import (
	"encoding/json"
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
	ContextWindow       int
	ReasoningEffort     string
	ModelCapabilities   ModelCapabilities
	AuthStatus          string
	CodeIndex           RuntimeIndexStatus
	BrainIndex          RuntimeIndexStatus
	LocalServicesStatus string
	ActiveChains        int
	Warnings            []RuntimeWarning
}

type ModelCapabilities struct {
	SupportsTools            bool
	SupportsThinking         bool
	SupportsReasoningEffort  bool
	SupportsStructuredOutput bool
	SupportsPromptCache      bool
	SupportsImages           bool
	SupportsToolChoice       bool
	MaxOutputTokens          int
	KnownQuirks              []string
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
	ID                string
	Status            string
	SourceTask        string
	SourceSpecs       []string
	TotalSteps        int
	TotalTokens       int
	TotalDurationSecs int
	ReceiptCount      int
	StartedAt         time.Time
	UpdatedAt         time.Time
	CurrentStep       *StepSummary
}

type ChainDetail struct {
	Chain        chain.Chain
	Steps        []chain.Step
	Receipts     []ReceiptSummary
	Approvals    []ApprovalView
	RecentEvents []chain.Event
	Timeline     []ChainTimelineItem
	Health       string
	Warnings     []RuntimeWarning
	Guardrails   ChainGuardrailDetails
	Metrics      ChainMetricsReport
}

type ChainTimelineItem struct {
	ID             string
	Source         string
	Kind           string
	Name           string
	Status         string
	TraceID        string
	SpanID         string
	ParentSpanID   string
	ConversationID string
	ChainID        string
	StepID         string
	TurnNumber     int
	Iteration      int
	StartedAt      time.Time
	EndedAt        *time.Time
	DurationMs     int64
	Attributes     map[string]any
	Error          string
	EventType      string
	EventData      string
}

type ChainGuardrailDetails struct {
	OpenFindingIDs             []string
	ClosedFindingIDs           []string
	AddressedFindingIDs        []string
	ReopenedFindingIDs         []string
	RepeatedResolverFindingIDs []string
	Findings                   []FindingLifecycleMetric
	LockHealth                 GuardrailLockHealth
	ChangedFiles               []ChangedFileManifest
	StepFacts                  []StepGuardrailFactSummary
}

type GuardrailLockHealth struct {
	Acquired          int
	Released          int
	Blocked           int
	ForceReleased     int
	ReleaseFailed     int
	HeartbeatFailed   int
	StaleReplaced     int
	UnreleasedWriters int
}

type ChangedFileManifest struct {
	StepID      string
	SequenceNum int
	Role        string
	Paths       []string
	Error       string
}

type StepGuardrailFactSummary struct {
	StepID                              string
	SequenceNum                         int
	Role                                string
	ReceiptPath                         string
	SourceMutating                      bool
	ExitCode                            int
	DurationSecs                        int
	ReceiptPresent                      bool
	SyntheticReceiptWritten             bool
	ReceiptValid                        bool
	ReceiptSchemaValid                  bool
	ReceiptStepValid                    bool
	ReceiptSectionsValid                bool
	ReceiptError                        string
	ParsedVerdict                       string
	TokensUsed                          int
	TurnsUsed                           int
	ReceiptDurationSeconds              int
	ClaimedValidationCommands           []string
	ChangedFileClaimPresent             bool
	ClaimedChangedFiles                 []string
	ChangedFileClaimMatchesManifest     bool
	ChangedFileClaimExtra               []string
	ChangedFileManifestUnclaimed        []string
	ChangedFileManifestPresent          bool
	ChangedFileManifestError            string
	ChangedFileCount                    int
	ChangedFiles                        []string
	CodeIndexStateSupported             bool
	CodeIndexStateFound                 bool
	CodeIndexDirtyMarkSupported         bool
	CodeIndexDirtyMarkAttempted         bool
	CodeIndexDirtyMarked                bool
	CodeIndexDirtyMarkError             string
	CodeIndexDirty                      bool
	CodeIndexDirtyReason                string
	CodeIndexStateError                 string
	BrainIndexStateSupported            bool
	BrainIndexStateFound                bool
	BrainIndexDirty                     bool
	BrainIndexDirtyReason               string
	BrainIndexStateError                string
	SourceWriterLockReleaseAttempted    bool
	SourceWriterLockReleased            bool
	SourceWriterLockReleaseError        string
	FindingCount                        int
	OpenFindingCount                    int
	ClosedFindingCount                  int
	AddressedFindingCount               int
	FindingIDs                          []string
	OpenFindingIDs                      []string
	ClosedFindingIDs                    []string
	AddressedIDs                        []string
	SuspiciousVerdictFindingCombination bool
	SuspiciousVerdictFindingReason      string
	RunError                            string
}

type ChainMetricsReport struct {
	ChainID                           string
	Status                            string
	Health                            string
	LaunchMode                        string
	StepMaxTurns                      int
	StepMaxTokens                     int
	HasStepMaxTurns                   bool
	HasStepMaxTokens                  bool
	TotalSteps                        int
	StepRows                          int
	MaxSteps                          int
	StepBudgetPct                     float64
	CompletedSteps                    int
	RunningSteps                      int
	PendingSteps                      int
	FailedSteps                       int
	TotalTokens                       int
	StepTokenTotal                    int
	StepTurnTotal                     int
	TokenBudget                       int
	TokenBudgetPct                    float64
	TotalDurationSecs                 int
	StepDurationSecs                  int
	MaxDurationSecs                   int
	DurationBudgetPct                 float64
	ResolverLoops                     int
	MaxResolverLoops                  int
	ResolverLoopPct                   float64
	EventTotal                        int
	OutputEvents                      int
	StepFailedEvents                  int
	ChangedFileEvents                 int
	StepGuardrailFactEvents           int
	ReceiptWarningEvents              int
	ReceiptFindingEvents              int
	FindingLifecycleFactEvents        int
	OpenFindingCount                  int
	ClosedFindingCount                int
	AddressedFindingCount             int
	OpenFindingIDs                    []string
	ClosedFindingIDs                  []string
	AddressedFindingIDs               []string
	ReopenedFindingIDs                []string
	RepeatedResolverFindingIDs        []string
	FindingLifecycle                  []FindingLifecycleMetric
	SourceWriterBlocks                int
	SourceWriterLockAcquires          int
	SourceWriterLockReleases          int
	SourceWriterLockForceReleases     int
	SourceWriterLockReleaseFailures   int
	SourceWriterLockHeartbeatFailures int
	SourceWriterLockStaleReplacements int
	SafetyLimitEvents                 int
	ReindexStartedEvents              int
	ReindexDoneEvents                 int
	ProcessStartedEvents              int
	ProcessExitedEvents               int
	Warnings                          []RuntimeWarning
	Steps                             []ChainStepMetric
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

type FindingLifecycleMetric struct {
	ID              string
	SourceRole      string
	Status          string
	Severity        string
	Evidence        string
	Summary         string
	RequiredFix     string
	Resolution      string
	FilesChanged    []string
	Validation      []string
	AddressedCount  int
	ClosedCount     int
	ReopenedCount   int
	FirstSeenStep   int
	LastUpdatedStep int
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

type ApprovalView struct {
	ID             string
	ChainID        string
	StepID         string
	ConversationID string
	TurnNumber     int
	Iteration      int
	ToolName       string
	ToolInput      json.RawMessage
	Reason         string
	RiskLevel      string
	Status         string
	CreatedAt      time.Time
	DecidedAt      *time.Time
	DecisionReason string
	DecidedBy      string
}

type ApprovalDecisionResult struct {
	Approval ApprovalView
	Message  string
}

type ProjectLockView struct {
	LockName     string
	OwnerChainID string
	OwnerStepID  string
	OwnerRole    string
	AcquiredAt   time.Time
	HeartbeatAt  time.Time
	ExpiresAt    time.Time
	Stale        bool
	MetadataJSON string
}

type ProjectLockForceReleaseResult struct {
	LockName     string
	Released     bool
	OwnerChainID string
	OwnerStepID  string
	OwnerRole    string
	Message      string
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
	TemplateID        string
	Mode              LaunchMode
	Role              string
	AllowedRoles      []string
	Roster            []string
	SourceTask        string
	SourceSpecs       []string
	MaxSteps          int
	MaxResolverLoops  int
	MaxDuration       time.Duration
	TokenBudget       int
	StepMaxTurns      int
	StepMaxTokens     int
	AllowApprovalWait bool
}

type LaunchPreview struct {
	Mode              LaunchMode
	Template          LaunchTemplate
	Role              string
	AllowedRoles      []string
	Roster            []string
	Summary           string
	CompiledTask      string
	StepMaxTurns      int
	StepMaxTokens     int
	AllowApprovalWait bool
	Warnings          []RuntimeWarning
}

type LaunchTemplate struct {
	ID              string          `json:"id"`
	Mode            LaunchMode      `json:"mode"`
	Label           string          `json:"label"`
	Description     string          `json:"description"`
	InputSchema     json.RawMessage `json:"input_schema,omitempty"`
	DefaultRoles    []string        `json:"default_roles,omitempty"`
	ReceiptSchema   string          `json:"receipt_schema,omitempty"`
	PreflightChecks []string        `json:"preflight_checks,omitempty"`
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
