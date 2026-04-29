package tool

import "fmt"

var defaultExcludedDirs = []string{
	".git",
	".yard",
	".brain",
	".obsidian",
	"vendor",
	"node_modules",
	".venv",
	"__pycache__",
	".idea",
	".vscode",
}

var defaultExcludedDirSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(defaultExcludedDirs))
	for _, name := range defaultExcludedDirs {
		out[name] = struct{}{}
	}
	return out
}()

func isDefaultExcludedDir(name string) bool {
	_, excluded := defaultExcludedDirSet[name]
	return excluded
}

func defaultExcludedDirGlobs() []string {
	globs := make([]string, 0, len(defaultExcludedDirs))
	for _, name := range defaultExcludedDirs {
		globs = append(globs, fmt.Sprintf("!%s/", name))
	}
	return globs
}
