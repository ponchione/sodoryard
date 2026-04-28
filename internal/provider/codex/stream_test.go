package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestSendStreamEventSendsWhenChannelReady(t *testing.T) {
	ctx := context.Background()
	ch := make(chan provider.StreamEvent, 1)
	event := provider.TokenDelta{Text: "hello"}

	if ok := sendStreamEvent(ctx, ch, event); !ok {
		t.Fatal("expected sendStreamEvent to succeed")
	}

	got := <-ch
	delta, ok := got.(provider.TokenDelta)
	if !ok {
		t.Fatalf("event type = %T, want provider.TokenDelta", got)
	}
	if delta.Text != "hello" {
		t.Fatalf("delta.Text = %q, want hello", delta.Text)
	}
}

func TestHandleSSEEventCompletedEmitsCodexReasoningBeforeDone(t *testing.T) {
	ch := make(chan provider.StreamEvent, 2)
	data := []byte(`{
		"response": {
			"id": "resp_1",
			"status": "completed",
			"usage": {
				"input_tokens": 10,
				"output_tokens": 5,
				"input_tokens_details": {"cached_tokens": 3},
				"output_tokens_details": {"reasoning_tokens": 2}
			},
			"output": [
				{
					"type": "reasoning",
					"id": "rs_1",
					"encrypted_content": "encrypted-data",
					"summary": [{"type": "summary_text", "text": "summary"}]
				}
			]
		}
	}`)

	if ok := (&CodexProvider{}).handleSSEEvent(context.Background(), "response.completed", data, &streamState{}, ch); !ok {
		t.Fatal("handleSSEEvent returned false")
	}

	first := <-ch
	reasoning, ok := first.(provider.CodexReasoning)
	if !ok {
		t.Fatalf("first event type = %T, want provider.CodexReasoning", first)
	}
	if reasoning.Block.Type != "codex_reasoning" || reasoning.Block.ReasoningID != "rs_1" || reasoning.Block.EncryptedContent != "encrypted-data" {
		t.Fatalf("reasoning block = %+v", reasoning.Block)
	}
	if len(reasoning.Block.Summary) != 1 || reasoning.Block.Summary[0].Text != "summary" {
		t.Fatalf("reasoning summary = %#v", reasoning.Block.Summary)
	}
	second := <-ch
	done, ok := second.(provider.StreamDone)
	if !ok {
		t.Fatalf("second event type = %T, want provider.StreamDone", second)
	}
	if done.Usage.CacheReadTokens != 3 {
		t.Fatalf("CacheReadTokens = %d, want 3", done.Usage.CacheReadTokens)
	}
}

func TestHandleSSEEventToolDoneUsesCompletedArguments(t *testing.T) {
	ch := make(chan provider.StreamEvent, 2)
	state := &streamState{}
	p := &CodexProvider{}

	if ok := p.handleSSEEvent(context.Background(), "response.output_item.added", []byte(`{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`), state, ch); !ok {
		t.Fatal("handleSSEEvent added returned false")
	}
	if ok := p.handleSSEEvent(context.Background(), "response.output_item.done", []byte(`{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`), state, ch); !ok {
		t.Fatal("handleSSEEvent done returned false")
	}

	start, ok := (<-ch).(provider.ToolCallStart)
	if !ok {
		t.Fatalf("first event type = %T, want provider.ToolCallStart", start)
	}
	if start.ID != "call_1" || start.Name != "read_file" {
		t.Fatalf("start = %+v", start)
	}
	end, ok := (<-ch).(provider.ToolCallEnd)
	if !ok {
		t.Fatalf("second event type = %T, want provider.ToolCallEnd", end)
	}
	if end.ID != "call_1" || string(end.Input) != `{"path":"README.md"}` {
		t.Fatalf("end = %+v", end)
	}
}

func TestHandleSSEEventKeepsInterleavedToolArgumentsByItemID(t *testing.T) {
	ch := make(chan provider.StreamEvent, 8)
	state := &streamState{}
	p := &CodexProvider{}
	events := []struct {
		eventType string
		data      string
	}{
		{"response.output_item.added", `{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`},
		{"response.output_item.added", `{"output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read_file"}}`},
		{"response.function_call_arguments.delta", `{"item_id":"fc_1","delta":"{\"path\":\""}`},
		{"response.function_call_arguments.delta", `{"item_id":"fc_2","delta":"{\"path\":\""}`},
		{"response.function_call_arguments.delta", `{"item_id":"fc_1","delta":"README.md\"}"}`},
		{"response.function_call_arguments.delta", `{"item_id":"fc_2","delta":"RTK.md\"}"}`},
		{"response.output_item.done", `{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`},
		{"response.output_item.done", `{"output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read_file"}}`},
	}

	for _, event := range events {
		if ok := p.handleSSEEvent(context.Background(), event.eventType, []byte(event.data), state, ch); !ok {
			t.Fatalf("handleSSEEvent(%s) returned false", event.eventType)
		}
	}

	ends := make(map[string]string)
	for range events {
		switch event := (<-ch).(type) {
		case provider.ToolCallDelta:
			if event.ID != "call_1" && event.ID != "call_2" {
				t.Fatalf("ToolCallDelta.ID = %q, want call_1 or call_2", event.ID)
			}
		case provider.ToolCallEnd:
			ends[event.ID] = string(event.Input)
		}
	}
	if ends["call_1"] != `{"path":"README.md"}` {
		t.Fatalf("call_1 input = %q", ends["call_1"])
	}
	if ends["call_2"] != `{"path":"RTK.md"}` {
		t.Fatalf("call_2 input = %q", ends["call_2"])
	}
}

func TestReadStreamedResponsePersistsCompletedCodexReasoning(t *testing.T) {
	stream := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"Answer"}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":1}},"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"enc"}]}}`,
		``,
	}, "\n")

	blocks, _, _, err := readStreamedResponse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("readStreamedResponse() error: %v", err)
	}
	data, _ := json.Marshal(blocks)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %s, want reasoning and text", data)
	}
	if blocks[0].Type != "codex_reasoning" || blocks[0].EncryptedContent != "enc" {
		t.Fatalf("first block = %+v", blocks[0])
	}
	if blocks[1].Type != "text" || blocks[1].Text != "Answer" {
		t.Fatalf("second block = %+v", blocks[1])
	}
}

func TestReadStreamedResponseKeepsToolArgumentsByItemID(t *testing.T) {
	stream := strings.Join([]string{
		`event: response.output_item.added`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read_file"}}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"item_id":"fc_2","delta":"{\"path\":\""}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"item_id":"fc_2","delta":"RTK.md\"}"}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read_file"}}`,
		``,
	}, "\n")

	blocks, _, stopReason, err := readStreamedResponse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("readStreamedResponse() error: %v", err)
	}
	if stopReason != provider.StopReasonToolUse {
		t.Fatalf("stopReason = %q, want tool_use", stopReason)
	}
	if len(blocks) != 2 {
		data, _ := json.Marshal(blocks)
		t.Fatalf("blocks = %s, want two tool calls", data)
	}
	tools := map[string]provider.ContentBlock{}
	for _, block := range blocks {
		if block.Type != "tool_use" {
			t.Fatalf("block.Type = %q, want tool_use", block.Type)
		}
		tools[block.ID] = block
	}
	if string(tools["call_1"].Input) != `{"path":"README.md"}` {
		t.Fatalf("call_1 input = %s", tools["call_1"].Input)
	}
	if string(tools["call_2"].Input) != `{"path":"RTK.md"}` {
		t.Fatalf("call_2 input = %s", tools["call_2"].Input)
	}
}

func TestReadStreamedResponseAcceptsLargeCompletedCodexReasoning(t *testing.T) {
	encrypted := strings.Repeat("x", 1024*1024+1024)
	stream := strings.Join([]string{
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":1}},"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"` + encrypted + `"}]}}`,
		``,
	}, "\n")

	blocks, _, _, err := readStreamedResponse(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("readStreamedResponse() error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Type != "codex_reasoning" || blocks[0].EncryptedContent != encrypted {
		t.Fatalf("large reasoning block was not preserved")
	}
}

func TestSendStreamEventReturnsWhenContextCancelledAndChannelBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.TokenDelta{Text: "already full"}

	done := make(chan bool, 1)
	go func() {
		done <- sendStreamEvent(ctx, ch, provider.StreamError{Fatal: true, Message: "stream cancelled"})
	}()

	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected sendStreamEvent to report failure after cancellation")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("sendStreamEvent blocked after context cancellation")
	}
}
