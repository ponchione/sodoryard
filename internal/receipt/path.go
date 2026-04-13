package receipt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var stepPathPattern = regexp.MustCompile(`-step-(\d+)\.md$`)

func DefaultPath(role string, chainID string) string {
	return fmt.Sprintf("receipts/%s/%s.md", strings.TrimSpace(role), strings.TrimSpace(chainID))
}

func StepPath(role string, chainID string, step int) string {
	if step <= 0 {
		step = 1
	}
	return fmt.Sprintf("receipts/%s/%s-step-%03d.md", strings.TrimSpace(role), strings.TrimSpace(chainID), step)
}

func OrchestratorPath(chainID string) string {
	return DefaultPath("orchestrator", chainID)
}

func StepFromPath(receiptPath string) int {
	path := filepath.Base(strings.TrimSpace(receiptPath))
	matches := stepPathPattern.FindStringSubmatch(path)
	if len(matches) != 2 {
		return 1
	}
	step, err := strconv.Atoi(matches[1])
	if err != nil || step <= 0 {
		return 1
	}
	return step
}
