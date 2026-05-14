package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/protocol"

	"github.com/ponchione/sodoryard/internal/config"
)

const desktopAPIVersion = "desktop-v1"

type DesktopOptions struct {
	YardVersion string
	ConfigPath  string
}

type DesktopHandler struct {
	cfg     *config.Config
	backend any
	options DesktopOptions
}

type desktopCapabilitiesResponse struct {
	YardVersion   string                        `json:"yard_version"`
	APIVersion    string                        `json:"api_version"`
	ProjectRoot   string                        `json:"project_root"`
	ConfigPath    string                        `json:"config_path"`
	Capabilities  []string                      `json:"capabilities"`
	ProjectMemory *desktopProjectMemoryResponse `json:"project_memory,omitempty"`
}

type desktopProjectMemoryResponse struct {
	Backend               string   `json:"backend"`
	Module                string   `json:"module,omitempty"`
	SchemaVersion         uint32   `json:"schema_version,omitempty"`
	ContractVersion       uint32   `json:"contract_version,omitempty"`
	ShunterVersion        string   `json:"shunter_version,omitempty"`
	DefaultSubprotocol    string   `json:"default_subprotocol,omitempty"`
	SupportedSubprotocols []string `json:"supported_subprotocols,omitempty"`
	SubscribeURL          string   `json:"subscribe_url,omitempty"`
	GeneratedBindingHash  string   `json:"generated_binding_hash,omitempty"`
}

func NewDesktopHandler(s *Server, cfg *config.Config, backend any, options DesktopOptions) *DesktopHandler {
	if s == nil {
		return nil
	}
	h := &DesktopHandler{
		cfg:     cfg,
		backend: backend,
		options: options,
	}
	s.HandleFunc("GET /api/desktop/capabilities", h.handleCapabilities)
	return h
}

func (h *DesktopHandler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, desktopCapabilitiesResponse{
		YardVersion:   h.yardVersion(),
		APIVersion:    desktopAPIVersion,
		ProjectRoot:   h.projectRoot(),
		ConfigPath:    cleanCapabilityPath(h.options.ConfigPath),
		Capabilities:  h.capabilities(),
		ProjectMemory: h.projectMemory(r),
	})
}

func (h *DesktopHandler) capabilities() []string {
	capabilities := []string{
		"runtime_status",
		"runtime_local_services",
		"conversation_chat",
		"chains_read",
		"chains_control",
		"launch_preview",
		"launch_start",
		"launch_drafts",
		"launch_presets",
		"project_tree",
		"project_file_preview",
		"project_path_validation",
		"context_reports",
		"metrics",
	}
	if _, ok := h.backend.(projectMemoryContractExporter); ok {
		capabilities = append(capabilities, "project_memory_contract")
	}
	if protocolBackend, ok := h.backend.(projectMemoryProtocolHandler); ok && protocolBackend.ProtocolEnabled() {
		capabilities = append(capabilities, "project_memory_protocol", "project_memory_subscriptions")
	}
	return capabilities
}

func (h *DesktopHandler) projectMemory(r *http.Request) *desktopProjectMemoryResponse {
	if h.cfg == nil || h.cfg.Memory.Backend == "" {
		return nil
	}
	out := &desktopProjectMemoryResponse{Backend: h.cfg.Memory.Backend}
	contract, hash := exportedProjectMemoryContract(h.backend)
	if contract != nil {
		out.Module = contract.Module.Name
		out.SchemaVersion = contract.Schema.Version
		out.ContractVersion = contract.ContractVersion
		out.GeneratedBindingHash = hash
	}
	if protocolBackend, ok := h.backend.(projectMemoryProtocolHandler); ok && protocolBackend.ProtocolEnabled() {
		out.ShunterVersion = shunter.CurrentBuildInfo().Version
		out.DefaultSubprotocol = protocol.SubprotocolV2
		out.SupportedSubprotocols = protocol.SupportedSubprotocols()
		out.SubscribeURL = projectMemorySubscribeURL(r)
	}
	return out
}

func exportedProjectMemoryContract(backend any) (*projectMemoryContractMetadata, string) {
	exporter, ok := backend.(projectMemoryContractExporter)
	if !ok {
		return nil, ""
	}
	data, err := exporter.ExportContractJSON()
	if err != nil {
		return nil, ""
	}
	var contract projectMemoryContractMetadata
	if err := json.Unmarshal(data, &contract); err != nil {
		return nil, ""
	}
	sum := sha256.Sum256(data)
	return &contract, "sha256:" + hex.EncodeToString(sum[:])
}

type projectMemoryContractMetadata struct {
	ContractVersion uint32 `json:"contract_version"`
	Module          struct {
		Name string `json:"name"`
	} `json:"module"`
	Schema struct {
		Version uint32 `json:"version"`
	} `json:"schema"`
}

func projectMemorySubscribeURL(r *http.Request) string {
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s/api/project-memory/subscribe", scheme, r.Host)
}

func (h *DesktopHandler) yardVersion() string {
	if h.options.YardVersion != "" {
		return h.options.YardVersion
	}
	return "dev"
}

func (h *DesktopHandler) projectRoot() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.ProjectRoot
}

func cleanCapabilityPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
