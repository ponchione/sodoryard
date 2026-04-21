package main

import (
	"io"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/headless"
)

type runProgressSink struct{ *headless.ProgressSink }

func newRunProgressSink(out io.Writer) *runProgressSink {
	return &runProgressSink{ProgressSink: headless.NewProgressSink(out)}
}

func (s *runProgressSink) format(event agent.Event) string {
	return headless.FormatEvent(event)
}
