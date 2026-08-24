// Package webembed embeds the built browser inspection console assets so the
// single binary serves the frontend without an external static server.
package webembed

import (
	"embed"
	"io/fs"
	"net/http"
)

// Assets holds the compiled frontend under web/dist.
//
//go:embed all:dist
var Assets embed.FS

// FS returns the embedded assets rooted at dist/.
func FS() (fs.FS, error) {
	return fs.Sub(Assets, "dist")
}

// Handler serves the embedded console at the given root path.
func Handler() http.Handler {
	sub, err := FS()
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
