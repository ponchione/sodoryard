package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
	providersse "github.com/ponchione/sodoryard/internal/provider/sse"
)

const maxSSEScannerTokenSize = 16 * 1024 * 1024

// streamState tracks in-progress output items during SSE parsing.
type streamState struct {
	toolCalls map[string]*streamToolCallState
}

type streamToolCallState struct {
	callID string
	name   string
	args   strings.Builder
}

func (s *streamState) getToolCall(itemID string) *streamToolCallState {
	if s.toolCalls == nil {
		s.toolCalls = make(map[string]*streamToolCallState)
	}
	call := s.toolCalls[itemID]
	if call == nil {
		call = &streamToolCallState{}
		s.toolCalls[itemID] = call
	}
	return call
}

func (s *streamState) lookupToolCall(itemID string) *streamToolCallState {
	if s == nil || s.toolCalls == nil {
		return nil
	}
	return s.toolCalls[itemID]
}

func (s *streamState) deleteToolCall(itemID string) {
	if s == nil || s.toolCalls == nil {
		return
	}
	delete(s.toolCalls, itemID)
}

type responsesSSEAccumulator struct {
	state           streamState
	text            strings.Builder
	usage           provider.Usage
	stopReason      provider.StopReason
	reasoningBlocks []provider.ContentBlock
	toolBlocks      []provider.ContentBlock
}

func newResponsesSSEAccumulator() *responsesSSEAccumulator {
	return &responsesSSEAccumulator{stopReason: provider.StopReasonEndTurn}
}

func (a *responsesSSEAccumulator) apply(eventType string, data []byte) ([]provider.StreamEvent, error) {
	if a.stopReason == "" {
		a.stopReason = provider.StopReasonEndTurn
	}
	switch eventType {
	case "response.output_text.delta":
		var delta sseTextDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			return nil, err
		}
		a.text.WriteString(delta.Delta)
		return []provider.StreamEvent{provider.TokenDelta{Text: delta.Delta}}, nil

	case "response.reasoning.delta":
		var delta sseReasoningDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			return nil, err
		}
		return []provider.StreamEvent{provider.ThinkingDelta{Thinking: delta.Delta}}, nil

	case "response.output_item.added":
		var added sseOutputItemAdded
		if err := json.Unmarshal(data, &added); err != nil {
			return nil, err
		}
		if added.Item.Type != "function_call" {
			return nil, nil
		}
		itemID := sseToolCallItemID(added.Item)
		call := a.state.getToolCall(itemID)
		call.callID = added.Item.CallID
		if call.callID == "" {
			call.callID = itemID
		}
		call.name = added.Item.Name
		call.args.Reset()
		a.stopReason = provider.StopReasonToolUse
		return []provider.StreamEvent{provider.ToolCallStart{ID: call.callID, Name: call.name}}, nil

	case "response.function_call_arguments.delta":
		var delta sseFuncArgDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			return nil, err
		}
		call := a.state.getToolCall(delta.ItemID)
		eventID := call.callID
		if eventID == "" {
			eventID = delta.ItemID
		}
		call.args.WriteString(delta.Delta)
		return []provider.StreamEvent{provider.ToolCallDelta{ID: eventID, Delta: delta.Delta}}, nil

	case "response.output_item.done":
		var done sseOutputItemDone
		if err := json.Unmarshal(data, &done); err != nil {
			return nil, err
		}
		if done.Item.Type != "function_call" {
			return nil, nil
		}
		itemID := sseToolCallItemID(done.Item)
		call := a.state.lookupToolCall(itemID)
		callID := done.Item.CallID
		if callID == "" && call != nil {
			callID = call.callID
		}
		if callID == "" {
			callID = itemID
		}
		name := done.Item.Name
		if name == "" && call != nil {
			name = call.name
		}
		input := json.RawMessage(sseToolCallArguments(done.Item.Arguments, call))
		a.toolBlocks = append(a.toolBlocks, provider.ContentBlock{
			Type:  "tool_use",
			ID:    callID,
			Name:  name,
			Input: input,
		})
		a.state.deleteToolCall(itemID)
		return []provider.StreamEvent{provider.ToolCallEnd{ID: callID, Input: input}}, nil

	case "response.completed":
		var completed sseCompleted
		if err := json.Unmarshal(data, &completed); err != nil {
			return nil, err
		}
		a.usage = usageFromResponsesUsage(completed.Response.Usage)
		if sseOutputStopReason(completed.Response.Output) == provider.StopReasonToolUse {
			a.stopReason = provider.StopReasonToolUse
		}

		events := make([]provider.StreamEvent, 0, len(completed.Response.Output)+1)
		for _, item := range completed.Response.Output {
			if block, ok := codexReasoningBlockFromSSEItem(item); ok {
				a.reasoningBlocks = append(a.reasoningBlocks, block)
				events = append(events, provider.CodexReasoning{Block: block})
			}
		}
		events = append(events, provider.StreamDone{
			StopReason: a.stopReason,
			Usage:      a.usage,
		})
		return events, nil

	case "response.content_part.added",
		"response.content_part.done",
		"response.created":
		return nil, nil

	default:
		return nil, nil
	}
}

func (a *responsesSSEAccumulator) contentBlocks() ([]provider.ContentBlock, provider.Usage, provider.StopReason) {
	if a.stopReason == "" {
		a.stopReason = provider.StopReasonEndTurn
	}
	blocks := make([]provider.ContentBlock, 0, len(a.reasoningBlocks)+1+len(a.toolBlocks))
	blocks = append(blocks, a.reasoningBlocks...)
	if a.text.Len() > 0 {
		blocks = append(blocks, provider.ContentBlock{Type: "text", Text: a.text.String()})
	}
	blocks = append(blocks, a.toolBlocks...)
	return blocks, a.usage, a.stopReason
}

// SSE event data payload types.

type sseTextDelta struct {
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type sseReasoningDelta struct {
	ItemID string `json:"item_id"`
	Delta  string `json:"delta"`
}

type sseOutputItemAdded struct {
	OutputIndex int               `json:"output_index"`
	Item        sseOutputItemData `json:"item"`
}

type sseOutputItemDone struct {
	OutputIndex int               `json:"output_index"`
	Item        sseOutputItemData `json:"item"`
}

type sseOutputItemData struct {
	Type             string                           `json:"type"`
	ID               string                           `json:"id"`
	CallID           string                           `json:"call_id,omitempty"`
	Name             string                           `json:"name,omitempty"`
	Arguments        string                           `json:"arguments,omitempty"`
	EncryptedContent string                           `json:"encrypted_content,omitempty"`
	Summary          []provider.ReasoningSummaryBlock `json:"summary,omitempty"`
}

type sseFuncArgDelta struct {
	ItemID string `json:"item_id"`
	Delta  string `json:"delta"`
}

type sseCompleted struct {
	Response sseCompletedResponse `json:"response"`
}

type sseCompletedResponse struct {
	ID     string              `json:"id"`
	Status string              `json:"status"`
	Usage  responsesUsage      `json:"usage"`
	Output []sseOutputItemData `json:"output,omitempty"`
}

// Stream sends a streaming request to the Responses API and returns a channel
// of unified StreamEvent values.
func (p *CodexProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	model := codexRequestModel(req.Model)

	apiReq := buildResponsesRequestWithReasoning(model, req, true, p.configuredReasoningEffort())
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, codexMarshalError(err)
	}

	httpReq, err := p.newResponsesHTTPRequest(ctx, body, token)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, codexRequestFailure(ctx, err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		return nil, codexStreamStatusFailure(resp.StatusCode, resp.Body)
	}

	ch := make(chan provider.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		accumulator := newResponsesSSEAccumulator()
		reader := providersse.NewReader(resp.Body, maxSSEScannerTokenSize)

		for {
			event, ok, err := reader.Next(ctx)
			if err != nil {
				provider.SendStreamEvent(ctx, ch, provider.StreamError{
					Err:     err,
					Fatal:   true,
					Message: fmt.Sprintf("stream read error: %v", err),
				})
				return
			}
			if !ok {
				return
			}
			if !p.handleSSEEvent(ctx, event.Type, []byte(event.Data), accumulator, ch) {
				return
			}
		}
	}()

	return ch, nil
}

// handleSSEEvent processes a single SSE event and emits unified StreamEvent
// values on the channel.
func (p *CodexProvider) handleSSEEvent(ctx context.Context, eventType string, data []byte, accumulator *responsesSSEAccumulator, ch chan<- provider.StreamEvent) bool {
	events, err := accumulator.apply(eventType, data)
	if err != nil {
		return provider.SendStreamEvent(ctx, ch, provider.StreamError{
			Err:     err,
			Fatal:   false,
			Message: fmt.Sprintf("failed to parse stream event: %v", err),
		})
	}
	for _, event := range events {
		if !provider.SendStreamEvent(ctx, ch, event) {
			return false
		}
	}
	return true
}

func sseToolCallItemID(item sseOutputItemData) string {
	if item.ID != "" {
		return item.ID
	}
	return item.CallID
}

func sseToolCallArguments(doneArguments string, call *streamToolCallState) string {
	if doneArguments != "" {
		return doneArguments
	}
	if call == nil {
		return ""
	}
	return call.args.String()
}

func codexReasoningBlockFromSSEItem(item sseOutputItemData) (provider.ContentBlock, bool) {
	return codexReasoningBlock(item.Type, item.ID, item.EncryptedContent, item.Summary)
}

func sseOutputStopReason(items []sseOutputItemData) provider.StopReason {
	for _, item := range items {
		if item.Type == "function_call" {
			return provider.StopReasonToolUse
		}
	}
	return provider.StopReasonEndTurn
}
