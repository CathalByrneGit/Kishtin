// Package frontend embeds the built kitchen screens (frontend/dist) so
// kishtind serves them from one binary.
package frontend

import (
	"embed"
	"io/fs"
)

// If the build stops here with "pattern all:dist: no matching files found",
// the Svelte app has not been built yet: run `make frontend` first.
//
//go:embed all:dist
var dist embed.FS

// Files returns the built app, rooted at its index.html.
func Files() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub only fails on an invalid path, and "dist" is valid.
		panic(err)
	}
	return sub
}
