package initializer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

// gitignoreEntries is the list of paths the railway needs excluded from
// git. Order matters: it's the order they get appended to a fresh
// .gitignore.
var gitignoreEntries = []string{
	appconfig.StateDirName + "/", // ".yard/"
	".brain/",
}

// EnsureGitignoreEntries appends the yard state entries to the project's
// .gitignore file if they're not already present. .brain/ is retained for
// legacy imports/exports, but default init no longer creates it.
func EnsureGitignoreEntries(projectRoot string) ([]string, error) {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	var toAdd []string
	for _, entry := range gitignoreEntries {
		if !gitignoreContains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil, nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open .gitignore: %w", err)
	}
	defer f.Close()

	// Make sure we start on a new line.
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return nil, fmt.Errorf("write newline: %w", err)
		}
	}

	if _, err := f.WriteString("\n# yard (auto-generated)\n"); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return nil, fmt.Errorf("write entry %s: %w", entry, err)
		}
	}

	return toAdd, nil
}

// gitignoreContains reports whether the .gitignore file content already
// contains the given entry on its own line. Tolerates trailing slash drift.
func gitignoreContains(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == entry || trimmed == strings.TrimSuffix(entry, "/") {
			return true
		}
	}
	return false
}
