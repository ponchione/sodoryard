// Package initializer creates the on-disk artifacts a project needs to be
// usable by the railway: yard.yaml, .yard/, and .gitignore. The
// templates/init/ tree is embedded into the binary at build time so the
// initializer has no runtime filesystem dependency.
package initializer

import (
	"embed"
	"fmt"
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

// yardYamlTemplatePath is the path to the yard.yaml template inside the
// embedded filesystem. Centralised so callers don't string-literal it.
const yardYamlTemplatePath = "templates/init/yard.yaml.example"
