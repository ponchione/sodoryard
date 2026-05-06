// Package webfs provides the embedded frontend filesystem.
//
// make build copies web/dist into webfs/dist before compiling the binaries.
// A tracked placeholder keeps this package buildable before frontend assets
// have been generated.
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
