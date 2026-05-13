package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/operator"
)

type agentRoleResponse struct {
	Name string `json:"name"`
}

type launchRequestPayload struct {
	TemplateID        string   `json:"template_id"`
	Mode              string   `json:"mode"`
	Role              string   `json:"role"`
	AllowedRoles      []string `json:"allowed_roles"`
	Roster            []string `json:"roster"`
	SourceTask        string   `json:"source_task"`
	SourceSpecs       []string `json:"source_specs"`
	Task              string   `json:"task"`
	Specs             []string `json:"specs"`
	MaxSteps          int      `json:"max_steps"`
	MaxResolverLoops  int      `json:"max_resolver_loops"`
	MaxDuration       string   `json:"max_duration"`
	TokenBudget       int      `json:"token_budget"`
	StepMaxTurns      int      `json:"step_max_turns"`
	StepMaxTokens     int      `json:"step_max_tokens"`
	AllowApprovalWait bool     `json:"allow_approval_wait"`
}

type launchRequestEnvelope struct {
	Request *launchRequestPayload `json:"request"`
	launchRequestPayload
}

type saveLaunchPresetRequest struct {
	Name    string                `json:"name"`
	Request *launchRequestPayload `json:"request"`
	launchRequestPayload
}

type launchRequestResponse struct {
	TemplateID        string   `json:"template_id,omitempty"`
	Mode              string   `json:"mode"`
	Role              string   `json:"role,omitempty"`
	AllowedRoles      []string `json:"allowed_roles,omitempty"`
	Roster            []string `json:"roster,omitempty"`
	SourceTask        string   `json:"source_task,omitempty"`
	SourceSpecs       []string `json:"source_specs,omitempty"`
	MaxSteps          int      `json:"max_steps,omitempty"`
	MaxResolverLoops  int      `json:"max_resolver_loops,omitempty"`
	MaxDuration       string   `json:"max_duration,omitempty"`
	TokenBudget       int      `json:"token_budget,omitempty"`
	StepMaxTurns      int      `json:"step_max_turns,omitempty"`
	StepMaxTokens     int      `json:"step_max_tokens,omitempty"`
	AllowApprovalWait bool     `json:"allow_approval_wait,omitempty"`
}

type launchPreviewResponse struct {
	Mode               string                   `json:"mode"`
	Template           launchTemplateResponse   `json:"template"`
	Role               string                   `json:"role,omitempty"`
	AllowedRoles       []string                 `json:"allowed_roles,omitempty"`
	Roster             []string                 `json:"roster,omitempty"`
	Summary            string                   `json:"summary"`
	CompiledTask       string                   `json:"compiled_task"`
	WorkPacketMarkdown string                   `json:"work_packet_markdown"`
	StepMaxTurns       int                      `json:"step_max_turns,omitempty"`
	StepMaxTokens      int                      `json:"step_max_tokens,omitempty"`
	AllowApprovalWait  bool                     `json:"allow_approval_wait,omitempty"`
	Warnings           []runtimeWarningResponse `json:"warnings"`
}

type launchDraftReadResponse struct {
	Found bool                 `json:"found"`
	Draft *launchDraftResponse `json:"draft,omitempty"`
}

type launchDraftResponse struct {
	ID        string                `json:"id"`
	Request   launchRequestResponse `json:"request"`
	UpdatedAt string                `json:"updated_at,omitempty"`
}

type launchPresetResponse struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Request   launchRequestResponse `json:"request"`
	UpdatedAt string                `json:"updated_at,omitempty"`
}

type launchStartResponse struct {
	ChainID string                `json:"chain_id"`
	Status  string                `json:"status"`
	Preview launchPreviewResponse `json:"preview"`
}

type controlResultResponse struct {
	ChainID        string                   `json:"chain_id"`
	PreviousStatus string                   `json:"previous_status,omitempty"`
	TargetStatus   string                   `json:"target_status,omitempty"`
	Status         string                   `json:"status,omitempty"`
	EventType      string                   `json:"event_type,omitempty"`
	Message        string                   `json:"message"`
	Already        bool                     `json:"already,omitempty"`
	SignaledPIDs   []int                    `json:"signaled_pids,omitempty"`
	Warnings       []runtimeWarningResponse `json:"warnings,omitempty"`
}

func (h *ChainInspectorHandler) handleRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListAgentRoles(r.Context())
	if err != nil {
		h.logger.Warn("list roles", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]agentRoleResponse, 0, len(roles))
	for _, role := range roles {
		out = append(out, agentRoleResponse{Name: role.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleGetLaunchDraft(w http.ResponseWriter, r *http.Request) {
	draft, found, err := h.svc.LoadLaunchDraft(r.Context())
	if err != nil {
		h.logger.Warn("load launch draft", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, launchDraftReadResponse{})
		return
	}
	response := launchDraftResponseFromOperator(draft)
	writeJSON(w, http.StatusOK, launchDraftReadResponse{Found: true, Draft: &response})
}

func (h *ChainInspectorHandler) handlePutLaunchDraft(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeLaunchRequest(w, r)
	if !ok {
		return
	}
	draft, err := h.svc.SaveLaunchDraft(r.Context(), req)
	if err != nil {
		h.logger.Warn("save launch draft", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, launchDraftResponseFromOperator(draft))
}

func (h *ChainInspectorHandler) handleListLaunchPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.svc.ListLaunchPresets(r.Context())
	if err != nil {
		h.logger.Warn("list launch presets", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]launchPresetResponse, 0, len(presets))
	for _, preset := range presets {
		out = append(out, launchPresetResponseFromOperator(preset))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ChainInspectorHandler) handleSaveLaunchPreset(w http.ResponseWriter, r *http.Request) {
	var payload saveLaunchPresetRequest
	if !decodeJSON(w, r, &payload, h.logger) {
		return
	}
	req, err := payload.request()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	preset, err := h.svc.SaveLaunchPreset(r.Context(), payload.Name, req)
	if err != nil {
		h.logger.Warn("save launch preset", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, launchPresetResponseFromOperator(preset))
}

func (h *ChainInspectorHandler) handleLaunchPreview(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeLaunchRequest(w, r)
	if !ok {
		return
	}
	preview, err := h.svc.ValidateLaunch(r.Context(), req)
	if err != nil {
		h.logger.Warn("validate launch", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, launchPreviewResponseFromOperator(preview))
}

func (h *ChainInspectorHandler) handleLaunchStart(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeLaunchRequest(w, r)
	if !ok {
		return
	}
	result, err := h.svc.StartChain(r.Context(), req)
	if err != nil {
		h.logger.Warn("start launch", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, launchStartResponseFromOperator(result))
}

func (h *ChainInspectorHandler) handlePauseChain(w http.ResponseWriter, r *http.Request) {
	h.handleChainControl(w, r, "pause")
}

func (h *ChainInspectorHandler) handleResumeChain(w http.ResponseWriter, r *http.Request) {
	h.handleChainControl(w, r, "resume")
}

func (h *ChainInspectorHandler) handleCancelChain(w http.ResponseWriter, r *http.Request) {
	h.handleChainControl(w, r, "cancel")
}

func (h *ChainInspectorHandler) handleChainControl(w http.ResponseWriter, r *http.Request, action string) {
	chainID := strings.TrimSpace(r.PathValue("id"))
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain id is required")
		return
	}
	var (
		result operator.ControlResult
		err    error
	)
	switch action {
	case "pause":
		result, err = h.svc.PauseChain(r.Context(), chainID)
	case "resume":
		result, err = h.svc.ResumeChain(r.Context(), chainID)
	case "cancel":
		result, err = h.svc.CancelChain(r.Context(), chainID)
	default:
		writeError(w, http.StatusBadRequest, "unsupported chain control action")
		return
	}
	if err != nil {
		h.logger.Warn("chain control", "chain_id", chainID, "action", action, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, controlResultResponseFromOperator(result))
}

func (h *ChainInspectorHandler) decodeLaunchRequest(w http.ResponseWriter, r *http.Request) (operator.LaunchRequest, bool) {
	var payload launchRequestEnvelope
	if !decodeJSON(w, r, &payload, h.logger) {
		return operator.LaunchRequest{}, false
	}
	req, err := payload.request()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return operator.LaunchRequest{}, false
	}
	return req, true
}

func (p launchRequestEnvelope) request() (operator.LaunchRequest, error) {
	payload := p.launchRequestPayload
	if p.Request != nil {
		payload = *p.Request
	}
	return payload.request()
}

func (p saveLaunchPresetRequest) request() (operator.LaunchRequest, error) {
	payload := p.launchRequestPayload
	if p.Request != nil {
		payload = *p.Request
	}
	return payload.request()
}

func (p launchRequestPayload) request() (operator.LaunchRequest, error) {
	var maxDuration time.Duration
	if strings.TrimSpace(p.MaxDuration) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(p.MaxDuration))
		if err != nil {
			return operator.LaunchRequest{}, err
		}
		maxDuration = parsed
	}
	sourceTask := p.SourceTask
	if strings.TrimSpace(sourceTask) == "" {
		sourceTask = p.Task
	}
	sourceSpecs := p.SourceSpecs
	if len(sourceSpecs) == 0 {
		sourceSpecs = p.Specs
	}
	return operator.LaunchRequest{
		TemplateID:        p.TemplateID,
		Mode:              operator.LaunchMode(p.Mode),
		Role:              p.Role,
		AllowedRoles:      append([]string(nil), p.AllowedRoles...),
		Roster:            append([]string(nil), p.Roster...),
		SourceTask:        sourceTask,
		SourceSpecs:       append([]string(nil), sourceSpecs...),
		MaxSteps:          p.MaxSteps,
		MaxResolverLoops:  p.MaxResolverLoops,
		MaxDuration:       maxDuration,
		TokenBudget:       p.TokenBudget,
		StepMaxTurns:      p.StepMaxTurns,
		StepMaxTokens:     p.StepMaxTokens,
		AllowApprovalWait: p.AllowApprovalWait,
	}, nil
}

func launchRequestResponseFromOperator(req operator.LaunchRequest) launchRequestResponse {
	out := launchRequestResponse{
		TemplateID:        req.TemplateID,
		Mode:              string(req.Mode),
		Role:              req.Role,
		AllowedRoles:      append([]string(nil), req.AllowedRoles...),
		Roster:            append([]string(nil), req.Roster...),
		SourceTask:        req.SourceTask,
		SourceSpecs:       append([]string(nil), req.SourceSpecs...),
		MaxSteps:          req.MaxSteps,
		MaxResolverLoops:  req.MaxResolverLoops,
		TokenBudget:       req.TokenBudget,
		StepMaxTurns:      req.StepMaxTurns,
		StepMaxTokens:     req.StepMaxTokens,
		AllowApprovalWait: req.AllowApprovalWait,
	}
	if req.MaxDuration > 0 {
		out.MaxDuration = req.MaxDuration.String()
	}
	return out
}

func launchPreviewResponseFromOperator(preview operator.LaunchPreview) launchPreviewResponse {
	warnings := make([]runtimeWarningResponse, 0, len(preview.Warnings))
	for _, warning := range preview.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	return launchPreviewResponse{
		Mode:               string(preview.Mode),
		Template:           launchTemplateResponseFromOperator(preview.Template),
		Role:               preview.Role,
		AllowedRoles:       append([]string(nil), preview.AllowedRoles...),
		Roster:             append([]string(nil), preview.Roster...),
		Summary:            preview.Summary,
		CompiledTask:       preview.CompiledTask,
		WorkPacketMarkdown: preview.CompiledTask,
		StepMaxTurns:       preview.StepMaxTurns,
		StepMaxTokens:      preview.StepMaxTokens,
		AllowApprovalWait:  preview.AllowApprovalWait,
		Warnings:           warnings,
	}
}

func launchDraftResponseFromOperator(draft operator.LaunchDraft) launchDraftResponse {
	return launchDraftResponse{
		ID:        draft.ID,
		Request:   launchRequestResponseFromOperator(draft.Request),
		UpdatedAt: draft.UpdatedAt,
	}
}

func launchPresetResponseFromOperator(preset operator.LaunchPreset) launchPresetResponse {
	return launchPresetResponse{
		ID:        preset.ID,
		Name:      preset.Name,
		Request:   launchRequestResponseFromOperator(preset.Request),
		UpdatedAt: preset.UpdatedAt,
	}
}

func launchStartResponseFromOperator(result operator.StartResult) launchStartResponse {
	return launchStartResponse{
		ChainID: result.ChainID,
		Status:  result.Status,
		Preview: launchPreviewResponseFromOperator(result.Preview),
	}
}

func controlResultResponseFromOperator(result operator.ControlResult) controlResultResponse {
	warnings := make([]runtimeWarningResponse, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, runtimeWarningResponse{Message: warning.Message})
	}
	return controlResultResponse{
		ChainID:        result.ChainID,
		PreviousStatus: result.PreviousStatus,
		TargetStatus:   result.TargetStatus,
		Status:         result.Status,
		EventType:      string(result.EventType),
		Message:        result.Message,
		Already:        result.Already,
		SignaledPIDs:   append([]int(nil), result.SignaledPIDs...),
		Warnings:       warnings,
	}
}
