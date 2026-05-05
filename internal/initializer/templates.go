// Package initializer creates the on-disk artifacts a project needs to be
// usable by the railway: yard.yaml, .yard/, and .gitignore. The
// templates/init/ tree is embedded into the binary at build time so the
// initializer has no runtime filesystem dependency.
package initializer

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// all: prefix is required so go:embed includes .gitkeep files (without it,
// files starting with `.` are skipped). One directive covers the whole tree
// including any future template files added under templates/init/.
//
//go:embed all:templates/init
var templateFS embed.FS

// readEmbeddedFile returns the bytes of a file inside the embedded templates
// tree. The path is the same path you would use in `go:embed`.
func readEmbeddedFile(path string) ([]byte, error) {
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", path, err)
	}
	return data, nil
}

// listBrainSectionDirs returns the names of the railway brain section
// directories that templates/init/brain/ declares, sorted alphabetically.
func listBrainSectionDirs() ([]string, error) {
	entries, err := fs.ReadDir(templateFS, "templates/init/brain")
	if err != nil {
		return nil, fmt.Errorf("read embedded brain dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// yardYamlTemplatePath is the path to the yard.yaml template inside the
// embedded filesystem. Centralised so callers don't string-literal it.
const yardYamlTemplatePath = "templates/init/yard.yaml.example"

// templatesPathTrimPrefix is the prefix the embed FS adds to every entry.
// Stripped when reporting paths to the operator.
const templatesPathTrimPrefix = "templates/init/"

// stripTemplatePrefix returns the path with the templates/init/ prefix
// removed, suitable for joining onto a destination project root.
func stripTemplatePrefix(p string) string {
	return strings.TrimPrefix(p, templatesPathTrimPrefix)
}
