package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
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

func appendBrainOperationLog(ctx context.Context, backend brain.Backend, operation string, target string, summary string) error {
	return appendBrainLog(ctx, backend, BrainLogEntry{
		Timestamp: time.Now().UTC(),
		Operation: operation,
		Target:    target,
		Summary:   summary,
		Session:   sessionIDFromContext(ctx),
	})
}

func finishBrainMutation(ctx context.Context, backend brain.Backend, cfg config.BrainConfig, projectRoot string, staleReason string, completedVerb string, operation string, target string, summary string) *ToolResult {
	if cfg.Backend != "shunter" {
		if err := brainindexstate.MarkStale(projectRoot, staleReason, time.Now().UTC()); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain document %s but failed to record stale brain index state: %v", completedVerb, err),
				Error:   err.Error(),
			}
		}
	}
	if cfg.LogBrainOperations {
		if err := appendBrainOperationLog(ctx, backend, operation, target, summary); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain document %s but failed to append operation log: %v", completedVerb, err),
				Error:   err.Error(),
			}
		}
	}
	return nil
}
