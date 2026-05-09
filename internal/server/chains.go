package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type ChainInspectorHandler struct {
	svc    *operator.Service
	logger *slog.Logger
}

func NewChainInspectorHandler(s *Server, svc *operator.Service, logger *slog.Logger) *ChainInspectorHandler {
	h := &ChainInspectorHandler{svc: svc, logger: logger}
	s.HandleFunc("GET /api/runtime/status", h.handleRuntimeStatus)
	s.HandleFunc("GET /api/chains", h.handleListChains)
	s.HandleFunc("GET /api/chains/templates", h.handleTemplates)
	s.HandleFunc("GET /api/chains/{id}", h.handleGetChain)
	s.HandleFunc("GET /api/chains/{id}/metrics", h.handleGetChainMetrics)
	s.HandleFunc("POST /api/chains/{id}/approvals/{approval_id}/approve", h.handleApproveChainApproval)
	s.HandleFunc("POST /api/chains/{id}/approvals/{approval_id}/deny", h.handleDenyChainApproval)
	s.HandleFunc("GET /api/chains/{id}/timeline", h.handleTimeline)
	s.HandleFunc("GET /api/chains/{id}/events", h.handleEvents)
	s.HandleFunc("GET /api/chains/{id}/receipts", h.handleReceiptList)
	s.HandleFunc("GET /api/chains/{id}/receipt", h.handleReceipt)
	return h
}

func (h *ChainInspectorHandler) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.RuntimeStatus(r.Context())
	if err != nil {
		h.logger.Warn("runtime status", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeStatusResponse(status))
}

func (h *ChainInspectorHandler) handleListChains(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	chains, err := h.svc.ListChains(r.Context(), limit)
	if err != nil {
		h.logger.Warn("list chains", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]chainSummaryResponse, 0, len(chains))
	for _, summary := range chains {
		out = append(out, chainSummaryResponseFromOperator(summary))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.svc.ListLaunchTemplates(r.Context())
	if err != nil {
		h.logger.Warn("list chain templates", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]launchTemplateResponse, 0, len(templates))
	for _, template := range templates {
		out = append(out, launchTemplateResponseFromOperator(template))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleGetChain(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	detail, err := h.svc.GetChainDetail(r.Context(), chainID)
	if err != nil {
		h.logger.Warn("get chain", "chain_id", chainID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chainDetailResponseFromOperator(detail))
}

func (h *ChainInspectorHandler) handleGetChainMetrics(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	report, err := h.svc.GetChainMetrics(r.Context(), chainID)
	if err != nil {
		h.logger.Warn("get chain metrics", "chain_id", chainID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chainMetricsResponseFromOperator(report))
}

func (h *ChainInspectorHandler) handleApproveChainApproval(w http.ResponseWriter, r *http.Request) {
	h.handleApprovalDecision(w, r, true)
}

func (h *ChainInspectorHandler) handleDenyChainApproval(w http.ResponseWriter, r *http.Request) {
	h.handleApprovalDecision(w, r, false)
}

func (h *ChainInspectorHandler) handleApprovalDecision(w http.ResponseWriter, r *http.Request, approved bool) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	approvalID := strings.TrimSpace(r.PathValue("approval_id"))
	if approvalID == "" {
		writeError(w, http.StatusBadRequest, "approval id is required")
		return
	}
	var req approvalDecisionRequest
	if r.Body != http.NoBody && r.ContentLength != 0 {
		if !decodeJSON(w, r, &req, h.logger) {
			return
		}
	}
	var (
		result operator.ApprovalDecisionResult
		err    error
	)
	if approved {
		result, err = h.svc.ApproveChainApproval(r.Context(), chainID, approvalID, req.Reason)
	} else {
		result, err = h.svc.DenyChainApproval(r.Context(), chainID, approvalID, req.Reason)
	}
	if err != nil {
		h.logger.Warn("record chain approval decision", "chain_id", chainID, "approval_id", approvalID, "approved", approved, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, approvalDecisionResponseFromOperator(result))
}

func (h *ChainInspectorHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	afterID, err := parseChainEventAfterID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var events []chain.Event
	if afterID > 0 {
		events, err = h.svc.ListEventsSince(r.Context(), chainID, afterID)
	} else {
		events, err = h.svc.ListEvents(r.Context(), chainID)
	}
	if err != nil {
		h.logger.Warn("list chain events", "chain_id", chainID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	out := make([]chainEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, chainEventResponseFromChain(event))
	}
	writeJSON(w, http.StatusOK, out)
}

func parseChainEventAfterID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after_id"))
	if raw == "" {
		return 0, nil
	}
	afterID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || afterID < 0 {
		return 0, fmt.Errorf("after_id must be a non-negative integer")
	}
	return afterID, nil
}

func (h *ChainInspectorHandler) handleTimeline(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	timeline, err := h.svc.GetChainTimeline(r.Context(), chainID)
	if err != nil {
		h.logger.Warn("list chain timeline", "chain_id", chainID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	out := make([]chainTimelineResponse, 0, len(timeline))
	for _, item := range timeline {
		out = append(out, chainTimelineResponseFromOperator(item))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleReceiptList(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	detail, err := h.svc.GetChainDetail(r.Context(), chainID)
	if err != nil {
		h.logger.Warn("list chain receipts", "chain_id", chainID, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	out := make([]receiptSummaryResponse, 0, len(detail.Receipts))
	for _, receipt := range detail.Receipts {
		out = append(out, receiptSummaryResponseFromOperator(receipt))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleReceipt(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	step := strings.TrimSpace(r.URL.Query().Get("step"))
	receipt, err := h.svc.ReadReceipt(r.Context(), chainID, step)
	if err != nil {
		h.logger.Warn("read chain receipt", "chain_id", chainID, "step", step, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, receiptViewResponseFromOperator(receipt))
}

type runtimeStatusResponse struct {
	ProjectRoot         string                    `json:"project_root"`
	ProjectName         string                    `json:"project_name"`
	Provider            string                    `json:"provider"`
	Model               string                    `json:"model"`
	ContextWindow       int                       `json:"context_window"`
	ModelCapabilities   modelCapabilitiesResponse `json:"model_capabilities"`
	AuthStatus          string                    `json:"auth_status"`
	CodeIndex           runtimeIndexResponse      `json:"code_index"`
	BrainIndex          runtimeIndexResponse      `json:"brain_index"`
	LocalServicesStatus string                    `json:"local_services_status"`
	ActiveChains        int                       `json:"active_chains"`
	Warnings            []runtimeWarningResponse  `json:"warnings"`
}

type modelCapabilitiesResponse struct {
	SupportsTools            bool     `json:"supports_tools"`
	SupportsThinking         bool     `json:"supports_thinking"`
	SupportsReasoningEffort  bool     `json:"supports_reasoning_effort"`
	SupportsStructuredOutput bool     `json:"supports_structured_output"`
	SupportsPromptCache      bool     `json:"supports_prompt_cache"`
	SupportsImages           bool     `json:"supports_images"`
	SupportsToolChoice       bool     `json:"supports_tool_choice"`
	MaxOutputTokens          int      `json:"max_output_tokens,omitempty"`
	KnownQuirks              []string `json:"known_quirks,omitempty"`
}

type runtimeIndexResponse struct {
	Status            string `json:"status"`
	LastIndexedAt     string `json:"last_indexed_at,omitempty"`
	LastIndexedCommit string `json:"last_indexed_commit,omitempty"`
	StaleSince        string `json:"stale_since,omitempty"`
	StaleReason       string `json:"stale_reason,omitempty"`
}

type runtimeWarningResponse struct {
	Message string `json:"message"`
}

type chainSummaryResponse struct {
	ID          string               `json:"id"`
	Status      string               `json:"status"`
	SourceTask  string               `json:"source_task"`
	SourceSpecs []string             `json:"source_specs"`
	TotalSteps  int                  `json:"total_steps"`
	TotalTokens int                  `json:"total_tokens"`
	StartedAt   string               `json:"started_at"`
	UpdatedAt   string               `json:"updated_at"`
	CurrentStep *stepSummaryResponse `json:"current_step,omitempty"`
}

type launchTemplateResponse struct {
	ID              string          `json:"id"`
	Mode            string          `json:"mode"`
	Label           string          `json:"label"`
	Description     string          `json:"description"`
	InputSchema     json.RawMessage `json:"input_schema,omitempty"`
	DefaultRoles    []string        `json:"default_roles,omitempty"`
	ReceiptSchema   string          `json:"receipt_schema,omitempty"`
	PreflightChecks []string        `json:"preflight_checks,omitempty"`
}

type chainDetailResponse struct {
	Chain        chainRecordResponse      `json:"chain"`
	Steps        []chainStepResponse      `json:"steps"`
	Receipts     []receiptSummaryResponse `json:"receipts"`
	Approvals    []approvalResponse       `json:"approvals"`
	RecentEvents []chainEventResponse     `json:"recent_events"`
	Timeline     []chainTimelineResponse  `json:"timeline"`
	Health       string                   `json:"health"`
	Warnings     []runtimeWarningResponse `json:"warnings"`
	Guardrails   chainGuardrailResponse   `json:"guardrails"`
	Metrics      chainMetricsResponse     `json:"metrics"`
}

type chainMetricsResponse struct {
	ChainID                           string                     `json:"chain_id"`
	Status                            string                     `json:"status"`
	Health                            string                     `json:"health"`
	LaunchMode                        string                     `json:"launch_mode,omitempty"`
	StepMaxTurns                      int                        `json:"step_max_turns"`
	StepMaxTokens                     int                        `json:"step_max_tokens"`
	HasStepMaxTurns                   bool                       `json:"has_step_max_turns"`
	HasStepMaxTokens                  bool                       `json:"has_step_max_tokens"`
	TotalSteps                        int                        `json:"total_steps"`
	StepRows                          int                        `json:"step_rows"`
	MaxSteps                          int                        `json:"max_steps"`
	StepBudgetPct                     float64                    `json:"step_budget_pct"`
	CompletedSteps                    int                        `json:"completed_steps"`
	RunningSteps                      int                        `json:"running_steps"`
	PendingSteps                      int                        `json:"pending_steps"`
	FailedSteps                       int                        `json:"failed_steps"`
	TotalTokens                       int                        `json:"total_tokens"`
	StepTokenTotal                    int                        `json:"step_token_total"`
	StepTurnTotal                     int                        `json:"step_turn_total"`
	TokenBudget                       int                        `json:"token_budget"`
	TokenBudgetPct                    float64                    `json:"token_budget_pct"`
	TotalDurationSecs                 int                        `json:"total_duration_secs"`
	StepDurationSecs                  int                        `json:"step_duration_secs"`
	MaxDurationSecs                   int                        `json:"max_duration_secs"`
	DurationBudgetPct                 float64                    `json:"duration_budget_pct"`
	ResolverLoops                     int                        `json:"resolver_loops"`
	MaxResolverLoops                  int                        `json:"max_resolver_loops"`
	ResolverLoopPct                   float64                    `json:"resolver_loop_pct"`
	EventTotal                        int                        `json:"event_total"`
	OutputEvents                      int                        `json:"output_events"`
	StepFailedEvents                  int                        `json:"step_failed_events"`
	ChangedFileEvents                 int                        `json:"changed_file_events"`
	StepGuardrailFactEvents           int                        `json:"step_guardrail_fact_events"`
	ReceiptWarningEvents              int                        `json:"receipt_warning_events"`
	ReceiptFindingEvents              int                        `json:"receipt_finding_events"`
	FindingLifecycleFactEvents        int                        `json:"finding_lifecycle_fact_events"`
	OpenFindingCount                  int                        `json:"open_finding_count"`
	ClosedFindingCount                int                        `json:"closed_finding_count"`
	AddressedFindingCount             int                        `json:"addressed_finding_count"`
	OpenFindingIDs                    []string                   `json:"open_finding_ids"`
	ClosedFindingIDs                  []string                   `json:"closed_finding_ids"`
	AddressedFindingIDs               []string                   `json:"addressed_finding_ids"`
	ReopenedFindingIDs                []string                   `json:"reopened_finding_ids"`
	RepeatedResolverFindingIDs        []string                   `json:"repeated_resolver_finding_ids"`
	FindingLifecycle                  []findingLifecycleResponse `json:"finding_lifecycle"`
	SourceWriterBlocks                int                        `json:"source_writer_blocks"`
	SourceWriterLockAcquires          int                        `json:"source_writer_lock_acquires"`
	SourceWriterLockReleases          int                        `json:"source_writer_lock_releases"`
	SourceWriterLockForceReleases     int                        `json:"source_writer_lock_force_releases"`
	SourceWriterLockReleaseFailures   int                        `json:"source_writer_lock_release_failures"`
	SourceWriterLockHeartbeatFailures int                        `json:"source_writer_lock_heartbeat_failures"`
	SourceWriterLockStaleReplacements int                        `json:"source_writer_lock_stale_replacements"`
	SafetyLimitEvents                 int                        `json:"safety_limit_events"`
	ReindexStartedEvents              int                        `json:"reindex_started_events"`
	ReindexDoneEvents                 int                        `json:"reindex_done_events"`
	ProcessStartedEvents              int                        `json:"process_started_events"`
	ProcessExitedEvents               int                        `json:"process_exited_events"`
	Warnings                          []runtimeWarningResponse   `json:"warnings"`
	Steps                             []chainStepMetricResponse  `json:"steps"`
}

type chainStepMetricResponse struct {
	SequenceNum  int    `json:"sequence_num"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	Verdict      string `json:"verdict"`
	ReceiptPath  string `json:"receipt_path"`
	TokensUsed   int    `json:"tokens_used"`
	TurnsUsed    int    `json:"turns_used"`
	DurationSecs int    `json:"duration_secs"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type approvalDecisionRequest struct {
	Reason string `json:"reason"`
}

type approvalDecisionResponse struct {
	Approval approvalResponse `json:"approval"`
	Message  string           `json:"message"`
}

type approvalResponse struct {
	ID             string          `json:"id"`
	ChainID        string          `json:"chain_id"`
	StepID         string          `json:"step_id,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	TurnNumber     int             `json:"turn_number,omitempty"`
	Iteration      int             `json:"iteration,omitempty"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	RiskLevel      string          `json:"risk_level,omitempty"`
	Status         string          `json:"status"`
	CreatedAt      string          `json:"created_at,omitempty"`
	DecidedAt      string          `json:"decided_at,omitempty"`
	DecisionReason string          `json:"decision_reason,omitempty"`
	DecidedBy      string          `json:"decided_by,omitempty"`
}

type chainTimelineResponse struct {
	ID             string         `json:"id"`
	Source         string         `json:"source"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	Status         string         `json:"status,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	SpanID         string         `json:"span_id,omitempty"`
	ParentSpanID   string         `json:"parent_span_id,omitempty"`
	ConversationID string         `json:"conversation_id,omitempty"`
	ChainID        string         `json:"chain_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	TurnNumber     int            `json:"turn_number,omitempty"`
	Iteration      int            `json:"iteration,omitempty"`
	StartedAt      string         `json:"started_at"`
	EndedAt        string         `json:"ended_at,omitempty"`
	DurationMs     int64          `json:"duration_ms,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`
	Error          string         `json:"error,omitempty"`
	EventType      string         `json:"event_type,omitempty"`
	EventData      string         `json:"event_data,omitempty"`
}

type chainGuardrailResponse struct {
	OpenFindingIDs             []string                      `json:"open_finding_ids"`
	ClosedFindingIDs           []string                      `json:"closed_finding_ids"`
	AddressedFindingIDs        []string                      `json:"addressed_finding_ids"`
	ReopenedFindingIDs         []string                      `json:"reopened_finding_ids"`
	RepeatedResolverFindingIDs []string                      `json:"repeated_resolver_finding_ids"`
	Findings                   []findingLifecycleResponse    `json:"findings"`
	LockHealth                 guardrailLockHealthResponse   `json:"lock_health"`
	ChangedFiles               []changedFileManifestResponse `json:"changed_files"`
	StepFacts                  []stepGuardrailFactResponse   `json:"step_facts"`
}

type findingLifecycleResponse struct {
	ID              string   `json:"id"`
	SourceRole      string   `json:"source_role"`
	Status          string   `json:"status"`
	Severity        string   `json:"severity,omitempty"`
	Evidence        string   `json:"evidence,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	RequiredFix     string   `json:"required_fix,omitempty"`
	Resolution      string   `json:"resolution,omitempty"`
	FilesChanged    []string `json:"files_changed"`
	Validation      []string `json:"validation"`
	AddressedCount  int      `json:"addressed_count"`
	ClosedCount     int      `json:"closed_count"`
	ReopenedCount   int      `json:"reopened_count"`
	FirstSeenStep   int      `json:"first_seen_step"`
	LastUpdatedStep int      `json:"last_updated_step"`
}

type guardrailLockHealthResponse struct {
	Acquired          int `json:"acquired"`
	Released          int `json:"released"`
	Blocked           int `json:"blocked"`
	ForceReleased     int `json:"force_released"`
	ReleaseFailed     int `json:"release_failed"`
	HeartbeatFailed   int `json:"heartbeat_failed"`
	StaleReplaced     int `json:"stale_replaced"`
	UnreleasedWriters int `json:"unreleased_writers"`
}

type changedFileManifestResponse struct {
	StepID      string   `json:"step_id"`
	SequenceNum int      `json:"sequence_num"`
	Role        string   `json:"role"`
	Paths       []string `json:"paths"`
	Error       string   `json:"error,omitempty"`
}

type stepGuardrailFactResponse struct {
	StepID                              string   `json:"step_id"`
	SequenceNum                         int      `json:"sequence_num"`
	Role                                string   `json:"role"`
	ReceiptPath                         string   `json:"receipt_path"`
	SourceMutating                      bool     `json:"source_mutating"`
	ExitCode                            int      `json:"exit_code"`
	DurationSecs                        int      `json:"duration_secs"`
	ReceiptPresent                      bool     `json:"receipt_present"`
	SyntheticReceiptWritten             bool     `json:"synthetic_receipt_written"`
	ReceiptValid                        bool     `json:"receipt_valid"`
	ReceiptSchemaValid                  bool     `json:"receipt_schema_valid"`
	ReceiptStepValid                    bool     `json:"receipt_step_valid"`
	ReceiptSectionsValid                bool     `json:"receipt_sections_valid"`
	ReceiptError                        string   `json:"receipt_error,omitempty"`
	ParsedVerdict                       string   `json:"parsed_verdict,omitempty"`
	TokensUsed                          int      `json:"tokens_used"`
	TurnsUsed                           int      `json:"turns_used"`
	ReceiptDurationSeconds              int      `json:"receipt_duration_seconds"`
	ClaimedValidationCommands           []string `json:"claimed_validation_commands"`
	ChangedFileClaimPresent             bool     `json:"changed_file_claim_present"`
	ClaimedChangedFiles                 []string `json:"claimed_changed_files"`
	ChangedFileClaimMatchesManifest     bool     `json:"changed_file_claim_matches_manifest"`
	ChangedFileClaimExtra               []string `json:"changed_file_claim_extra"`
	ChangedFileManifestUnclaimed        []string `json:"changed_file_manifest_unclaimed"`
	ChangedFileManifestPresent          bool     `json:"changed_file_manifest_present"`
	ChangedFileManifestError            string   `json:"changed_file_manifest_error,omitempty"`
	ChangedFileCount                    int      `json:"changed_file_count"`
	ChangedFiles                        []string `json:"changed_files"`
	CodeIndexStateSupported             bool     `json:"code_index_state_supported"`
	CodeIndexStateFound                 bool     `json:"code_index_state_found"`
	CodeIndexDirtyMarkSupported         bool     `json:"code_index_dirty_mark_supported"`
	CodeIndexDirtyMarkAttempted         bool     `json:"code_index_dirty_mark_attempted"`
	CodeIndexDirtyMarked                bool     `json:"code_index_dirty_marked"`
	CodeIndexDirtyMarkError             string   `json:"code_index_dirty_mark_error,omitempty"`
	CodeIndexDirty                      bool     `json:"code_index_dirty"`
	CodeIndexDirtyReason                string   `json:"code_index_dirty_reason,omitempty"`
	CodeIndexStateError                 string   `json:"code_index_state_error,omitempty"`
	BrainIndexStateSupported            bool     `json:"brain_index_state_supported"`
	BrainIndexStateFound                bool     `json:"brain_index_state_found"`
	BrainIndexDirty                     bool     `json:"brain_index_dirty"`
	BrainIndexDirtyReason               string   `json:"brain_index_dirty_reason,omitempty"`
	BrainIndexStateError                string   `json:"brain_index_state_error,omitempty"`
	SourceWriterLockReleaseAttempted    bool     `json:"source_writer_lock_release_attempted"`
	SourceWriterLockReleased            bool     `json:"source_writer_lock_released"`
	SourceWriterLockReleaseError        string   `json:"source_writer_lock_release_error,omitempty"`
	FindingCount                        int      `json:"finding_count"`
	OpenFindingCount                    int      `json:"open_finding_count"`
	ClosedFindingCount                  int      `json:"closed_finding_count"`
	AddressedFindingCount               int      `json:"addressed_finding_count"`
	FindingIDs                          []string `json:"finding_ids"`
	OpenFindingIDs                      []string `json:"open_finding_ids"`
	ClosedFindingIDs                    []string `json:"closed_finding_ids"`
	AddressedIDs                        []string `json:"addressed_ids"`
	SuspiciousVerdictFindingCombination bool     `json:"suspicious_verdict_finding_combination"`
	SuspiciousVerdictFindingReason      string   `json:"suspicious_verdict_finding_reason,omitempty"`
	RunError                            string   `json:"run_error,omitempty"`
}

type chainRecordResponse struct {
	ID                string   `json:"id"`
	SourceSpecs       []string `json:"source_specs"`
	SourceTask        string   `json:"source_task"`
	Status            string   `json:"status"`
	Summary           string   `json:"summary"`
	TotalSteps        int      `json:"total_steps"`
	TotalTokens       int      `json:"total_tokens"`
	TotalDurationSecs int      `json:"total_duration_secs"`
	ResolverLoops     int      `json:"resolver_loops"`
	StartedAt         string   `json:"started_at"`
	CompletedAt       string   `json:"completed_at,omitempty"`
	UpdatedAt         string   `json:"updated_at"`
}

type stepSummaryResponse struct {
	ID          string `json:"id"`
	SequenceNum int    `json:"sequence_num"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Verdict     string `json:"verdict"`
	ReceiptPath string `json:"receipt_path"`
	TokensUsed  int    `json:"tokens_used"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type chainStepResponse struct {
	ID           string `json:"id"`
	ChainID      string `json:"chain_id"`
	SequenceNum  int    `json:"sequence_num"`
	Role         string `json:"role"`
	Task         string `json:"task"`
	Status       string `json:"status"`
	Verdict      string `json:"verdict"`
	ReceiptPath  string `json:"receipt_path"`
	TokensUsed   int    `json:"tokens_used"`
	TurnsUsed    int    `json:"turns_used"`
	DurationSecs int    `json:"duration_secs"`
	ErrorMessage string `json:"error_message,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
}

type chainEventResponse struct {
	ID        int64  `json:"id"`
	ChainID   string `json:"chain_id"`
	StepID    string `json:"step_id"`
	EventType string `json:"event_type"`
	EventData string `json:"event_data"`
	CreatedAt string `json:"created_at"`
}

type receiptSummaryResponse struct {
	Label string `json:"label"`
	Step  string `json:"step"`
	Path  string `json:"path"`
}

type receiptViewResponse struct {
	ChainID string `json:"chain_id"`
	Step    string `json:"step"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func chainSummaryResponseFromOperator(summary operator.ChainSummary) chainSummaryResponse {
	return chainSummaryResponse{
		ID:          summary.ID,
		Status:      summary.Status,
		SourceTask:  summary.SourceTask,
		SourceSpecs: append([]string(nil), summary.SourceSpecs...),
		TotalSteps:  summary.TotalSteps,
		TotalTokens: summary.TotalTokens,
		StartedAt:   formatTime(summary.StartedAt),
		UpdatedAt:   formatTime(summary.UpdatedAt),
		CurrentStep: stepSummaryResponseFromOperator(summary.CurrentStep),
	}
}

func chainDetailResponseFromOperator(detail operator.ChainDetail) chainDetailResponse {
	steps := make([]chainStepResponse, 0, len(detail.Steps))
	for _, step := range detail.Steps {
		steps = append(steps, chainStepResponseFromChain(step))
	}
	receipts := make([]receiptSummaryResponse, 0, len(detail.Receipts))
	for _, receipt := range detail.Receipts {
		receipts = append(receipts, receiptSummaryResponseFromOperator(receipt))
	}
	approvals := make([]approvalResponse, 0, len(detail.Approvals))
	for _, approval := range detail.Approvals {
		approvals = append(approvals, approvalResponseFromOperator(approval))
	}
	events := make([]chainEventResponse, 0, len(detail.RecentEvents))
	for _, event := range detail.RecentEvents {
		events = append(events, chainEventResponseFromChain(event))
	}
	timeline := make([]chainTimelineResponse, 0, len(detail.Timeline))
	for _, item := range detail.Timeline {
		timeline = append(timeline, chainTimelineResponseFromOperator(item))
	}
	warnings := make([]runtimeWarningResponse, 0, len(detail.Warnings))
	for _, warning := range detail.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	return chainDetailResponse{
		Chain:        chainRecordResponseFromChain(detail.Chain),
		Steps:        steps,
		Receipts:     receipts,
		Approvals:    approvals,
		RecentEvents: events,
		Timeline:     timeline,
		Health:       detail.Health,
		Warnings:     warnings,
		Guardrails:   chainGuardrailResponseFromOperator(detail.Guardrails),
		Metrics:      chainMetricsResponseFromOperator(detail.Metrics),
	}
}

func chainMetricsResponseFromOperator(report operator.ChainMetricsReport) chainMetricsResponse {
	warnings := make([]runtimeWarningResponse, 0, len(report.Warnings))
	for _, warning := range report.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	steps := make([]chainStepMetricResponse, 0, len(report.Steps))
	for _, step := range report.Steps {
		steps = append(steps, chainStepMetricResponse{
			SequenceNum:  step.SequenceNum,
			Role:         step.Role,
			Status:       step.Status,
			Verdict:      step.Verdict,
			ReceiptPath:  step.ReceiptPath,
			TokensUsed:   step.TokensUsed,
			TurnsUsed:    step.TurnsUsed,
			DurationSecs: step.DurationSecs,
			ExitCode:     step.ExitCode,
			ErrorMessage: step.ErrorMessage,
		})
	}
	findings := make([]findingLifecycleResponse, 0, len(report.FindingLifecycle))
	for _, finding := range report.FindingLifecycle {
		findings = append(findings, findingLifecycleResponseFromOperator(finding))
	}
	return chainMetricsResponse{
		ChainID:                           report.ChainID,
		Status:                            report.Status,
		Health:                            report.Health,
		LaunchMode:                        report.LaunchMode,
		StepMaxTurns:                      report.StepMaxTurns,
		StepMaxTokens:                     report.StepMaxTokens,
		HasStepMaxTurns:                   report.HasStepMaxTurns,
		HasStepMaxTokens:                  report.HasStepMaxTokens,
		TotalSteps:                        report.TotalSteps,
		StepRows:                          report.StepRows,
		MaxSteps:                          report.MaxSteps,
		StepBudgetPct:                     report.StepBudgetPct,
		CompletedSteps:                    report.CompletedSteps,
		RunningSteps:                      report.RunningSteps,
		PendingSteps:                      report.PendingSteps,
		FailedSteps:                       report.FailedSteps,
		TotalTokens:                       report.TotalTokens,
		StepTokenTotal:                    report.StepTokenTotal,
		StepTurnTotal:                     report.StepTurnTotal,
		TokenBudget:                       report.TokenBudget,
		TokenBudgetPct:                    report.TokenBudgetPct,
		TotalDurationSecs:                 report.TotalDurationSecs,
		StepDurationSecs:                  report.StepDurationSecs,
		MaxDurationSecs:                   report.MaxDurationSecs,
		DurationBudgetPct:                 report.DurationBudgetPct,
		ResolverLoops:                     report.ResolverLoops,
		MaxResolverLoops:                  report.MaxResolverLoops,
		ResolverLoopPct:                   report.ResolverLoopPct,
		EventTotal:                        report.EventTotal,
		OutputEvents:                      report.OutputEvents,
		StepFailedEvents:                  report.StepFailedEvents,
		ChangedFileEvents:                 report.ChangedFileEvents,
		StepGuardrailFactEvents:           report.StepGuardrailFactEvents,
		ReceiptWarningEvents:              report.ReceiptWarningEvents,
		ReceiptFindingEvents:              report.ReceiptFindingEvents,
		FindingLifecycleFactEvents:        report.FindingLifecycleFactEvents,
		OpenFindingCount:                  report.OpenFindingCount,
		ClosedFindingCount:                report.ClosedFindingCount,
		AddressedFindingCount:             report.AddressedFindingCount,
		OpenFindingIDs:                    append([]string(nil), report.OpenFindingIDs...),
		ClosedFindingIDs:                  append([]string(nil), report.ClosedFindingIDs...),
		AddressedFindingIDs:               append([]string(nil), report.AddressedFindingIDs...),
		ReopenedFindingIDs:                append([]string(nil), report.ReopenedFindingIDs...),
		RepeatedResolverFindingIDs:        append([]string(nil), report.RepeatedResolverFindingIDs...),
		FindingLifecycle:                  findings,
		SourceWriterBlocks:                report.SourceWriterBlocks,
		SourceWriterLockAcquires:          report.SourceWriterLockAcquires,
		SourceWriterLockReleases:          report.SourceWriterLockReleases,
		SourceWriterLockForceReleases:     report.SourceWriterLockForceReleases,
		SourceWriterLockReleaseFailures:   report.SourceWriterLockReleaseFailures,
		SourceWriterLockHeartbeatFailures: report.SourceWriterLockHeartbeatFailures,
		SourceWriterLockStaleReplacements: report.SourceWriterLockStaleReplacements,
		SafetyLimitEvents:                 report.SafetyLimitEvents,
		ReindexStartedEvents:              report.ReindexStartedEvents,
		ReindexDoneEvents:                 report.ReindexDoneEvents,
		ProcessStartedEvents:              report.ProcessStartedEvents,
		ProcessExitedEvents:               report.ProcessExitedEvents,
		Warnings:                          warnings,
		Steps:                             steps,
	}
}

func approvalDecisionResponseFromOperator(result operator.ApprovalDecisionResult) approvalDecisionResponse {
	return approvalDecisionResponse{
		Approval: approvalResponseFromOperator(result.Approval),
		Message:  result.Message,
	}
}

func approvalResponseFromOperator(approval operator.ApprovalView) approvalResponse {
	return approvalResponse{
		ID:             approval.ID,
		ChainID:        approval.ChainID,
		StepID:         approval.StepID,
		ConversationID: approval.ConversationID,
		TurnNumber:     approval.TurnNumber,
		Iteration:      approval.Iteration,
		ToolName:       approval.ToolName,
		ToolInput:      append(json.RawMessage(nil), approval.ToolInput...),
		Reason:         approval.Reason,
		RiskLevel:      approval.RiskLevel,
		Status:         approval.Status,
		CreatedAt:      formatTime(approval.CreatedAt),
		DecidedAt:      formatTimePtr(approval.DecidedAt),
		DecisionReason: approval.DecisionReason,
		DecidedBy:      approval.DecidedBy,
	}
}

func chainTimelineResponseFromOperator(item operator.ChainTimelineItem) chainTimelineResponse {
	attrs := item.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return chainTimelineResponse{
		ID:             item.ID,
		Source:         item.Source,
		Kind:           item.Kind,
		Name:           item.Name,
		Status:         item.Status,
		TraceID:        item.TraceID,
		SpanID:         item.SpanID,
		ParentSpanID:   item.ParentSpanID,
		ConversationID: item.ConversationID,
		ChainID:        item.ChainID,
		StepID:         item.StepID,
		TurnNumber:     item.TurnNumber,
		Iteration:      item.Iteration,
		StartedAt:      formatTime(item.StartedAt),
		EndedAt:        formatTimePtr(item.EndedAt),
		DurationMs:     item.DurationMs,
		Attributes:     attrs,
		Error:          item.Error,
		EventType:      item.EventType,
		EventData:      item.EventData,
	}
}

func findingLifecycleResponseFromOperator(finding operator.FindingLifecycleMetric) findingLifecycleResponse {
	return findingLifecycleResponse{
		ID:              finding.ID,
		SourceRole:      finding.SourceRole,
		Status:          finding.Status,
		Severity:        finding.Severity,
		Evidence:        finding.Evidence,
		Summary:         finding.Summary,
		RequiredFix:     finding.RequiredFix,
		Resolution:      finding.Resolution,
		FilesChanged:    append([]string(nil), finding.FilesChanged...),
		Validation:      append([]string(nil), finding.Validation...),
		AddressedCount:  finding.AddressedCount,
		ClosedCount:     finding.ClosedCount,
		ReopenedCount:   finding.ReopenedCount,
		FirstSeenStep:   finding.FirstSeenStep,
		LastUpdatedStep: finding.LastUpdatedStep,
	}
}

func chainGuardrailResponseFromOperator(details operator.ChainGuardrailDetails) chainGuardrailResponse {
	findings := make([]findingLifecycleResponse, 0, len(details.Findings))
	for _, finding := range details.Findings {
		findings = append(findings, findingLifecycleResponseFromOperator(finding))
	}
	changedFiles := make([]changedFileManifestResponse, 0, len(details.ChangedFiles))
	for _, manifest := range details.ChangedFiles {
		changedFiles = append(changedFiles, changedFileManifestResponse{
			StepID:      manifest.StepID,
			SequenceNum: manifest.SequenceNum,
			Role:        manifest.Role,
			Paths:       append([]string(nil), manifest.Paths...),
			Error:       manifest.Error,
		})
	}
	stepFacts := make([]stepGuardrailFactResponse, 0, len(details.StepFacts))
	for _, facts := range details.StepFacts {
		stepFacts = append(stepFacts, stepGuardrailFactResponse{
			StepID:                              facts.StepID,
			SequenceNum:                         facts.SequenceNum,
			Role:                                facts.Role,
			ReceiptPath:                         facts.ReceiptPath,
			SourceMutating:                      facts.SourceMutating,
			ExitCode:                            facts.ExitCode,
			DurationSecs:                        facts.DurationSecs,
			ReceiptPresent:                      facts.ReceiptPresent,
			SyntheticReceiptWritten:             facts.SyntheticReceiptWritten,
			ReceiptValid:                        facts.ReceiptValid,
			ReceiptSchemaValid:                  facts.ReceiptSchemaValid,
			ReceiptStepValid:                    facts.ReceiptStepValid,
			ReceiptSectionsValid:                facts.ReceiptSectionsValid,
			ReceiptError:                        facts.ReceiptError,
			ParsedVerdict:                       facts.ParsedVerdict,
			TokensUsed:                          facts.TokensUsed,
			TurnsUsed:                           facts.TurnsUsed,
			ReceiptDurationSeconds:              facts.ReceiptDurationSeconds,
			ClaimedValidationCommands:           append([]string(nil), facts.ClaimedValidationCommands...),
			ChangedFileClaimPresent:             facts.ChangedFileClaimPresent,
			ClaimedChangedFiles:                 append([]string(nil), facts.ClaimedChangedFiles...),
			ChangedFileClaimMatchesManifest:     facts.ChangedFileClaimMatchesManifest,
			ChangedFileClaimExtra:               append([]string(nil), facts.ChangedFileClaimExtra...),
			ChangedFileManifestUnclaimed:        append([]string(nil), facts.ChangedFileManifestUnclaimed...),
			ChangedFileManifestPresent:          facts.ChangedFileManifestPresent,
			ChangedFileManifestError:            facts.ChangedFileManifestError,
			ChangedFileCount:                    facts.ChangedFileCount,
			ChangedFiles:                        append([]string(nil), facts.ChangedFiles...),
			CodeIndexStateSupported:             facts.CodeIndexStateSupported,
			CodeIndexStateFound:                 facts.CodeIndexStateFound,
			CodeIndexDirtyMarkSupported:         facts.CodeIndexDirtyMarkSupported,
			CodeIndexDirtyMarkAttempted:         facts.CodeIndexDirtyMarkAttempted,
			CodeIndexDirtyMarked:                facts.CodeIndexDirtyMarked,
			CodeIndexDirtyMarkError:             facts.CodeIndexDirtyMarkError,
			CodeIndexDirty:                      facts.CodeIndexDirty,
			CodeIndexDirtyReason:                facts.CodeIndexDirtyReason,
			CodeIndexStateError:                 facts.CodeIndexStateError,
			BrainIndexStateSupported:            facts.BrainIndexStateSupported,
			BrainIndexStateFound:                facts.BrainIndexStateFound,
			BrainIndexDirty:                     facts.BrainIndexDirty,
			BrainIndexDirtyReason:               facts.BrainIndexDirtyReason,
			BrainIndexStateError:                facts.BrainIndexStateError,
			SourceWriterLockReleaseAttempted:    facts.SourceWriterLockReleaseAttempted,
			SourceWriterLockReleased:            facts.SourceWriterLockReleased,
			SourceWriterLockReleaseError:        facts.SourceWriterLockReleaseError,
			FindingCount:                        facts.FindingCount,
			OpenFindingCount:                    facts.OpenFindingCount,
			ClosedFindingCount:                  facts.ClosedFindingCount,
			AddressedFindingCount:               facts.AddressedFindingCount,
			FindingIDs:                          append([]string(nil), facts.FindingIDs...),
			OpenFindingIDs:                      append([]string(nil), facts.OpenFindingIDs...),
			ClosedFindingIDs:                    append([]string(nil), facts.ClosedFindingIDs...),
			AddressedIDs:                        append([]string(nil), facts.AddressedIDs...),
			SuspiciousVerdictFindingCombination: facts.SuspiciousVerdictFindingCombination,
			SuspiciousVerdictFindingReason:      facts.SuspiciousVerdictFindingReason,
			RunError:                            facts.RunError,
		})
	}
	return chainGuardrailResponse{
		OpenFindingIDs:             append([]string(nil), details.OpenFindingIDs...),
		ClosedFindingIDs:           append([]string(nil), details.ClosedFindingIDs...),
		AddressedFindingIDs:        append([]string(nil), details.AddressedFindingIDs...),
		ReopenedFindingIDs:         append([]string(nil), details.ReopenedFindingIDs...),
		RepeatedResolverFindingIDs: append([]string(nil), details.RepeatedResolverFindingIDs...),
		Findings:                   findings,
		LockHealth: guardrailLockHealthResponse{
			Acquired:          details.LockHealth.Acquired,
			Released:          details.LockHealth.Released,
			Blocked:           details.LockHealth.Blocked,
			ForceReleased:     details.LockHealth.ForceReleased,
			ReleaseFailed:     details.LockHealth.ReleaseFailed,
			HeartbeatFailed:   details.LockHealth.HeartbeatFailed,
			StaleReplaced:     details.LockHealth.StaleReplaced,
			UnreleasedWriters: details.LockHealth.UnreleasedWriters,
		},
		ChangedFiles: changedFiles,
		StepFacts:    stepFacts,
	}
}

func toRuntimeStatusResponse(status operator.RuntimeStatus) runtimeStatusResponse {
	warnings := make([]runtimeWarningResponse, 0, len(status.Warnings))
	for _, warning := range status.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	return runtimeStatusResponse{
		ProjectRoot:         status.ProjectRoot,
		ProjectName:         status.ProjectName,
		Provider:            status.Provider,
		Model:               status.Model,
		ContextWindow:       status.ContextWindow,
		ModelCapabilities:   modelCapabilitiesResponseFromOperator(status.ModelCapabilities),
		AuthStatus:          status.AuthStatus,
		CodeIndex:           toRuntimeIndexResponse(status.CodeIndex),
		BrainIndex:          toRuntimeIndexResponse(status.BrainIndex),
		LocalServicesStatus: status.LocalServicesStatus,
		ActiveChains:        status.ActiveChains,
		Warnings:            warnings,
	}
}

func modelCapabilitiesResponseFromOperator(capabilities operator.ModelCapabilities) modelCapabilitiesResponse {
	return modelCapabilitiesResponse{
		SupportsTools:            capabilities.SupportsTools,
		SupportsThinking:         capabilities.SupportsThinking,
		SupportsReasoningEffort:  capabilities.SupportsReasoningEffort,
		SupportsStructuredOutput: capabilities.SupportsStructuredOutput,
		SupportsPromptCache:      capabilities.SupportsPromptCache,
		SupportsImages:           capabilities.SupportsImages,
		SupportsToolChoice:       capabilities.SupportsToolChoice,
		MaxOutputTokens:          capabilities.MaxOutputTokens,
		KnownQuirks:              append([]string(nil), capabilities.KnownQuirks...),
	}
}

func toRuntimeIndexResponse(status operator.RuntimeIndexStatus) runtimeIndexResponse {
	return runtimeIndexResponse{
		Status:            status.Status,
		LastIndexedAt:     status.LastIndexedAt,
		LastIndexedCommit: status.LastIndexedCommit,
		StaleSince:        status.StaleSince,
		StaleReason:       status.StaleReason,
	}
}

func stepSummaryResponseFromOperator(step *operator.StepSummary) *stepSummaryResponse {
	if step == nil {
		return nil
	}
	return &stepSummaryResponse{
		ID:          step.ID,
		SequenceNum: step.SequenceNum,
		Role:        step.Role,
		Status:      step.Status,
		Verdict:     step.Verdict,
		ReceiptPath: step.ReceiptPath,
		TokensUsed:  step.TokensUsed,
		StartedAt:   formatTimePtr(step.StartedAt),
		CompletedAt: formatTimePtr(step.CompletedAt),
	}
}

func launchTemplateResponseFromOperator(template operator.LaunchTemplate) launchTemplateResponse {
	return launchTemplateResponse{
		ID:              template.ID,
		Mode:            string(template.Mode),
		Label:           template.Label,
		Description:     template.Description,
		InputSchema:     append(json.RawMessage(nil), template.InputSchema...),
		DefaultRoles:    append([]string(nil), template.DefaultRoles...),
		ReceiptSchema:   template.ReceiptSchema,
		PreflightChecks: append([]string(nil), template.PreflightChecks...),
	}
}

func chainRecordResponseFromChain(ch chain.Chain) chainRecordResponse {
	return chainRecordResponse{
		ID:                ch.ID,
		SourceSpecs:       append([]string(nil), ch.SourceSpecs...),
		SourceTask:        ch.SourceTask,
		Status:            ch.Status,
		Summary:           ch.Summary,
		TotalSteps:        ch.TotalSteps,
		TotalTokens:       ch.TotalTokens,
		TotalDurationSecs: ch.TotalDurationSecs,
		ResolverLoops:     ch.ResolverLoops,
		StartedAt:         formatTime(ch.StartedAt),
		CompletedAt:       formatTimePtr(ch.CompletedAt),
		UpdatedAt:         formatTime(ch.UpdatedAt),
	}
}

func chainStepResponseFromChain(step chain.Step) chainStepResponse {
	return chainStepResponse{
		ID:           step.ID,
		ChainID:      step.ChainID,
		SequenceNum:  step.SequenceNum,
		Role:         step.Role,
		Task:         step.Task,
		Status:       step.Status,
		Verdict:      step.Verdict,
		ReceiptPath:  step.ReceiptPath,
		TokensUsed:   step.TokensUsed,
		TurnsUsed:    step.TurnsUsed,
		DurationSecs: step.DurationSecs,
		ErrorMessage: step.ErrorMessage,
		StartedAt:    formatTimePtr(step.StartedAt),
		CompletedAt:  formatTimePtr(step.CompletedAt),
	}
}

func chainEventResponseFromChain(event chain.Event) chainEventResponse {
	return chainEventResponse{
		ID:        event.ID,
		ChainID:   event.ChainID,
		StepID:    event.StepID,
		EventType: string(event.EventType),
		EventData: event.EventData,
		CreatedAt: formatTime(event.CreatedAt),
	}
}

func receiptSummaryResponseFromOperator(receipt operator.ReceiptSummary) receiptSummaryResponse {
	return receiptSummaryResponse{Label: receipt.Label, Step: receipt.Step, Path: receipt.Path}
}

func receiptViewResponseFromOperator(receipt operator.ReceiptView) receiptViewResponse {
	return receiptViewResponse{ChainID: receipt.ChainID, Step: receipt.Step, Path: receipt.Path, Content: receipt.Content}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
