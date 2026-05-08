package server

import (
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
	s.HandleFunc("GET /api/chains/{id}", h.handleGetChain)
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

func (h *ChainInspectorHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	events, err := h.svc.ListEvents(r.Context(), chainID)
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
	ProjectRoot         string                   `json:"project_root"`
	ProjectName         string                   `json:"project_name"`
	Provider            string                   `json:"provider"`
	Model               string                   `json:"model"`
	AuthStatus          string                   `json:"auth_status"`
	CodeIndex           runtimeIndexResponse     `json:"code_index"`
	BrainIndex          runtimeIndexResponse     `json:"brain_index"`
	LocalServicesStatus string                   `json:"local_services_status"`
	ActiveChains        int                      `json:"active_chains"`
	Warnings            []runtimeWarningResponse `json:"warnings"`
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

type chainDetailResponse struct {
	Chain        chainRecordResponse      `json:"chain"`
	Steps        []chainStepResponse      `json:"steps"`
	Receipts     []receiptSummaryResponse `json:"receipts"`
	RecentEvents []chainEventResponse     `json:"recent_events"`
	Health       string                   `json:"health"`
	Warnings     []runtimeWarningResponse `json:"warnings"`
	Guardrails   chainGuardrailResponse   `json:"guardrails"`
}

type chainGuardrailResponse struct {
	OpenFindingIDs             []string                      `json:"open_finding_ids"`
	ClosedFindingIDs           []string                      `json:"closed_finding_ids"`
	AddressedFindingIDs        []string                      `json:"addressed_finding_ids"`
	ReopenedFindingIDs         []string                      `json:"reopened_finding_ids"`
	RepeatedResolverFindingIDs []string                      `json:"repeated_resolver_finding_ids"`
	LockHealth                 guardrailLockHealthResponse   `json:"lock_health"`
	ChangedFiles               []changedFileManifestResponse `json:"changed_files"`
	StepFacts                  []stepGuardrailFactResponse   `json:"step_facts"`
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
	StepID                           string   `json:"step_id"`
	SequenceNum                      int      `json:"sequence_num"`
	Role                             string   `json:"role"`
	ReceiptPath                      string   `json:"receipt_path"`
	SourceMutating                   bool     `json:"source_mutating"`
	ReceiptValid                     bool     `json:"receipt_valid"`
	ReceiptSchemaValid               bool     `json:"receipt_schema_valid"`
	ReceiptSectionsValid             bool     `json:"receipt_sections_valid"`
	ReceiptError                     string   `json:"receipt_error,omitempty"`
	ChangedFileManifestPresent       bool     `json:"changed_file_manifest_present"`
	ChangedFileCount                 int      `json:"changed_file_count"`
	ChangedFiles                     []string `json:"changed_files"`
	SourceWriterLockReleaseAttempted bool     `json:"source_writer_lock_release_attempted"`
	SourceWriterLockReleased         bool     `json:"source_writer_lock_released"`
	SourceWriterLockReleaseError     string   `json:"source_writer_lock_release_error,omitempty"`
	OpenFindingIDs                   []string `json:"open_finding_ids"`
	ClosedFindingIDs                 []string `json:"closed_finding_ids"`
	AddressedIDs                     []string `json:"addressed_ids"`
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
	events := make([]chainEventResponse, 0, len(detail.RecentEvents))
	for _, event := range detail.RecentEvents {
		events = append(events, chainEventResponseFromChain(event))
	}
	warnings := make([]runtimeWarningResponse, 0, len(detail.Warnings))
	for _, warning := range detail.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	return chainDetailResponse{
		Chain:        chainRecordResponseFromChain(detail.Chain),
		Steps:        steps,
		Receipts:     receipts,
		RecentEvents: events,
		Health:       detail.Health,
		Warnings:     warnings,
		Guardrails:   chainGuardrailResponseFromOperator(detail.Guardrails),
	}
}

func chainGuardrailResponseFromOperator(details operator.ChainGuardrailDetails) chainGuardrailResponse {
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
			StepID:                           facts.StepID,
			SequenceNum:                      facts.SequenceNum,
			Role:                             facts.Role,
			ReceiptPath:                      facts.ReceiptPath,
			SourceMutating:                   facts.SourceMutating,
			ReceiptValid:                     facts.ReceiptValid,
			ReceiptSchemaValid:               facts.ReceiptSchemaValid,
			ReceiptSectionsValid:             facts.ReceiptSectionsValid,
			ReceiptError:                     facts.ReceiptError,
			ChangedFileManifestPresent:       facts.ChangedFileManifestPresent,
			ChangedFileCount:                 facts.ChangedFileCount,
			ChangedFiles:                     append([]string(nil), facts.ChangedFiles...),
			SourceWriterLockReleaseAttempted: facts.SourceWriterLockReleaseAttempted,
			SourceWriterLockReleased:         facts.SourceWriterLockReleased,
			SourceWriterLockReleaseError:     facts.SourceWriterLockReleaseError,
			OpenFindingIDs:                   append([]string(nil), facts.OpenFindingIDs...),
			ClosedFindingIDs:                 append([]string(nil), facts.ClosedFindingIDs...),
			AddressedIDs:                     append([]string(nil), facts.AddressedIDs...),
		})
	}
	return chainGuardrailResponse{
		OpenFindingIDs:             append([]string(nil), details.OpenFindingIDs...),
		ClosedFindingIDs:           append([]string(nil), details.ClosedFindingIDs...),
		AddressedFindingIDs:        append([]string(nil), details.AddressedFindingIDs...),
		ReopenedFindingIDs:         append([]string(nil), details.ReopenedFindingIDs...),
		RepeatedResolverFindingIDs: append([]string(nil), details.RepeatedResolverFindingIDs...),
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
		AuthStatus:          status.AuthStatus,
		CodeIndex:           toRuntimeIndexResponse(status.CodeIndex),
		BrainIndex:          toRuntimeIndexResponse(status.BrainIndex),
		LocalServicesStatus: status.LocalServicesStatus,
		ActiveChains:        status.ActiveChains,
		Warnings:            warnings,
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
