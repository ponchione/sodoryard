package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/operator"
)

// DiagnosticsOptions captures deployment metadata that is useful in exported
// support bundles without exposing provider credentials.
type DiagnosticsOptions struct {
	YardVersion string
	ConfigPath  string
}

// DiagnosticsHandler exposes read-only desktop diagnostics and export data.
type DiagnosticsHandler struct {
	cfg       *config.Config
	svc       *operator.Service
	providers ProviderRuntimeInspector
	options   DiagnosticsOptions
	logger    *slog.Logger
}

func NewDiagnosticsHandler(s *Server, cfg *config.Config, svc *operator.Service, providers ProviderRuntimeInspector, options DiagnosticsOptions, logger *slog.Logger) *DiagnosticsHandler {
	h := &DiagnosticsHandler{
		cfg:       cfg,
		svc:       svc,
		providers: providers,
		options:   options,
		logger:    logger,
	}
	if h.logger == nil {
		h.logger = slog.Default()
	}
	if s != nil {
		s.HandleFunc("GET /api/diagnostics", h.handleGet)
		s.HandleFunc("POST /api/diagnostics/export", h.handleExport)
	}
	return h
}

type diagnosticsResponse struct {
	GeneratedAt  string                     `json:"generated_at"`
	YardVersion  string                     `json:"yard_version,omitempty"`
	APIVersion   string                     `json:"api_version"`
	Project      diagnosticsProjectResponse `json:"project"`
	Runtime      runtimeStatusResponse      `json:"runtime"`
	Providers    []authProviderStatus       `json:"providers"`
	RecentChains []chainSummaryResponse     `json:"recent_chains"`
	Warnings     []runtimeWarningResponse   `json:"warnings"`
	Capabilities []string                   `json:"capabilities"`
}

type diagnosticsProjectResponse struct {
	Name              string `json:"name"`
	RootPath          string `json:"root_path"`
	ConfigPath        string `json:"config_path,omitempty"`
	MemoryBackend     string `json:"memory_backend,omitempty"`
	BrainBackend      string `json:"brain_backend,omitempty"`
	LocalServicesMode string `json:"local_services_mode,omitempty"`
}

func (h *DiagnosticsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	body, err := h.snapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *DiagnosticsHandler) handleExport(w http.ResponseWriter, r *http.Request) {
	body, err := h.snapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, diagnosticsFilename(time.Now().UTC())))
	writeJSON(w, http.StatusOK, body)
}

func (h *DiagnosticsHandler) snapshot(ctx context.Context) (diagnosticsResponse, error) {
	if h == nil || h.svc == nil {
		return diagnosticsResponse{}, fmt.Errorf("diagnostics runtime is unavailable")
	}

	status, err := h.svc.RuntimeStatus(ctx)
	if err != nil {
		return diagnosticsResponse{}, fmt.Errorf("runtime status: %w", err)
	}
	warnings := append([]operator.RuntimeWarning(nil), status.Warnings...)

	chains, err := h.svc.ListChains(ctx, 20)
	if err != nil {
		h.logger.Warn("diagnostics chain collection failed", "error", err)
		warnings = append(warnings, operator.RuntimeWarning{
			Message: fmt.Sprintf("recent chain collection failed: %v", err),
		})
	}

	providers, providerWarnings := h.providerDiagnostics(ctx)
	warnings = append(warnings, providerWarnings...)

	response := diagnosticsResponse{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		YardVersion:  h.options.YardVersion,
		APIVersion:   desktopAPIVersion,
		Project:      h.projectDiagnostics(),
		Runtime:      toRuntimeStatusResponse(status),
		Providers:    providers,
		RecentChains: chainSummariesFromOperator(chains),
		Warnings:     runtimeWarnings(warnings),
		Capabilities: diagnosticCapabilities(),
	}
	return response, nil
}

func (h *DiagnosticsHandler) projectDiagnostics() diagnosticsProjectResponse {
	if h == nil || h.cfg == nil {
		return diagnosticsProjectResponse{}
	}
	cfgPath := h.options.ConfigPath
	if cfgPath != "" {
		cfgPath = cleanCapabilityPath(filepath.Clean(cfgPath))
	}
	return diagnosticsProjectResponse{
		Name:              h.cfg.ProjectName(),
		RootPath:          cleanCapabilityPath(h.cfg.ProjectRoot),
		ConfigPath:        cfgPath,
		MemoryBackend:     h.cfg.Memory.Backend,
		BrainBackend:      h.cfg.Brain.Backend,
		LocalServicesMode: h.cfg.LocalServices.Mode,
	}
}

func (h *DiagnosticsHandler) providerDiagnostics(ctx context.Context) ([]authProviderStatus, []operator.RuntimeWarning) {
	if h == nil || h.cfg == nil {
		return nil, nil
	}

	data := providerRuntimeData{}
	warnings := make([]operator.RuntimeWarning, 0)
	if h.providers != nil {
		data.health = h.providers.ProviderHealthMap()
		statuses, err := h.providers.AuthStatuses(ctx)
		if err != nil {
			h.logger.Warn("diagnostics provider auth collection failed", "error", err)
			warnings = append(warnings, operator.RuntimeWarning{
				Message: fmt.Sprintf("provider credential status collection failed: %v", err),
			})
		} else {
			data.authStatuses = statuses
		}
	}

	names := h.cfg.ProviderNamesForSurfaces()
	providers := make([]authProviderStatus, 0, len(names))
	for _, name := range names {
		pc, ok := h.cfg.Providers[name]
		if !ok {
			continue
		}
		status, healthy, lastError := providerHealthSummary(data.health[name])
		providers = append(providers, authProviderStatus{
			Name:      name,
			Type:      pc.Type,
			Status:    status,
			Healthy:   healthy,
			LastError: lastError,
			Auth:      data.authStatuses[name],
		})
	}
	return providers, warnings
}

func chainSummariesFromOperator(chains []operator.ChainSummary) []chainSummaryResponse {
	out := make([]chainSummaryResponse, 0, len(chains))
	for _, chain := range chains {
		out = append(out, chainSummaryResponseFromOperator(chain))
	}
	return out
}

func runtimeWarnings(warnings []operator.RuntimeWarning) []runtimeWarningResponse {
	out := make([]runtimeWarningResponse, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, runtimeWarningResponse{
			Message: warning.Message,
		})
	}
	return out
}

func diagnosticCapabilities() []string {
	return []string{
		"runtime_status",
		"chains_read",
		"provider_credentials",
		"diagnostics",
		"diagnostics_export",
	}
}

func diagnosticsFilename(now time.Time) string {
	return "yard-diagnostics-" + now.UTC().Format("20060102T150405Z") + ".json"
}
