// Package web exposes the built Vite SPA bundle as an embedded fs.FS so
// the Go binary can serve it without depending on a sibling directory
// at runtime. Run `npm run build` before `go build` to populate dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the SPA bundle rooted at dist/. The returned FS is empty
// when only the placeholder .gitkeep is present (e.g. backend-only dev
// where Vite serves the SPA at :5173 instead).
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
