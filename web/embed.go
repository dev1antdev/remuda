// Package web embeds the built SPA (web/dist) so the Go binary serves the
// whole app from a single file.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the embedded built SPA rooted at dist/.
func Assets() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
