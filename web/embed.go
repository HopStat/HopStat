package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFS embed.FS

// Dist returns the embedded dist/ directory as an fs.FS rooted at dist/,
// matching the path expectations of the server (index.html, assets/...).
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: failed to sub dist: " + err.Error())
	}
	return sub
}
