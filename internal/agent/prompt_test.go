package agent

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
	anthropicpkg "github.com/ponchione/sodoryard/internal/provider/anthropic"
)

func TestBuildPromptRequiresBasePrompt(t *testing.T) {
	b := NewPromptBuilder(nil)
	_, err := b.BuildPrompt(PromptConfig{})
	if err == nil {
		t.Fatal("BuildPrompt with empty base prompt: error = nil, want error")
	}
	if !strings.Contains(err.Error(), "base prompt is required") {
		t.Fatalf("error = %q, want base prompt required", err.Error())
	}
}

func TestBuildPromptBasicAssembly(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "You are an assistant.",
		ProviderName: "anthropic",
		ModelName:    "claude-sonnet-4-20250514",
		MaxTokens:    8192,
		Purpose:      "chat",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if req.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("Model = %q, want claude-sonnet-4-20250514", req.Model)
	}
	if req.MaxTokens != 8192 {
		t.Fatalf("MaxTokens = %d, want 8192", req.MaxTokens)
	}
	if req.Purpose != "chat" {
		t.Fatalf("Purpose = %q, want chat", req.Purpose)
	}
	if len(req.SystemBlocks) != 1 {
		t.Fatalf("SystemBlocks count = %d, want 1 (base only, no context)", len(req.SystemBlocks))
	}
	if req.SystemBlocks[0].Text != "You are an assistant." {
		t.Fatalf("SystemBlocks[0].Text = %q, want base prompt", req.SystemBlocks[0].Text)
	}
}

func TestBuildPromptDefaultMaxTokens(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "You are an assistant.",
		ProviderName: "openai",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if req.MaxTokens != defaultMaxTokens {
		t.Fatalf("MaxTokens = %d, want %d (default)", req.MaxTokens, defaultMaxTokens)
	}
}

func TestBuildPromptSystemBlocksWithContext(t *testing.T) {
	b := NewPromptBuilder(nil)
	pkg := &contextpkg.FullContextPackage{
		Content:    "## Relevant Code\nfunc Foo() {}",
		TokenCount: 100,
		Frozen:     true,
	}
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:     "You are an assistant.",
		ContextPackage: pkg,
		ProviderName:   "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.SystemBlocks) != 2 {
		t.Fatalf("SystemBlocks count = %d, want 2 (base + context)", len(req.SystemBlocks))
	}
	if req.SystemBlocks[0].Text != "You are an assistant." {
		t.Fatalf("SystemBlocks[0].Text = %q, want base prompt", req.SystemBlocks[0].Text)
	}
	if req.SystemBlocks[1].Text != pkg.Content {
		t.Fatalf("SystemBlocks[1].Text = %q, want context content", req.SystemBlocks[1].Text)
	}
}

func TestBuildPromptNilContextPackageOmitsBlock2(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "You are an assistant.",
		ProviderName: "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.SystemBlocks) != 1 {
		t.Fatalf("SystemBlocks count = %d, want 1 (no context block)", len(req.SystemBlocks))
	}
}

func TestBuildPromptEmptyContextPackageOmitsBlock2(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:     "You are an assistant.",
		ContextPackage: &contextpkg.FullContextPackage{Content: "   ", TokenCount: 0},
		ProviderName:   "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.SystemBlocks) != 1 {
		t.Fatalf("SystemBlocks count = %d, want 1 (empty context omitted)", len(req.SystemBlocks))
	}
}

// --- Cache marker tests ---

func TestBuildPromptAnthropicCacheMarkersWithContext(t *testing.T) {
	b := NewPromptBuilder(nil)
	pkg := &contextpkg.FullContextPackage{Content: "context text", TokenCount: 50}
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:     "base prompt",
		ContextPackage: pkg,
		ProviderName:   "anthropic",
		CacheAssembledContext: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	// Block 1 (base prompt) should NOT have cache marker when Block 2 exists.
	if req.SystemBlocks[0].CacheControl != nil {
		t.Fatal("SystemBlocks[0] (base) has cache marker, want nil when context block exists")
	}
	// Block 2 (context) should have cache marker.
	if req.SystemBlocks[1].CacheControl == nil {
		t.Fatal("SystemBlocks[1] (context) missing cache marker")
	}
	if req.SystemBlocks[1].CacheControl.Type != "ephemeral" {
		t.Fatalf("SystemBlocks[1].CacheControl.Type = %q, want ephemeral", req.SystemBlocks[1].CacheControl.Type)
	}
}

func TestBuildPromptAnthropicCacheMarkersWithoutContext(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "base prompt",
		ProviderName: "anthropic",
		CacheSystemPrompt: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	// Block 1 (base prompt) should have cache marker when no context block.
	if req.SystemBlocks[0].CacheControl == nil {
		t.Fatal("SystemBlocks[0] (base) missing cache marker when no context")
	}
	if req.SystemBlocks[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("SystemBlocks[0].CacheControl.Type = %q, want ephemeral", req.SystemBlocks[0].CacheControl.Type)
	}
}

func TestBuildPromptNonAnthropicNoCacheMarkers(t *testing.T) {
	for _, prov := range []string{"openai", "local", "codex", "unknown_provider"} {
		t.Run(prov, func(t *testing.T) {
			b := NewPromptBuilder(nil)
			req, err := b.BuildPrompt(PromptConfig{
				BasePrompt:     "base prompt",
				ContextPackage: &contextpkg.FullContextPackage{Content: "context", TokenCount: 10},
				ProviderName:   prov,
			})
			if err != nil {
				t.Fatalf("BuildPrompt returned error: %v", err)
			}
			for i, block := range req.SystemBlocks {
				if block.CacheControl != nil {
					t.Fatalf("SystemBlocks[%d] has cache marker for provider %q, want nil", i, prov)
				}
			}
		})
	}
}

func TestBuildPromptAnthropicCaseInsensitive(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "base prompt",
		ProviderName: "Anthropic",
		CacheSystemPrompt: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if req.SystemBlocks[0].CacheControl == nil {
		t.Fatal("cache marker missing for case-insensitive 'Anthropic'")
	}
}

func TestBuildPromptAnthropicHonorsPerBlockCacheToggles(t *testing.T) {
	b := NewPromptBuilder(nil)
	history := []db.Message{{Role: "user", Content: nullStr("prior turn")}}
	currentTurn := []provider.Message{provider.NewUserMessage("current turn")}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:                "base prompt",
		ContextPackage:            &contextpkg.FullContextPackage{Content: "context block", TokenCount: 10},
		History:                   history,
		CurrentTurnMessages:       currentTurn,
		ProviderName:              "anthropic",
		CacheSystemPrompt:         true,
		CacheAssembledContext:     false,
		CacheConversationHistory:  true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	if req.SystemBlocks[0].CacheControl == nil {
		t.Fatal("SystemBlocks[0] (base) missing cache marker when cache_system_prompt=true")
	}
	if req.SystemBlocks[1].CacheControl != nil {
		t.Fatal("SystemBlocks[1] (context) has cache marker when cache_assembled_context=false")
	}
	if len(req.Messages) != 2 {
		t.Fatalf("Messages count = %d, want 2", len(req.Messages))
	}
	if req.Messages[0].CacheControl == nil {
		t.Fatal("Messages[0] (history) missing cache marker when cache_conversation_history=true")
	}
	if req.Messages[1].CacheControl != nil {
		t.Fatal("Messages[1] (current turn) has cache marker, want nil")
	}
}

func TestBuildPromptAnthropicDisablesAllCacheMarkersWhenTogglesOff(t *testing.T) {
	b := NewPromptBuilder(nil)
	history := []db.Message{{Role: "user", Content: nullStr("prior turn")}}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:               "base prompt",
		ContextPackage:           &contextpkg.FullContextPackage{Content: "context block", TokenCount: 10},
		History:                  history,
		ProviderName:             "anthropic",
		CacheSystemPrompt:        false,
		CacheAssembledContext:    false,
		CacheConversationHistory: false,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	for i, block := range req.SystemBlocks {
		if block.CacheControl != nil {
			t.Fatalf("SystemBlocks[%d] has cache marker with all cache toggles disabled", i)
		}
	}
	for i, msg := range req.Messages {
		if msg.CacheControl != nil {
			t.Fatalf("Messages[%d] has cache marker with all cache toggles disabled", i)
		}
	}
}

// --- Message assembly tests ---

func TestBuildPromptHistoryMessageOrdering(t *testing.T) {
	b := NewPromptBuilder(nil)
	history := []db.Message{
		{Role: "user", Content: nullStr("fix auth"), Sequence: 0.0},
		{Role: "assistant", Content: nullStr(`[{"type":"text","text":"checking"}]`), Sequence: 1.0},
		{Role: "tool", Content: nullStr("file contents"), ToolUseID: nullStr("tc1"), ToolName: nullStr("file_read"), Sequence: 2.0},
	}
	currentTurn := []provider.Message{
		provider.NewUserMessage("add tests"),
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:          "base prompt",
		History:             history,
		CurrentTurnMessages: currentTurn,
		ProviderName:        "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	// 3 history + 1 current = 4
	if len(req.Messages) != 4 {
		t.Fatalf("Messages count = %d, want 4", len(req.Messages))
	}

	// Verify roles in order.
	wantRoles := []provider.Role{provider.RoleUser, provider.RoleAssistant, provider.RoleTool, provider.RoleUser}
	for i, want := range wantRoles {
		if req.Messages[i].Role != want {
			t.Fatalf("Messages[%d].Role = %q, want %q", i, req.Messages[i].Role, want)
		}
	}

	// Verify user message is JSON-encoded string.
	var userContent string
	if err := json.Unmarshal(req.Messages[0].Content, &userContent); err != nil {
		t.Fatalf("Messages[0].Content unmarshal error: %v", err)
	}
	if userContent != "fix auth" {
		t.Fatalf("Messages[0] content = %q, want 'fix auth'", userContent)
	}

	// Verify assistant message is JSON array passthrough.
	var blocks []json.RawMessage
	if err := json.Unmarshal(req.Messages[1].Content, &blocks); err != nil {
		t.Fatalf("Messages[1].Content should be a JSON array: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("Messages[1] block count = %d, want 1", len(blocks))
	}

	// Verify tool message has tool metadata.
	if req.Messages[2].ToolUseID != "tc1" {
		t.Fatalf("Messages[2].ToolUseID = %q, want tc1", req.Messages[2].ToolUseID)
	}
	if req.Messages[2].ToolName != "file_read" {
		t.Fatalf("Messages[2].ToolName = %q, want file_read", req.Messages[2].ToolName)
	}

	// Verify current turn message is last.
	var currentContent string
	if err := json.Unmarshal(req.Messages[3].Content, &currentContent); err != nil {
		t.Fatalf("Messages[3].Content unmarshal error: %v", err)
	}
	if currentContent != "add tests" {
		t.Fatalf("Messages[3] content = %q, want 'add tests'", currentContent)
	}
}

func TestBuildPromptEmptyHistoryNoMessages(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:          "base prompt",
		CurrentTurnMessages: []provider.Message{provider.NewUserMessage("hello")},
		ProviderName:        "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages count = %d, want 1 (current turn only)", len(req.Messages))
	}
}

func TestBuildPromptHistoryGrowthStability(t *testing.T) {
	b := NewPromptBuilder(nil)

	// Build with 2 history messages.
	history2 := []db.Message{
		{Role: "user", Content: nullStr("hello"), Sequence: 0.0},
		{Role: "assistant", Content: nullStr(`[{"type":"text","text":"hi"}]`), Sequence: 1.0},
	}
	req1, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "base prompt",
		History:      history2,
		ProviderName: "anthropic",
		CacheConversationHistory: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt(2 history) returned error: %v", err)
	}

	// Build with 4 history messages (2 original + 2 new).
	history4 := append(history2,
		db.Message{Role: "user", Content: nullStr("fix auth"), Sequence: 2.0},
		db.Message{Role: "assistant", Content: nullStr(`[{"type":"text","text":"done"}]`), Sequence: 3.0},
	)
	req2, err := b.BuildPrompt(PromptConfig{
		BasePrompt:   "base prompt",
		History:      history4,
		ProviderName: "anthropic",
		CacheConversationHistory: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt(4 history) returned error: %v", err)
	}

	// The first 2 messages should have identical role + content for cache hits.
	// CacheControl may differ (the breakpoint moves to the last history message),
	// so we compare role and content rather than full JSON byte equality.
	for i := 0; i < 2; i++ {
		if string(req1.Messages[i].Role) != string(req2.Messages[i].Role) {
			t.Fatalf("Messages[%d] role changed: %s vs %s", i, req1.Messages[i].Role, req2.Messages[i].Role)
		}
		if string(req1.Messages[i].Content) != string(req2.Messages[i].Content) {
			t.Fatalf("Messages[%d] content changed: %s vs %s", i, req1.Messages[i].Content, req2.Messages[i].Content)
		}
	}

	// Cache marker should be on the last history message in each build.
	if req1.Messages[1].CacheControl == nil {
		t.Fatal("req1: last history message (index 1) should have CacheControl set")
	}
	if req2.Messages[3].CacheControl == nil {
		t.Fatal("req2: last history message (index 3) should have CacheControl set")
	}
	// Interior message in the longer build should NOT have a cache marker.
	if req2.Messages[1].CacheControl != nil {
		t.Fatal("req2: interior history message (index 1) should not have CacheControl")
	}
}

// --- Tool schema tests ---

func TestBuildPromptToolDefinitionsIncluded(t *testing.T) {
	b := NewPromptBuilder(nil)
	tools := []provider.ToolDefinition{
		{Name: "file_read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "file_write", Description: "Write a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:      "base prompt",
		ToolDefinitions: tools,
		ProviderName:    "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("Tools count = %d, want 2", len(req.Tools))
	}
	if req.Tools[0].Name != "file_read" || req.Tools[1].Name != "file_write" {
		t.Fatalf("Tools = %v, want file_read + file_write", req.Tools)
	}
}

func TestBuildPromptDisableToolsOmitsTools(t *testing.T) {
	b := NewPromptBuilder(nil)
	tools := []provider.ToolDefinition{
		{Name: "file_read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:      "base prompt",
		ToolDefinitions: tools,
		DisableTools:    true,
		ProviderName:    "anthropic",
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("Tools count = %d, want 0 (disabled)", len(req.Tools))
	}
}

// --- Tracking metadata passthrough ---

func TestBuildPromptPassesThroughTrackingMetadata(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:     "base prompt",
		ProviderName:   "anthropic",
		Purpose:        "chat",
		ConversationID: "conv-123",
		TurnNumber:     4,
		Iteration:      2,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if req.Purpose != "chat" {
		t.Fatalf("Purpose = %q, want chat", req.Purpose)
	}
	if req.ConversationID != "conv-123" {
		t.Fatalf("ConversationID = %q, want conv-123", req.ConversationID)
	}
	if req.TurnNumber != 4 {
		t.Fatalf("TurnNumber = %d, want 4", req.TurnNumber)
	}
	if req.Iteration != 2 {
		t.Fatalf("Iteration = %d, want 2", req.Iteration)
	}
}

// --- db.Message to provider.Message conversion ---

func TestDbMessageToProviderMessageUserRole(t *testing.T) {
	msg := db.Message{
		Role:    "user",
		Content: sql.NullString{String: "fix auth", Valid: true},
	}
	pm := dbMessageToProviderMessage(msg)
	if pm.Role != provider.RoleUser {
		t.Fatalf("Role = %q, want user", pm.Role)
	}
	var text string
	if err := json.Unmarshal(pm.Content, &text); err != nil {
		t.Fatalf("Content unmarshal error: %v", err)
	}
	if text != "fix auth" {
		t.Fatalf("Content = %q, want 'fix auth'", text)
	}
}

func TestDbMessageToProviderMessageAssistantRole(t *testing.T) {
	rawContent := `[{"type":"text","text":"hello"},{"type":"tool_use","id":"tc1","name":"file_read","input":{}}]`
	msg := db.Message{
		Role:    "assistant",
		Content: sql.NullString{String: rawContent, Valid: true},
	}
	pm := dbMessageToProviderMessage(msg)
	if pm.Role != provider.RoleAssistant {
		t.Fatalf("Role = %q, want assistant", pm.Role)
	}
	// Assistant content should be passed through as raw JSON, not re-encoded.
	if string(pm.Content) != rawContent {
		t.Fatalf("Content = %q, want raw passthrough", string(pm.Content))
	}
}

func TestDbMessageToProviderMessageToolRole(t *testing.T) {
	msg := db.Message{
		Role:      "tool",
		Content:   sql.NullString{String: "file contents here", Valid: true},
		ToolUseID: sql.NullString{String: "tc1", Valid: true},
		ToolName:  sql.NullString{String: "file_read", Valid: true},
	}
	pm := dbMessageToProviderMessage(msg)
	if pm.Role != provider.RoleTool {
		t.Fatalf("Role = %q, want tool", pm.Role)
	}
	if pm.ToolUseID != "tc1" {
		t.Fatalf("ToolUseID = %q, want tc1", pm.ToolUseID)
	}
	if pm.ToolName != "file_read" {
		t.Fatalf("ToolName = %q, want file_read", pm.ToolName)
	}
	var text string
	if err := json.Unmarshal(pm.Content, &text); err != nil {
		t.Fatalf("Content unmarshal error: %v", err)
	}
	if text != "file contents here" {
		t.Fatalf("Content = %q, want 'file contents here'", text)
	}
}

// --- Full integration-style test ---

func TestBuildPromptFullScenarioAnthropicWithAllBlocks(t *testing.T) {
	b := NewPromptBuilder(nil)
	pkg := &contextpkg.FullContextPackage{
		Content:    "## Code Context\nfunc Auth() {}",
		TokenCount: 200,
		Frozen:     true,
	}
	history := []db.Message{
		{Role: "user", Content: nullStr("hello"), Sequence: 0.0},
		{Role: "assistant", Content: nullStr(`[{"type":"text","text":"hi there"}]`), Sequence: 1.0},
	}
	currentTurn := []provider.Message{
		provider.NewUserMessage("fix auth bug"),
	}
	tools := []provider.ToolDefinition{
		{Name: "file_read", Description: "Read files", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:          "You are a helpful coding assistant.",
		ContextPackage:      pkg,
		History:             history,
		CurrentTurnMessages: currentTurn,
		ToolDefinitions:     tools,
		ProviderName:        "anthropic",
		ModelName:           "claude-sonnet-4-20250514",
		ContextLimit:        200000,
		Purpose:             "chat",
		ConversationID:      "conv-abc",
		TurnNumber:          2,
		Iteration:           1,
		CacheAssembledContext: true,
		CacheConversationHistory: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	// System blocks: base (no marker) + context (with marker).
	if len(req.SystemBlocks) != 2 {
		t.Fatalf("SystemBlocks count = %d, want 2", len(req.SystemBlocks))
	}
	if req.SystemBlocks[0].CacheControl != nil {
		t.Fatal("block 1 should not have cache marker when block 2 exists")
	}
	if req.SystemBlocks[1].CacheControl == nil || req.SystemBlocks[1].CacheControl.Type != "ephemeral" {
		t.Fatal("block 2 should have ephemeral cache marker")
	}

	// Messages: 2 history + 1 current = 3.
	if len(req.Messages) != 3 {
		t.Fatalf("Messages count = %d, want 3", len(req.Messages))
	}

	// Tools: 1.
	if len(req.Tools) != 1 || req.Tools[0].Name != "file_read" {
		t.Fatalf("Tools = %v, want [file_read]", req.Tools)
	}

	// Metadata.
	if req.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("Model = %q", req.Model)
	}
	if req.ConversationID != "conv-abc" || req.TurnNumber != 2 || req.Iteration != 1 {
		t.Fatal("tracking metadata not passed through")
	}
}

func TestBuildPromptPhase2HistoryCompression(t *testing.T) {
	b := NewPromptBuilder(nil)

	// Simulate a history with a file_read from turn 1, then the same file re-read in turn 2.
	// The current turn is 3. Turn 1 result should be elided, turn 2 should have line numbers stripped.
	history := []db.Message{
		{Role: "user", Content: nullStr("read config"), TurnNumber: 1, Iteration: 0, Sequence: 1},
		{Role: "tool", Content: nullStr("File: config.go (3 lines)\n 1\tpackage config\n 2\t\n 3\tvar x = 1\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t1"), TurnNumber: 1, Iteration: 1, Sequence: 2},
		{Role: "user", Content: nullStr("read it again"), TurnNumber: 2, Iteration: 0, Sequence: 3},
		{Role: "tool", Content: nullStr("File: config.go (3 lines)\n 1\tpackage config\n 2\t\n 3\tvar x = 2\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t2"), TurnNumber: 2, Iteration: 1, Sequence: 4},
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:                 "You are helpful.",
		History:                    history,
		TurnNumber:                 3,
		CompressHistoricalResults:  true,
		StripHistoricalLineNumbers: true,
		ElideDuplicateReads:        true,
		HistorySummarizeAfterTurns: 10,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	// Should have 4 messages (user + tool + user + tool).
	if len(req.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(req.Messages))
	}

	// Message 1 (index 1): turn 1 file_read should be elided.
	var turn1Content string
	if err := json.Unmarshal(req.Messages[1].Content, &turn1Content); err != nil {
		t.Fatalf("unmarshal turn1 content: %v", err)
	}
	if !strings.Contains(turn1Content, "elided") {
		t.Errorf("turn 1 file_read should be elided, got: %q", turn1Content)
	}

	// Message 3 (index 3): turn 2 file_read should have line numbers stripped.
	var turn2Content string
	if err := json.Unmarshal(req.Messages[3].Content, &turn2Content); err != nil {
		t.Fatalf("unmarshal turn2 content: %v", err)
	}
	if strings.Contains(turn2Content, " 1\t") {
		t.Errorf("turn 2 file_read should have line numbers stripped, got: %q", turn2Content)
	}
	if !strings.Contains(turn2Content, "package config") {
		t.Errorf("turn 2 file_read should still have content, got: %q", turn2Content)
	}
}

func TestBuildPromptPhase2CompressionDisabled(t *testing.T) {
	b := NewPromptBuilder(nil)

	history := []db.Message{
		{Role: "tool", Content: nullStr("File: main.go (2 lines)\n 1\tpackage main\n 2\tfunc main() {}\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t1"), TurnNumber: 1, Iteration: 1, Sequence: 1},
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:                "You are helpful.",
		History:                   history,
		TurnNumber:                3,
		CompressHistoricalResults: false, // Disabled
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	// Content should be unmodified (line numbers preserved).
	var content string
	if err := json.Unmarshal(req.Messages[0].Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, " 1\t") {
		t.Errorf("with compression disabled, line numbers should be preserved, got: %q", content)
	}
}

func TestBuildPromptPhase2CompressionHonorsStripLineNumbersToggle(t *testing.T) {
	b := NewPromptBuilder(nil)

	history := []db.Message{
		{Role: "tool", Content: nullStr("File: main.go (2 lines)\n 1\tpackage main\n 2\tfunc main() {}\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t1"), TurnNumber: 1, Iteration: 1, Sequence: 1},
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:                  "You are helpful.",
		History:                     history,
		TurnNumber:                  3,
		CompressHistoricalResults:   true,
		StripHistoricalLineNumbers:  false,
		ElideDuplicateReads:         true,
		HistorySummarizeAfterTurns:  10,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	var content string
	if err := json.Unmarshal(req.Messages[0].Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, " 1\tpackage main") {
		t.Fatalf("strip-line-numbers=false should preserve line prefixes, got: %q", content)
	}
}

func TestBuildPromptPhase2CompressionHonorsElideDuplicateReadsToggle(t *testing.T) {
	b := NewPromptBuilder(nil)

	history := []db.Message{
		{Role: "tool", Content: nullStr("File: config.go (2 lines)\n 1\tpackage config\n 2\tvar x = 1\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t1"), TurnNumber: 1, Iteration: 1, Sequence: 1},
		{Role: "tool", Content: nullStr("File: config.go (2 lines)\n 1\tpackage config\n 2\tvar x = 2\n"), ToolName: nullStr("file_read"), ToolUseID: nullStr("t2"), TurnNumber: 2, Iteration: 1, Sequence: 2},
	}

	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:                  "You are helpful.",
		History:                     history,
		TurnNumber:                  3,
		CompressHistoricalResults:   true,
		StripHistoricalLineNumbers:  true,
		ElideDuplicateReads:         false,
		HistorySummarizeAfterTurns:  10,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	var firstContent string
	if err := json.Unmarshal(req.Messages[0].Content, &firstContent); err != nil {
		t.Fatalf("unmarshal first content: %v", err)
	}
	if strings.Contains(firstContent, "elided") {
		t.Fatalf("elide-duplicate-reads=false should keep earlier read content, got: %q", firstContent)
	}
	if !strings.Contains(firstContent, "package config") {
		t.Fatalf("earlier read should remain readable, got: %q", firstContent)
	}
}

// --- Extended thinking tests ---

func TestBuildPromptExtendedThinkingWiredForAnthropic(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:       "test",
		ProviderName:     "anthropic",
		ExtendedThinking: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if req.ProviderOptions == nil {
		t.Fatal("ProviderOptions is nil, want Anthropic options with thinking enabled")
	}
	var opts anthropicpkg.AnthropicOptions
	if err := json.Unmarshal(req.ProviderOptions, &opts); err != nil {
		t.Fatalf("failed to unmarshal ProviderOptions: %v", err)
	}
	if !opts.ThinkingEnabled {
		t.Fatal("ThinkingEnabled = false, want true")
	}
	if opts.ThinkingBudget != anthropicpkg.DefaultThinkingBudget {
		t.Fatalf("ThinkingBudget = %d, want %d", opts.ThinkingBudget, anthropicpkg.DefaultThinkingBudget)
	}
}

func TestBuildPromptExtendedThinkingNotSetForNonAnthropic(t *testing.T) {
	b := NewPromptBuilder(nil)
	req, err := b.BuildPrompt(PromptConfig{
		BasePrompt:       "test",
		ProviderName:     "openai",
		ExtendedThinking: true,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if len(req.ProviderOptions) != 0 {
		t.Fatalf("ProviderOptions = %s, want nil/empty for non-Anthropic provider", string(req.ProviderOptions))
	}
}
