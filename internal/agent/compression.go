package agent

import (
	stdctx "context"
	"encoding/json"

	"github.com/ponchione/sirtopham/internal/config"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/provider"
)

// CompressionEngine is the narrow interface the agent loop uses to check and
// trigger compression on the persisted conversation history. The concrete
// implementation lives in internal/context (Layer 3).
type CompressionEngine interface {
	Compress(ctx stdctx.Context, conversationID string, cfg config.ContextConfig) (*contextpkg.CompressionResult, error)
}

// TitleGenerator is the narrow interface for non-blocking title generation
// after the first turn in a new conversation. Implementations live outside
// the agent loop — the loop only fires and forgets.
type TitleGenerator interface {
	GenerateTitle(ctx stdctx.Context, conversationID string)
}

// estimateRequestChars returns a rough char count across system blocks,
// messages, and tool definitions in a provider.Request. Used to drive the
// preflight compression heuristic (chars / 4 ≈ tokens).
func estimateRequestChars(req *provider.Request) int {
	if req == nil {
		return 0
	}
	total := 0

	for _, sb := range req.SystemBlocks {
		total += len(sb.Text)
	}

	for _, m := range req.Messages {
		total += len(m.Content)
		total += len(m.ToolUseID)
		total += len(m.ToolName)
	}

	for _, td := range req.Tools {
		total += len(td.Name)
		total += len(td.Description)
		total += len(td.InputSchema)
	}

	return total
}

// analyzeToolCalls scans the tool calls from all completed iterations in the
// turn and reports (a) whether a semantic search tool was invoked and (b) the
// list of files read via file_read.
//
// These are the two signals ContextAssembler.UpdateQuality needs for post-turn
// quality metric persistence.
func analyzeToolCalls(calls []completedToolCall) (usedSearchTool bool, readFiles []string) {
	for _, c := range calls {
		switch c.ToolName {
		case "search_semantic", "search_text", "search_regex":
			usedSearchTool = true
		case "file_read":
			// Extract the "path" argument from the tool call input.
			if path := extractStringArg(c.Arguments, "path"); path != "" {
				readFiles = append(readFiles, path)
			}
		}
	}
	return
}

// completedToolCall is a lightweight record of a tool call dispatched during
// the turn, used only for post-turn quality metric analysis.
type completedToolCall struct {
	ToolName  string
	Arguments json.RawMessage
}

// extractStringArg extracts a string value for the given key from a
// JSON-encoded arguments blob. Returns "" if the key is missing or is not a
// string.
func extractStringArg(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	val, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(val, &s); err != nil {
		return ""
	}
	return s
}
