package tool

import (
	"fmt"
	"path"
	"strings"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/pathglob"
)

func normalizeBrainDocumentPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("path must be brain-relative: %s", raw)
	}
	cleaned := path.Clean(trimmed)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("path is required")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path must stay within the project brain: %s", raw)
	}
	if cleaned == ".brain" || strings.HasPrefix(cleaned, ".brain/") {
		return "", fmt.Errorf(".brain paths are not supported by Shunter project memory: %s", raw)
	}
	return cleaned, nil
}

func ValidateBrainWritePath(cfg config.BrainConfig, rawPath string) (string, error) {
	normalizedPath, err := normalizeBrainDocumentPath(rawPath)
	if err != nil {
		return "", err
	}
	if pathglob.MatchAny(cfg.BrainDenyPaths, normalizedPath) {
		return "", fmt.Errorf("brain write path is denied by policy: %s", normalizedPath)
	}
	if len(cfg.BrainWritePaths) > 0 && !pathglob.MatchAny(cfg.BrainWritePaths, normalizedPath) {
		return "", fmt.Errorf("brain write path is not allowed by policy: %s", normalizedPath)
	}
	return normalizedPath, nil
}
