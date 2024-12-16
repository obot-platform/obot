package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"
)

//go:embed all:admin/*build all:user/*build
var embedded embed.FS

func Handler(devPort int) http.Handler {
	server := &uiServer{}

	if devPort == 0 {
		return server
	}
	rp := httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			if strings.HasPrefix(r.URL.Path, "/admin") {
				r.URL.Host = fmt.Sprintf("localhost:%d", devPort)
			} else {
				r.URL.Host = fmt.Sprintf("localhost:%d", devPort+1)
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "" && r.URL.Path == "/" {
			http.Redirect(w, r, "/admin/", http.StatusFound)
		} else {
			rp.ServeHTTP(w, r)
		}
	})
}

type uiServer struct{}

func (s *uiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		http.NotFound(w, r)
		return
	}

	userPath := path.Join("user/build/", r.URL.Path)
	adminPath := path.Join("admin/build/client", strings.TrimPrefix(r.URL.Path, "/admin"))

	if r.URL.Path == "/" {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	} else if _, err := fs.Stat(embedded, userPath); err == nil {
		http.ServeFileFS(w, r, embedded, userPath)
	} else if _, err := fs.Stat(embedded, adminPath); err == nil {
		http.ServeFileFS(w, r, embedded, adminPath)
	} else if strings.HasPrefix(r.URL.Path, "/admin") {
		http.ServeFileFS(w, r, embedded, "admin/build/client/index.html")
	} else {
		http.ServeFileFS(w, r, embedded, "user/build/fallback.html")
	}
}
