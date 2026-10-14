// Package webui embeds the built front end (web/, built with `make web` into dist/). dist/ is not
// committed: the make targets that compile Go build it first.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the front end's files (index.html at the root).
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
