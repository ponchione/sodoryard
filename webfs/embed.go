// Package webfs provides the embedded frontend filesystem.
//
// The go:embed directive references web/dist/ relative to the project root.
// Since this package sits at <root>/webfs/, the path is ../web/dist — but
// go:embed does not support ".." paths. Instead, we place a symlink or use
// the Makefile to copy web/dist/ into webfs/dist/ before go build.
//
// Alternative approach: this file is at the project root level, so we can
// use a relative path directly.
package webfs

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var frontend embed.FS

// FS returns a filesystem rooted at the embedded dist/ contents.
// Returns an error if the embedded data is invalid.
func FS() (fs.FS, error) {
	return fs.Sub(frontend, "dist")
}
