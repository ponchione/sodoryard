package describer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// MaxDescriptionFileLength is the maximum character length of file content
// sent to the local LLM for description generation.
const MaxDescriptionFileLength = 6000

// LLMCompleter is the interface the describer needs from an LLM client.
type LLMCompleter interface {
	Complete(ctx context.Context, systemPrompt string, userMessage string) (string, error)
}

// Describer generates semantic descriptions for code entities using a local LLM.
type Describer struct {
	llm          LLMCompleter
	systemPrompt string
}

// New creates a Describer backed by the given LLM client.
func New(llm LLMCompleter, systemPrompt string) *Describer {
	return &Describer{llm: llm, systemPrompt: systemPrompt}
}

// DescribeFile sends file content plus optional relationship context to the LLM.
// Returns one Description per function/type found. If the LLM fails or returns
// invalid JSON, returns an empty slice and nil error so indexing can continue.
func (d *Describer) DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]codeintel.Description, error) {
	content := fileContent
	if len(content) > MaxDescriptionFileLength {
		content = content[:MaxDescriptionFileLength]
	}

	var userMsg string
	if relationshipContext != "" {
		userMsg = fmt.Sprintf("```\n%s\n```\n\n%s", content, relationshipContext)
	} else {
		userMsg = fmt.Sprintf("```\n%s\n```", content)
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("llm describe: %w", ctx.Err())
	}

	raw, err := d.llm.Complete(ctx, d.systemPrompt, userMsg)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("llm describe: %w", ctx.Err())
		}
		slog.Warn("describer LLM call failed", "error", err)
		return nil, nil
	}

	raw = stripCodeFence(raw)

	var entries []descriptionEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		slog.Warn("describer parse failed", "error", err, "raw", truncate(raw, 200))
		return nil, nil
	}

	var descs []codeintel.Description
	for _, e := range entries {
		if e.Name != "" && e.Description != "" {
			descs = append(descs, codeintel.Description{
				Name:        e.Name,
				Description: e.Description,
			})
		}
	}

	return descs, nil
}

type descriptionEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FormatRelationshipContext builds a structured relationship section from
// chunk metadata for appending to file content before description generation.
func FormatRelationshipContext(chunks []codeintel.Chunk) string {
	var b strings.Builder

	for _, c := range chunks {
		if c.Name == "" {
			continue
		}
		if len(c.Calls) == 0 && len(c.CalledBy) == 0 && len(c.TypesUsed) == 0 && len(c.ImplementsIfaces) == 0 {
			continue
		}

		b.WriteString("Function: ")
		b.WriteString(c.Name)
		b.WriteByte('\n')

		if len(c.Calls) > 0 {
			b.WriteString("  Calls: ")
			b.WriteString(formatFuncRefs(c.Calls))
			b.WriteByte('\n')
		}
		if len(c.CalledBy) > 0 {
			b.WriteString("  Called by: ")
			b.WriteString(formatFuncRefs(c.CalledBy))
			b.WriteByte('\n')
		}
		if len(c.TypesUsed) > 0 {
			b.WriteString("  Types used: ")
			b.WriteString(strings.Join(c.TypesUsed, ", "))
			b.WriteByte('\n')
		}
		if len(c.ImplementsIfaces) > 0 {
			b.WriteString("  Implements: ")
			b.WriteString(strings.Join(c.ImplementsIfaces, ", "))
			b.WriteByte('\n')
		}

		b.WriteByte('\n')
	}

	if b.Len() == 0 {
		return ""
	}

	return "=== RELATIONSHIP CONTEXT ===\n" + b.String()
}

func formatFuncRefs(refs []codeintel.FuncRef) string {
	parts := make([]string, len(refs))
	for i, r := range refs {
		if r.Package != "" {
			parts[i] = r.Name + " (" + r.Package + ")"
		} else {
			parts[i] = r.Name
		}
	}
	return strings.Join(parts, ", ")
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
