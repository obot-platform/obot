package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed admin/build/client admin/build/client/assets/_*
var embedded embed.FS

func Handler() http.Handler {
	return http.HandlerFunc(serve)
}

func serve(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		http.NotFound(w, r)
		return
	}

	path := path.Join("admin/build/client", strings.TrimPrefix(r.URL.Path, "/admin"))
	if _, err := fs.Stat(embedded, path); err == nil {
		http.ServeFileFS(w, r, embedded, path)
	} else {
		http.ServeFileFS(w, r, embedded, "admin/build/client/index.html")
	}
}
