package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestProcessStreamSupportsMultilineDataFields(t *testing.T) {
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"id":"chatcmpl-stream-multiline","object":"chat.completion.chunk",`,
			`data: "choices":[{"index":0,"delta":{"content":"Hello multiline"},"finish_reason":null}]}`,
			"event: ignored",
			": keep-alive",
			"",
			`data: {"id":"chatcmpl-stream-multiline","object":"chat.completion.chunk",`,
			`data: "choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			"",
			"data: [DONE]",
		}, "\n"))),
	}

	p := &OpenAIProvider{name: "test"}
	ch := make(chan provider.StreamEvent, 8)

	p.processStream(context.Background(), resp, ch)
	close(ch)

	var textEvents []provider.TokenDelta
	var doneEvents []provider.StreamDone
	for event := range ch {
		switch v := event.(type) {
		case provider.TokenDelta:
			textEvents = append(textEvents, v)
		case provider.StreamDone:
			doneEvents = append(doneEvents, v)
		case provider.StreamError:
			t.Fatalf("unexpected stream error: %v", v)
		}
	}

	if len(textEvents) != 1 {
		t.Fatalf("expected 1 text event, got %d", len(textEvents))
	}
	if textEvents[0].Text != "Hello multiline" {
		t.Fatalf("expected multiline token delta, got %q", textEvents[0].Text)
	}
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 done event, got %d", len(doneEvents))
	}
	if doneEvents[0].StopReason != provider.StopReasonEndTurn {
		t.Fatalf("expected StopReasonEndTurn, got %q", doneEvents[0].StopReason)
	}
}
