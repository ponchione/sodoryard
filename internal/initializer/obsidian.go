package initializer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// obsidianAppJSON is the minimal Obsidian app.json config the railway seeds.
var obsidianAppJSON = map[string]any{
	"vimMode": false,
}

// obsidianCorePluginsJSON lists the core Obsidian plugins to enable so the
// vault is usable for browsing receipts/specs/plans/etc out of the box.
var obsidianCorePluginsJSON = []string{
	"file-explorer",
	"global-search",
	"graph",
	"outline",
	"page-preview",
}

// obsidianFiles maps each .obsidian/<name>.json filename to the value that
// gets JSON-marshaled and written when the file does not already exist.
var obsidianFiles = map[string]any{
	"app.json":               obsidianAppJSON,
	"appearance.json":        map[string]any{},
	"community-plugins.json": []string{},
	"core-plugins.json":      obsidianCorePluginsJSON,
}

// EnsureObsidianConfig writes the .obsidian/ config files into the given
// brain directory. Files that already exist are left untouched. The brain
// directory itself must exist before calling this — initializer.Run() makes
// it as part of the brain mkdir step.
func EnsureObsidianConfig(brainDir string) error {
	obsidianDir := filepath.Join(brainDir, ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", obsidianDir, err)
	}

	for name, content := range obsidianFiles {
		fp := filepath.Join(obsidianDir, name)
		if _, err := os.Stat(fp); err == nil {
			continue // already exists
		}
		data, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		if err := os.WriteFile(fp, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
