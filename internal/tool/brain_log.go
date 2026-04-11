package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
)

type BrainLogEntry struct {
	Timestamp time.Time
	Operation string
	Target    string
	Summary   string
	Session   string
}

func sessionIDFromContext(ctx context.Context) string {
	meta, ok := ExecutionMetaFromContext(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(meta.ConversationID)
}

func formatBrainLogEntry(entry BrainLogEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## [%s] %s | %s\n", entry.Timestamp.UTC().Format(time.RFC3339), entry.Operation, entry.Target))
	if strings.TrimSpace(entry.Summary) != "" {
		b.WriteString(strings.TrimSpace(entry.Summary))
		b.WriteString("\n")
	}
	if strings.TrimSpace(entry.Session) != "" {
		b.WriteString("Session: ")
		b.WriteString(strings.TrimSpace(entry.Session))
		b.WriteString("\n")
	}
	return b.String()
}

func appendBrainLog(ctx context.Context, backend brain.Backend, entry BrainLogEntry) error {
	existing, err := backend.ReadDocument(ctx, "_log.md")
	if err != nil && !strings.Contains(err.Error(), "Document not found") {
		return err
	}
	addition := formatBrainLogEntry(entry)
	content := addition
	if strings.TrimSpace(existing) != "" {
		content = strings.TrimRight(existing, "\n") + "\n\n" + addition
	}
	return backend.WriteDocument(ctx, "_log.md", content)
}
