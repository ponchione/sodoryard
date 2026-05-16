package server_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/chainrun"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/ponchione/sodoryard/internal/operator"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/server"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

type chainTestBrain struct {
	docs map[string]string
}

type fakeLocalServicesManager struct {
	status      localservices.StackStatus
	upStatus    localservices.StackStatus
	logs        string
	statusCalls int
	upCalls     int
	downCalls   int
	logCalls    int
	lastLogTail int
}

func (f *fakeLocalServicesManager) Status(context.Context, *config.Config) (localservices.StackStatus, error) {
	f.statusCalls++
	return f.status, nil
}

func (f *fakeLocalServicesManager) EnsureUp(context.Context, *config.Config) (localservices.StackStatus, error) {
	f.upCalls++
	if f.upStatus.Mode != "" {
		return f.upStatus, nil
	}
	return f.status, nil
}

func (f *fakeLocalServicesManager) Down(context.Context, *config.Config) error {
	f.downCalls++
	return nil
}

func (f *fakeLocalServicesManager) Logs(_ context.Context, _ *config.Config, tail int) (string, error) {
	f.logCalls++
	f.lastLogTail = tail
	return f.logs, nil
}

func (b *chainTestBrain) ReadDocument(_ context.Context, path string) (string, error) {
	content, ok := b.docs[path]
	if !ok {
		return "", fmt.Errorf("missing document %s", path)
	}
	return content, nil
}

func (b *chainTestBrain) WriteDocument(_ context.Context, path string, content string) error {
	b.docs[path] = content
	return nil
}

func (b *chainTestBrain) PatchDocument(context.Context, string, string, string) error {
	return nil
}

func (b *chainTestBrain) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (b *chainTestBrain) ListDocuments(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestChainInspectorEndpoints(t *testing.T) {
	ctx := context.Background()
	db := newChainInspectorTestDB(t)
	store := chain.NewStore(db)
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-web", SourceTask: "inspect"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	receiptPath := "receipts/coder/chain-web-step-001.md"
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "accepted", ReceiptPath: receiptPath, TokensUsed: 42, DurationSecs: 5}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 1, TotalTokens: 42, TotalDurationSecs: 5}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventFindingLifecycleFacts, map[string]any{
		"role": "correctness-auditor",
		"facts": []map[string]any{{
			"id":          "FIND-correctness-001",
			"source_role": "correctness-auditor",
			"action":      "opened",
			"status":      "open",
			"severity":    "high",
			"evidence":    "internal/example.go:42",
			"summary":     "nil panic",
		}},
	}); err != nil {
		t.Fatalf("LogEvent finding facts returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepGuardrailFacts, map[string]any{
		"role":                                 "coder",
		"sequence":                             1,
		"source_mutating":                      true,
		"exit_code":                            0,
		"duration_secs":                        5,
		"receipt_present":                      true,
		"synthetic_receipt_written":            false,
		"receipt_valid":                        true,
		"receipt_schema_valid":                 true,
		"receipt_step_valid":                   true,
		"receipt_sections_valid":               true,
		"parsed_verdict":                       "completed",
		"tokens_used":                          13,
		"turns_used":                           2,
		"receipt_duration_seconds":             4,
		"claimed_validation_commands":          []string{"rtk make test"},
		"changed_file_manifest_present":        true,
		"changed_file_count":                   1,
		"changed_files":                        []string{"internal/example.go"},
		"code_index_dirty_mark_supported":      true,
		"code_index_dirty_mark_attempted":      true,
		"code_index_dirty_marked":              true,
		"code_index_state_supported":           true,
		"code_index_state_found":               true,
		"code_index_dirty":                     true,
		"code_index_dirty_reason":              "source_write",
		"source_writer_lock_release_attempted": true,
		"source_writer_lock_released":          true,
		"finding_count":                        1,
		"open_finding_count":                   1,
		"finding_ids":                          []string{"FIND-correctness-001"},
		"open_finding_ids":                     []string{"FIND-correctness-001"},
	}); err != nil {
		t.Fatalf("LogEvent guardrail facts returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventApprovalRequired, map[string]any{
		"approval_id":     "approval-web-1",
		"tool_name":       "shell",
		"tool_input":      map[string]any{"command": "git push --force"},
		"reason":          "shell command matches approval policy",
		"risk_level":      "high",
		"status":          chain.ApprovalStatusPending,
		"conversation_id": "conv-web",
		"turn_number":     1,
		"iteration":       1,
	}); err != nil {
		t.Fatalf("LogEvent approval required returned error: %v", err)
	}
	if err := store.CompleteChain(ctx, chainID, "completed", "done"); err != nil {
		t.Fatalf("CompleteChain returned error: %v", err)
	}
	traceRecorder := tracepkg.NewSQLiteRecorder(db)
	spanCtx := tracepkg.ContextWithScope(ctx, tracepkg.Scope{ChainID: chainID, StepID: stepID, ConversationID: "conv-web", TurnNumber: 1, Iteration: 1})
	spanCtx, span := tracepkg.StartSpan(spanCtx, traceRecorder, tracepkg.SpanStart{Name: "provider.stream", Kind: tracepkg.KindProvider})
	span.End(spanCtx, tracepkg.StatusError, fmt.Errorf("provider failed"))

	cfg := &config.Config{
		ProjectRoot: t.TempDir(),
		Routing: config.RoutingConfig{
			Default: config.RouteConfig{Provider: "codex", Model: "test-model"},
		},
	}
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:        cfg,
		Database:      db,
		ChainStore:    store,
		BrainBackend:  &chainTestBrain{docs: map[string]string{receiptPath: "receipt content"}},
		TraceRecorder: traceRecorder,
		Cleanup:       func() {},
	}, operator.Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var chains []struct {
		ID                string   `json:"id"`
		Status            string   `json:"status"`
		Roles             []string `json:"roles"`
		TotalDurationSecs int      `json:"total_duration_secs"`
		ReceiptCount      int      `json:"receipt_count"`
		LastEventType     string   `json:"last_event_type"`
		LastEventAt       string   `json:"last_event_at"`
	}
	getJSON(t, base+"/api/chains", &chains)
	if len(chains) != 1 || chains[0].ID != chainID || chains[0].Status != "completed" {
		t.Fatalf("chains response = %+v, want completed chain-web", chains)
	}
	if chains[0].TotalDurationSecs != 5 || chains[0].ReceiptCount != 1 {
		t.Fatalf("chains summary indicators = %+v, want duration 5 and one receipt", chains[0])
	}
	if len(chains[0].Roles) != 1 || chains[0].Roles[0] != "coder" {
		t.Fatalf("chains summary roles = %+v, want coder", chains[0].Roles)
	}
	if chains[0].LastEventType != string(chain.EventApprovalRequired) || chains[0].LastEventAt == "" {
		t.Fatalf("chains summary last event = %+v, want approval_required timestamp", chains[0])
	}

	var templates []struct {
		ID              string          `json:"id"`
		Mode            string          `json:"mode"`
		Label           string          `json:"label"`
		InputSchema     json.RawMessage `json:"input_schema"`
		ReceiptSchema   string          `json:"receipt_schema"`
		PreflightChecks []string        `json:"preflight_checks"`
	}
	getJSON(t, base+"/api/chains/templates", &templates)
	if len(templates) != 4 {
		t.Fatalf("templates response = %+v, want 4 launch templates", templates)
	}
	if templates[0].ID != "constrained_orchestration" || templates[0].Mode != string(operator.LaunchModeConstrained) || templates[0].ReceiptSchema != "yard.receipt.v1" {
		t.Fatalf("first template = %+v, want constrained launch template metadata", templates[0])
	}
	if len(templates[0].PreflightChecks) == 0 || templates[0].Label == "" {
		t.Fatalf("first template = %+v, want label and preflight checks", templates[0])
	}
	if !json.Valid(templates[0].InputSchema) || !strings.Contains(string(templates[0].InputSchema), `"allowed_roles"`) {
		t.Fatalf("first template input schema = %s, want constrained schema", templates[0].InputSchema)
	}

	var detail struct {
		Chain struct {
			ID string `json:"id"`
		} `json:"chain"`
		Steps []struct {
			Role        string `json:"role"`
			ReceiptPath string `json:"receipt_path"`
		} `json:"steps"`
		Receipts []struct {
			Step string `json:"step"`
			Path string `json:"path"`
		} `json:"receipts"`
		Approvals []struct {
			ID             string          `json:"id"`
			ChainID        string          `json:"chain_id"`
			StepID         string          `json:"step_id"`
			ConversationID string          `json:"conversation_id"`
			TurnNumber     int             `json:"turn_number"`
			Iteration      int             `json:"iteration"`
			ToolName       string          `json:"tool_name"`
			ToolInput      json.RawMessage `json:"tool_input"`
			Reason         string          `json:"reason"`
			RiskLevel      string          `json:"risk_level"`
			Status         string          `json:"status"`
			DecisionReason string          `json:"decision_reason"`
			DecidedBy      string          `json:"decided_by"`
		} `json:"approvals"`
		Guardrails struct {
			Findings []struct {
				ID       string `json:"id"`
				Severity string `json:"severity"`
				Evidence string `json:"evidence"`
			} `json:"findings"`
			StepFacts []struct {
				SequenceNum                 int      `json:"sequence_num"`
				Role                        string   `json:"role"`
				ExitCode                    int      `json:"exit_code"`
				DurationSecs                int      `json:"duration_secs"`
				ReceiptPresent              bool     `json:"receipt_present"`
				SyntheticReceiptWritten     bool     `json:"synthetic_receipt_written"`
				ReceiptSchemaValid          bool     `json:"receipt_schema_valid"`
				ReceiptStepValid            bool     `json:"receipt_step_valid"`
				ReceiptSectionsValid        bool     `json:"receipt_sections_valid"`
				ParsedVerdict               string   `json:"parsed_verdict"`
				TokensUsed                  int      `json:"tokens_used"`
				TurnsUsed                   int      `json:"turns_used"`
				ReceiptDurationSeconds      int      `json:"receipt_duration_seconds"`
				ClaimedValidationCommands   []string `json:"claimed_validation_commands"`
				CodeIndexDirtyMarkSupported bool     `json:"code_index_dirty_mark_supported"`
				CodeIndexDirtyMarkAttempted bool     `json:"code_index_dirty_mark_attempted"`
				CodeIndexDirtyMarked        bool     `json:"code_index_dirty_marked"`
				CodeIndexDirty              bool     `json:"code_index_dirty"`
				CodeIndexDirtyReason        string   `json:"code_index_dirty_reason"`
				ChangedFiles                []string `json:"changed_files"`
				SourceWriterLockReleased    bool     `json:"source_writer_lock_released"`
				FindingCount                int      `json:"finding_count"`
				OpenFindingCount            int      `json:"open_finding_count"`
				FindingIDs                  []string `json:"finding_ids"`
				OpenFindingIDs              []string `json:"open_finding_ids"`
			} `json:"step_facts"`
		} `json:"guardrails"`
		Metrics struct {
			ChainID                    string   `json:"chain_id"`
			Health                     string   `json:"health"`
			TotalSteps                 int      `json:"total_steps"`
			StepRows                   int      `json:"step_rows"`
			TotalTokens                int      `json:"total_tokens"`
			StepTokenTotal             int      `json:"step_token_total"`
			StepTurnTotal              int      `json:"step_turn_total"`
			EventTotal                 int      `json:"event_total"`
			StepGuardrailFactEvents    int      `json:"step_guardrail_fact_events"`
			FindingLifecycleFactEvents int      `json:"finding_lifecycle_fact_events"`
			OpenFindingIDs             []string `json:"open_finding_ids"`
			Warnings                   []struct {
				Message string `json:"message"`
			} `json:"warnings"`
			Steps []struct {
				SequenceNum int    `json:"sequence_num"`
				Role        string `json:"role"`
				TokensUsed  int    `json:"tokens_used"`
				TurnsUsed   int    `json:"turns_used"`
			} `json:"steps"`
		} `json:"metrics"`
		Timeline []struct {
			Source     string `json:"source"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			StepID     string `json:"step_id"`
			DurationMs int64  `json:"duration_ms"`
			Error      string `json:"error"`
		} `json:"timeline"`
	}
	getJSON(t, base+"/api/chains/"+chainID, &detail)
	if detail.Chain.ID != chainID || len(detail.Steps) != 1 || detail.Steps[0].Role != "coder" {
		t.Fatalf("detail response = %+v, want chain detail", detail)
	}
	if len(detail.Receipts) != 1 || detail.Receipts[0].Path != receiptPath {
		t.Fatalf("receipts = %+v, want step receipt", detail.Receipts)
	}
	if len(detail.Approvals) != 1 || detail.Approvals[0].ID != "approval-web-1" || detail.Approvals[0].Status != chain.ApprovalStatusPending || detail.Approvals[0].ToolName != "shell" || detail.Approvals[0].RiskLevel != "high" {
		t.Fatalf("approvals = %+v, want pending shell approval", detail.Approvals)
	}
	if detail.Approvals[0].ChainID != chainID || detail.Approvals[0].StepID != stepID || detail.Approvals[0].ConversationID != "conv-web" || detail.Approvals[0].TurnNumber != 1 || detail.Approvals[0].Iteration != 1 {
		t.Fatalf("approval scope = %+v, want chain/step/runtime scope", detail.Approvals[0])
	}
	if !strings.Contains(string(detail.Approvals[0].ToolInput), "git push --force") {
		t.Fatalf("approval tool input = %s, want shell command", detail.Approvals[0].ToolInput)
	}
	if len(detail.Guardrails.Findings) != 1 || detail.Guardrails.Findings[0].ID != "FIND-correctness-001" || detail.Guardrails.Findings[0].Severity != "high" || detail.Guardrails.Findings[0].Evidence != "internal/example.go:42" {
		t.Fatalf("guardrail findings = %+v, want lifecycle detail", detail.Guardrails.Findings)
	}
	if len(detail.Guardrails.StepFacts) != 1 {
		t.Fatalf("guardrail step facts = %+v, want one fact event", detail.Guardrails.StepFacts)
	}
	facts := detail.Guardrails.StepFacts[0]
	if facts.SequenceNum != 1 || facts.Role != "coder" || !facts.CodeIndexDirtyMarkSupported || !facts.CodeIndexDirtyMarkAttempted || !facts.CodeIndexDirtyMarked || !facts.CodeIndexDirty || facts.CodeIndexDirtyReason != "source_write" || !facts.SourceWriterLockReleased {
		t.Fatalf("guardrail step facts = %+v, want index mark and lock facts", facts)
	}
	if facts.ExitCode != 0 || facts.DurationSecs != 5 || !facts.ReceiptPresent || facts.SyntheticReceiptWritten || !facts.ReceiptSchemaValid || !facts.ReceiptStepValid || !facts.ReceiptSectionsValid || facts.ParsedVerdict != "completed" || facts.TokensUsed != 13 || facts.TurnsUsed != 2 || facts.ReceiptDurationSeconds != 4 || len(facts.ClaimedValidationCommands) != 1 || facts.ClaimedValidationCommands[0] != "rtk make test" {
		t.Fatalf("guardrail receipt/run facts = %+v, want surfaced post-step metadata", facts)
	}
	if facts.FindingCount != 1 || facts.OpenFindingCount != 1 || len(facts.FindingIDs) != 1 || facts.FindingIDs[0] != "FIND-correctness-001" || len(facts.OpenFindingIDs) != 1 || facts.OpenFindingIDs[0] != "FIND-correctness-001" {
		t.Fatalf("guardrail finding facts = %+v, want finding counts and ids", facts)
	}
	if len(facts.ChangedFiles) != 1 || facts.ChangedFiles[0] != "internal/example.go" {
		t.Fatalf("guardrail changed files = %+v, want internal/example.go", facts.ChangedFiles)
	}
	if detail.Metrics.ChainID != chainID || detail.Metrics.Health != "attention" || detail.Metrics.TotalSteps != 1 || detail.Metrics.StepRows != 1 || detail.Metrics.TotalTokens != 42 || detail.Metrics.StepTokenTotal != 42 || detail.Metrics.EventTotal != 3 || detail.Metrics.StepGuardrailFactEvents != 1 || detail.Metrics.FindingLifecycleFactEvents != 1 {
		t.Fatalf("metrics = %+v, want serialized chain metrics report", detail.Metrics)
	}
	if len(detail.Metrics.Steps) != 1 || detail.Metrics.Steps[0].SequenceNum != 1 || detail.Metrics.Steps[0].Role != "coder" || detail.Metrics.Steps[0].TokensUsed != 42 {
		t.Fatalf("metric steps = %+v, want step token summary", detail.Metrics.Steps)
	}
	if len(detail.Metrics.OpenFindingIDs) != 1 || detail.Metrics.OpenFindingIDs[0] != "FIND-correctness-001" {
		t.Fatalf("metric open findings = %+v, want open finding id", detail.Metrics.OpenFindingIDs)
	}
	if len(detail.Metrics.Warnings) == 0 {
		t.Fatalf("metric warnings = %+v, want dogfooding warnings", detail.Metrics.Warnings)
	}
	var metrics struct {
		ChainID        string `json:"chain_id"`
		Health         string `json:"health"`
		StepTokenTotal int    `json:"step_token_total"`
		Warnings       []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	}
	getJSON(t, base+"/api/chains/"+chainID+"/metrics", &metrics)
	if metrics.ChainID != chainID || metrics.Health != "attention" || metrics.StepTokenTotal != 42 || len(metrics.Warnings) == 0 {
		t.Fatalf("metrics endpoint = %+v, want chain metrics report", metrics)
	}
	var sawTimelineSpan bool
	for _, item := range detail.Timeline {
		if item.Source == "span" && item.Kind == tracepkg.KindProvider && item.Name == "provider.stream" && item.Status == tracepkg.StatusError && item.StepID == stepID && item.Error == "provider failed" {
			sawTimelineSpan = true
		}
	}
	if !sawTimelineSpan {
		t.Fatalf("timeline = %+v, want serialized provider failure span", detail.Timeline)
	}

	var timeline []struct {
		Source string `json:"source"`
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		Status string `json:"status"`
		StepID string `json:"step_id"`
		Error  string `json:"error"`
	}
	getJSON(t, base+"/api/chains/"+chainID+"/timeline", &timeline)
	sawTimelineEndpointSpan := false
	for _, item := range timeline {
		if item.Source == "span" && item.Kind == tracepkg.KindProvider && item.Name == "provider.stream" && item.Status == tracepkg.StatusError && item.StepID == stepID && item.Error == "provider failed" {
			sawTimelineEndpointSpan = true
		}
	}
	if !sawTimelineEndpointSpan {
		t.Fatalf("timeline endpoint = %+v, want provider failure span", timeline)
	}

	var events []struct {
		ID        int64  `json:"id"`
		EventType string `json:"event_type"`
	}
	getJSON(t, base+"/api/chains/"+chainID+"/events", &events)
	if len(events) != 3 || events[0].EventType != string(chain.EventFindingLifecycleFacts) || events[1].EventType != string(chain.EventStepGuardrailFacts) || events[2].EventType != string(chain.EventApprovalRequired) {
		t.Fatalf("events endpoint = %+v, want finding, guardrail, and approval events", events)
	}
	var eventsAfter []struct {
		ID        int64  `json:"id"`
		EventType string `json:"event_type"`
	}
	getJSON(t, fmt.Sprintf("%s/api/chains/%s/events?after_id=%d", base, chainID, events[1].ID), &eventsAfter)
	if len(eventsAfter) != 1 || eventsAfter[0].ID != events[2].ID {
		t.Fatalf("events after cursor = %+v, want only approval event %+v", eventsAfter, events[2])
	}
	eventsAfter = nil
	getJSON(t, fmt.Sprintf("%s/api/chains/%s/events?after=%d", base, chainID, events[1].ID), &eventsAfter)
	if len(eventsAfter) != 1 || eventsAfter[0].ID != events[2].ID {
		t.Fatalf("events after alias = %+v, want only approval event %+v", eventsAfter, events[2])
	}

	var receipt struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	getJSON(t, base+"/api/chains/"+chainID+"/receipt?step=1", &receipt)
	if receipt.Path != receiptPath || receipt.Content != "receipt content" {
		t.Fatalf("receipt = %+v, want content", receipt)
	}
	receipt = struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{}
	getJSON(t, base+"/api/chains/"+chainID+"/receipts/1", &receipt)
	if receipt.Path != receiptPath || receipt.Content != "receipt content" {
		t.Fatalf("receipt step alias = %+v, want content", receipt)
	}

	var decision struct {
		Message  string `json:"message"`
		Approval struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			DecisionReason string `json:"decision_reason"`
			DecidedBy      string `json:"decided_by"`
		} `json:"approval"`
	}
	postJSON(t, base+"/api/chains/"+chainID+"/approvals/approval-web-1/approve", `{"reason":"reviewed in browser"}`, &decision)
	if decision.Message != "approval approval-web-1 approved" || decision.Approval.ID != "approval-web-1" || decision.Approval.Status != chain.ApprovalStatusApproved || decision.Approval.DecisionReason != "reviewed in browser" || decision.Approval.DecidedBy != "operator" {
		t.Fatalf("approval decision = %+v, want approved response", decision)
	}

	var decidedDetail struct {
		Approvals []struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			DecisionReason string `json:"decision_reason"`
		} `json:"approvals"`
	}
	getJSON(t, base+"/api/chains/"+chainID, &decidedDetail)
	if len(decidedDetail.Approvals) != 1 || decidedDetail.Approvals[0].ID != "approval-web-1" || decidedDetail.Approvals[0].Status != chain.ApprovalStatusApproved || decidedDetail.Approvals[0].DecisionReason != "reviewed in browser" {
		t.Fatalf("decided detail approvals = %+v, want approved approval", decidedDetail.Approvals)
	}
}

func TestLaunchOperatorEndpoints(t *testing.T) {
	ctx := context.Background()
	db := newChainInspectorTestDB(t)
	store := chain.NewStore(db)
	projectRoot := t.TempDir()
	cfg := &config.Config{
		ProjectRoot: projectRoot,
		Routing: config.RoutingConfig{
			Default: config.RouteConfig{Provider: "codex", Model: "test-model"},
		},
		Providers: map[string]config.ProviderConfig{
			"codex": {Type: "codex", Model: "test-model"},
		},
		AgentRoles: map[string]config.AgentRoleConfig{
			"coder":        {SystemPrompt: "prompts/coder.md"},
			"orchestrator": {SystemPrompt: "prompts/orchestrator.md"},
		},
	}
	var gotStart chainrun.Options
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     db,
		ChainStore:   store,
		BrainBackend: &chainTestBrain{docs: map[string]string{}},
		Cleanup:      func() {},
	}, operator.Options{
		ChainStarter: func(ctx context.Context, cfg *config.Config, opts chainrun.Options, deps chainrun.Deps) (*chainrun.Result, error) {
			gotStart = opts
			chainID := opts.ChainID
			if chainID == "" {
				chainID = "launch-http-chain"
				if _, err := store.StartChain(context.Background(), chain.ChainSpec{ChainID: chainID, SourceTask: opts.SourceTask, SourceSpecs: opts.SourceSpecs}); err != nil {
					return nil, err
				}
			} else if err := store.SetChainStatus(context.Background(), chainID, "running"); err != nil {
				return nil, err
			}
			if opts.OnChainID != nil {
				opts.OnChainID(chainID)
			}
			return &chainrun.Result{ChainID: chainID, Status: "running"}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var roles []struct {
		Name string `json:"name"`
	}
	getJSON(t, base+"/api/roles", &roles)
	if len(roles) != 2 || roles[0].Name != "coder" || roles[1].Name != "orchestrator" {
		t.Fatalf("roles = %+v, want sorted coder/orchestrator", roles)
	}

	var emptyDraft struct {
		Found bool `json:"found"`
	}
	getJSON(t, base+"/api/launch/draft", &emptyDraft)
	if emptyDraft.Found {
		t.Fatalf("empty draft found = true, want false")
	}

	var draft struct {
		ID      string `json:"id"`
		Request struct {
			Mode         string `json:"mode"`
			Role         string `json:"role"`
			SourceTask   string `json:"source_task"`
			StepMaxTurns int    `json:"step_max_turns"`
		} `json:"request"`
	}
	putJSON(t, base+"/api/launch/draft", `{"template_id":"one_step","role":"coder","task":"draft task","step_max_turns":2}`, &draft)
	if draft.ID != "current" || draft.Request.Mode != string(operator.LaunchModeOneStep) || draft.Request.Role != "coder" || draft.Request.SourceTask != "draft task" || draft.Request.StepMaxTurns != 2 {
		t.Fatalf("draft = %+v, want saved current one-step draft", draft)
	}

	var loadedDraft struct {
		Found bool `json:"found"`
		Draft struct {
			ID      string `json:"id"`
			Request struct {
				SourceTask string `json:"source_task"`
			} `json:"request"`
		} `json:"draft"`
	}
	getJSON(t, base+"/api/launch/draft", &loadedDraft)
	if !loadedDraft.Found || loadedDraft.Draft.ID != "current" || loadedDraft.Draft.Request.SourceTask != "draft task" {
		t.Fatalf("loaded draft = %+v, want saved draft", loadedDraft)
	}

	var preset struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Request struct {
			Mode          string   `json:"mode"`
			Roster        []string `json:"roster"`
			StepMaxTokens int      `json:"step_max_tokens"`
		} `json:"request"`
	}
	postJSON(t, base+"/api/launch/presets", `{"name":"audit pair","request":{"mode":"manual_roster","roster":["coder","orchestrator"],"step_max_tokens":4000}}`, &preset)
	if preset.ID != "custom:audit pair" || preset.Name != "audit pair" || preset.Request.Mode != string(operator.LaunchModeManualRoster) || len(preset.Request.Roster) != 2 || preset.Request.StepMaxTokens != 0 {
		t.Fatalf("preset = %+v, want saved manual roster preset with template limits dropped", preset)
	}

	var presets []struct {
		Name string `json:"name"`
	}
	getJSON(t, base+"/api/launch/presets", &presets)
	if len(presets) != 1 || presets[0].Name != "audit pair" {
		t.Fatalf("presets = %+v, want audit pair", presets)
	}

	var preview struct {
		Mode               string   `json:"mode"`
		Role               string   `json:"role"`
		AllowedRoles       []string `json:"allowed_roles"`
		Summary            string   `json:"summary"`
		CompiledTask       string   `json:"compiled_task"`
		WorkPacketMarkdown string   `json:"work_packet_markdown"`
		Template           struct {
			ID string `json:"id"`
		} `json:"template"`
	}
	postJSON(t, base+"/api/launch/preview", `{"mode":"constrained_orchestration","allowed_roles":[" coder ","coder"],"source_task":"ship constrained"}`, &preview)
	if preview.Mode != string(operator.LaunchModeConstrained) || preview.Role != "orchestrator" || len(preview.AllowedRoles) != 1 || preview.AllowedRoles[0] != "coder" || preview.Template.ID != "constrained_orchestration" {
		t.Fatalf("preview = %+v, want normalized constrained launch preview", preview)
	}
	if !strings.Contains(preview.CompiledTask, "Allowed roles: coder") || !strings.Contains(preview.WorkPacketMarkdown, "Work packet") || !strings.Contains(preview.WorkPacketMarkdown, "Allowed roles:") {
		t.Fatalf("preview packet = %+v, want compiled task and work packet markdown", preview)
	}

	var structuredPreview struct {
		Mode             string   `json:"mode"`
		Roster           []string `json:"roster"`
		SourceTask       string   `json:"source_task"`
		SourceSpecs      []string `json:"source_specs"`
		RunSheetMarkdown string   `json:"run_sheet_markdown"`
		Steps            []struct {
			Sequence         int      `json:"sequence"`
			Role             string   `json:"role"`
			Note             string   `json:"note"`
			DossierSources   []string `json:"dossier_sources"`
			EffectiveSources []string `json:"effective_sources"`
		} `json:"steps"`
	}
	postJSON(t, base+"/api/launch/preview", `{"mode":"manual_roster","source_task":" ship composer ","source_specs":["README.md","README.md"],"roster":["coder","orchestrator","coder"],"steps":[{"role":" coder ","note":" plan ","sources":["docs/a.md","docs/a.md"]},{"role":"orchestrator"},{"role":"coder"}]}`, &structuredPreview)
	if structuredPreview.Mode != string(operator.LaunchModeManualRoster) || structuredPreview.SourceTask != "ship composer" || len(structuredPreview.Steps) != 3 || len(structuredPreview.Roster) != 3 {
		t.Fatalf("structured preview = %+v, want normalized manual roster response", structuredPreview)
	}
	if structuredPreview.Steps[0].Role != "coder" || structuredPreview.Steps[0].Note != "plan" || len(structuredPreview.Steps[0].DossierSources) != 1 || structuredPreview.Steps[2].Role != "coder" {
		t.Fatalf("structured preview steps = %+v, want trimmed dossier and duplicate coder preserved", structuredPreview.Steps)
	}
	if !strings.Contains(structuredPreview.RunSheetMarkdown, "Run sheet") || !strings.Contains(structuredPreview.RunSheetMarkdown, "prior receipts: 2") {
		t.Fatalf("structured run sheet = %q, want prior receipt summary", structuredPreview.RunSheetMarkdown)
	}

	var started struct {
		ChainID string `json:"chain_id"`
		Status  string `json:"status"`
		Preview struct {
			Role              string `json:"role"`
			StepMaxTurns      int    `json:"step_max_turns"`
			AllowApprovalWait bool   `json:"allow_approval_wait"`
		} `json:"preview"`
	}
	postJSON(t, base+"/api/launch/start", `{"mode":"one_step_chain","role":"coder","source_task":"ship it","max_duration":"10m","step_max_turns":3,"allow_approval_wait":true}`, &started)
	if started.ChainID != "launch-http-chain" || started.Status != "running" || started.Preview.Role != "coder" || started.Preview.StepMaxTurns != 3 || !started.Preview.AllowApprovalWait {
		t.Fatalf("started = %+v, want running launch-http-chain one-step launch", started)
	}
	if gotStart.Mode != chainrun.ModeOneStep || gotStart.Role != "coder" || gotStart.SourceTask != "ship it" || gotStart.MaxDuration != 10*time.Minute || gotStart.StepMaxTurns != 3 || !gotStart.AllowApprovalWait {
		t.Fatalf("start opts = %+v, want decoded one-step launch options", gotStart)
	}

	var control struct {
		ChainID        string `json:"chain_id"`
		PreviousStatus string `json:"previous_status"`
		Status         string `json:"status"`
		Message        string `json:"message"`
	}
	postJSON(t, base+"/api/chains/launch-http-chain/pause", `{}`, &control)
	if control.ChainID != "launch-http-chain" || control.PreviousStatus != "running" || control.Status != "pause_requested" || control.Message != "pause requested" {
		t.Fatalf("pause control = %+v, want pause requested", control)
	}
	if err := store.SetChainStatus(ctx, "launch-http-chain", "paused"); err != nil {
		t.Fatalf("SetChainStatus paused returned error: %v", err)
	}
	postJSON(t, base+"/api/chains/launch-http-chain/resume", `{}`, &control)
	if control.PreviousStatus != "paused" || control.Status != "running" || control.Message != "resumed" {
		t.Fatalf("resume control = %+v, want resumed", control)
	}
	postJSON(t, base+"/api/chains/launch-http-chain/cancel", `{}`, &control)
	if control.PreviousStatus != "running" || control.Status != "cancel_requested" || control.Message != "cancel requested" {
		t.Fatalf("cancel control = %+v, want cancel requested", control)
	}
}

func TestRuntimeLocalServicesEndpoints(t *testing.T) {
	db := newChainInspectorTestDB(t)
	store := chain.NewStore(db)
	cfg := &config.Config{
		ProjectRoot: t.TempDir(),
		LocalServices: config.LocalServicesConfig{
			Enabled:     true,
			Mode:        "auto",
			ComposeFile: "/tmp/docker-compose.yml",
			ProjectDir:  "/tmp",
		},
	}
	manager := &fakeLocalServicesManager{
		status: localservices.StackStatus{
			Mode:              "auto",
			ComposeFile:       "/tmp/docker-compose.yml",
			ProjectDir:        "/tmp",
			DockerAvailable:   true,
			DaemonAvailable:   true,
			ComposeAvailable:  true,
			ComposeFileExists: true,
			NetworkStatus:     map[string]bool{"llm-net": true},
			Services: []localservices.ServiceStatus{{
				Name:        "qwen-coder",
				Healthy:     false,
				Reachable:   true,
				ModelsReady: false,
				Required:    true,
			}},
			RequiredServices: []string{"qwen-coder"},
			Problems:         []string{"required service qwen-coder unhealthy"},
			Remediation:      []string{"inspect stack logs: yard llm logs"},
		},
		upStatus: localservices.StackStatus{
			Mode:              "auto",
			ComposeFile:       "/tmp/docker-compose.yml",
			ProjectDir:        "/tmp",
			DockerAvailable:   true,
			DaemonAvailable:   true,
			ComposeAvailable:  true,
			ComposeFileExists: true,
			NetworkStatus:     map[string]bool{"llm-net": true},
			Services: []localservices.ServiceStatus{{
				Name:        "qwen-coder",
				Healthy:     true,
				Reachable:   true,
				ModelsReady: true,
				Required:    true,
			}},
		},
		logs: "line one\nline two\n",
	}
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     db,
		ChainStore:   store,
		BrainBackend: &chainTestBrain{docs: map[string]string{}},
		Cleanup:      func() {},
	}, operator.Options{LocalServices: manager})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var status localservices.StackStatus
	getJSON(t, base+"/api/runtime/local-services", &status)
	if status.Mode != "auto" || len(status.Services) != 1 || status.Services[0].Name != "qwen-coder" || len(status.Problems) != 1 {
		t.Fatalf("local services status = %+v, want fake unhealthy qwen-coder status", status)
	}
	if manager.statusCalls != 1 {
		t.Fatalf("status calls = %d, want 1", manager.statusCalls)
	}

	var up struct {
		Message string                    `json:"message"`
		Status  localservices.StackStatus `json:"status"`
	}
	postJSON(t, base+"/api/runtime/local-services/up", `{}`, &up)
	if up.Message != "local services ready" || len(up.Status.Services) != 1 || !up.Status.Services[0].Healthy {
		t.Fatalf("local services up = %+v, want ready healthy qwen-coder status", up)
	}
	if manager.upCalls != 1 {
		t.Fatalf("up calls = %d, want 1", manager.upCalls)
	}

	var down struct {
		Message string                    `json:"message"`
		Status  localservices.StackStatus `json:"status"`
	}
	postJSON(t, base+"/api/runtime/local-services/down", `{}`, &down)
	if down.Message != "local services stopped" || down.Status.Mode != "auto" {
		t.Fatalf("local services down = %+v, want stopped response with status", down)
	}
	if manager.downCalls != 1 || manager.statusCalls != 2 {
		t.Fatalf("down/status calls = %d/%d, want 1/2", manager.downCalls, manager.statusCalls)
	}

	var logs struct {
		Tail int    `json:"tail"`
		Logs string `json:"logs"`
	}
	getJSON(t, base+"/api/runtime/local-services/logs?tail=40", &logs)
	if logs.Tail != 40 || logs.Logs != "line one\nline two" {
		t.Fatalf("local services logs = %+v, want trimmed fake logs", logs)
	}
	if manager.logCalls != 1 || manager.lastLogTail != 40 {
		t.Fatalf("log calls/tail = %d/%d, want 1/40", manager.logCalls, manager.lastLogTail)
	}
}

func TestRuntimeStatusEndpointReadsShunterIndexStateWithoutLegacyStores(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Memory.Backend = "shunter"
	cfg.Memory.ShunterDataDir = filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	cfg.Memory.DurableAck = true
	cfg.Brain.Enabled = true
	cfg.Brain.Backend = "shunter"
	cfg.Brain.ShunterDataDir = cfg.Memory.ShunterDataDir
	cfg.Routing.Default.Model = "test-model"

	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: cfg.Memory.ShunterDataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	codeIndexedAt := time.Date(2026, 5, 5, 17, 0, 0, 0, time.UTC)
	brainIndexedAt := time.Date(2026, 5, 5, 17, 5, 0, 0, time.UTC)
	if err := backend.MarkCodeIndexClean(ctx, "status123", codeIndexedAt, []projectmemory.CodeFileIndexArg{{FilePath: "main.go", FileHash: "hash-main", ChunkCount: 1}}, nil, `{"source":"test"}`); err != nil {
		t.Fatalf("MarkCodeIndexClean: %v", err)
	}
	if err := backend.MarkBrainIndexClean(ctx, brainIndexedAt, `{"source":"test"}`); err != nil {
		t.Fatalf("MarkBrainIndexClean: %v", err)
	}

	store := chain.NewStore(newChainInspectorTestDB(t))
	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		ChainStore:   store,
		BrainBackend: backend,
		Cleanup:      func() {},
	}, operator.Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewChainInspectorHandler(srv, opSvc, newTestLogger())
	_, base := startServer(t, srv)

	var status struct {
		CodeIndex struct {
			Status            string `json:"status"`
			LastIndexedAt     string `json:"last_indexed_at"`
			LastIndexedCommit string `json:"last_indexed_commit"`
		} `json:"code_index"`
		BrainIndex struct {
			Status        string `json:"status"`
			LastIndexedAt string `json:"last_indexed_at"`
		} `json:"brain_index"`
	}
	getJSON(t, base+"/api/runtime/status", &status)

	if status.CodeIndex.Status != "indexed" || status.CodeIndex.LastIndexedCommit != "status123" || status.CodeIndex.LastIndexedAt != codeIndexedAt.Format(time.RFC3339) {
		t.Fatalf("code_index = %+v, want Shunter indexed status123", status.CodeIndex)
	}
	if status.BrainIndex.Status != brainindexstate.StatusClean || status.BrainIndex.LastIndexedAt != brainIndexedAt.Format(time.RFC3339) {
		t.Fatalf("brain_index = %+v, want Shunter clean state", status.BrainIndex)
	}
	if _, err := os.Stat(cfg.DatabasePath()); !os.IsNotExist(err) {
		t.Fatalf("database stat err = %v, want no yard.db created in Shunter mode", err)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state stat err = %v, want no file-backed state created in Shunter mode", err)
	}
}

func newChainInspectorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := appdb.OpenDB(context.Background(), filepath.Join(t.TempDir(), "server-chains.db"))
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := appdb.InitIfNeeded(context.Background(), db); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	return db
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func postJSON(t *testing.T, url string, body string, v any) {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func putJSON(t *testing.T, url string, body string, v any) {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("PUT %s request failed: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}
