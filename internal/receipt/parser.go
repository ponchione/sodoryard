package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrMissingFrontmatter = errors.New("receipt: missing or malformed YAML frontmatter")
	ErrMissingField       = errors.New("receipt: missing required field")
	ErrInvalidField       = errors.New("receipt: invalid field")
	ErrInvalidVerdict     = errors.New("receipt: invalid verdict")
)

func Parse(content []byte) (Receipt, error) {
	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return Receipt{}, ErrMissingFrontmatter
	}

	var receipt Receipt
	if err := yaml.Unmarshal(frontmatter, &receipt); err != nil {
		return Receipt{}, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	receipt.RawBody = string(body)
	if err := receipt.validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func (r Receipt) validate() error {
	if strings.TrimSpace(r.Agent) == "" {
		return fmt.Errorf("%w: agent", ErrMissingField)
	}
	if strings.TrimSpace(r.ChainID) == "" {
		return fmt.Errorf("%w: chain_id", ErrMissingField)
	}
	if r.Step <= 0 {
		return fmt.Errorf("%w: step (must be >= 1, got %d)", ErrInvalidField, r.Step)
	}
	if r.Verdict == "" {
		return fmt.Errorf("%w: verdict", ErrMissingField)
	}
	if !validVerdict(r.Verdict) {
		return fmt.Errorf("%w: %q", ErrInvalidVerdict, r.Verdict)
	}
	if r.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if r.TurnsUsed < 0 {
		return fmt.Errorf("%w: turns_used (must be >= 0, got %d)", ErrInvalidField, r.TurnsUsed)
	}
	if r.TokensUsed < 0 {
		return fmt.Errorf("%w: tokens_used (must be >= 0, got %d)", ErrInvalidField, r.TokensUsed)
	}
	if r.DurationSeconds < 0 {
		return fmt.Errorf("%w: duration_seconds (must be >= 0, got %d)", ErrInvalidField, r.DurationSeconds)
	}
	return nil
}

func validVerdict(verdict Verdict) bool {
	switch verdict {
	case VerdictCompleted, VerdictCompletedNoReceipt, VerdictBlocked, VerdictEscalate, VerdictSafetyLimit:
		return true
	default:
		return false
	}
}

func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
	if bytes.HasPrefix(content, []byte("---\n")) {
		rest := content[len("---\n"):]
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
		return nil, nil, false
	}
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest := content[len("---\r\n"):]
		if idx := bytes.Index(rest, []byte("\r\n---\r\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\r\n---\r\n"):], true
		}
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
	}
	return nil, nil, false
}
