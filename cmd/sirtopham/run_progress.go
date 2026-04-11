package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/ponchione/sodoryard/internal/agent"
)

type runProgressSink struct {
	mu  sync.Mutex
	out io.Writer
}

func newRunProgressSink(out io.Writer) *runProgressSink {
	return &runProgressSink{out: out}
}

func (s *runProgressSink) Emit(event agent.Event) {
	if s == nil || s.out == nil || event == nil {
		return
	}
	line := s.format(event)
	if line == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintln(s.out, line)
}

func (s *runProgressSink) Close() {}

func (s *runProgressSink) format(event agent.Event) string {
	switch e := event.(type) {
	case agent.StatusEvent:
		return fmt.Sprintf("status: %s", e.State)
	case agent.ContextDebugEvent:
		if e.Report == nil {
			return "context: assembled"
		}
		return fmt.Sprintf("context: assembled rag=%d brain=%d explicit_files=%d", len(e.Report.RAGResults), len(e.Report.BrainResults), len(e.Report.ExplicitFileResults))
	case agent.ToolCallStartEvent:
		args := ""
		if len(e.Arguments) > 0 {
			var compact map[string]any
			if json.Unmarshal(e.Arguments, &compact) == nil {
				if marshaled, err := json.Marshal(compact); err == nil {
					args = " " + string(marshaled)
				}
			}
		}
		return fmt.Sprintf("tool: start %s%s", e.ToolName, args)
	case agent.ToolCallEndEvent:
		return fmt.Sprintf("tool: end %s success=%t duration=%s", e.ToolCallID, e.Success, e.Duration)
	case agent.TurnCompleteEvent:
		return fmt.Sprintf("complete: iterations=%d input_tokens=%d output_tokens=%d duration=%s", e.IterationCount, e.TotalInputTokens, e.TotalOutputTokens, e.Duration)
	case agent.ErrorEvent:
		return fmt.Sprintf("error: %s", e.Message)
	default:
		return ""
	}
}
