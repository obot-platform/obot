package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"

	"github.com/obot-platform/obot/pkg/oauth"
)

//go:embed all:admin/*build all:user/*build
var embedded embed.FS

func Handler(devPort, userOnlyPort int) http.Handler {
	server := &uiServer{}

	if userOnlyPort != 0 {
		server.rp = &httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				r.URL.Host = fmt.Sprintf("localhost:%d", userOnlyPort)
			},
		}
		server.userOnly = true
	} else if devPort != 0 {
		server.rp = &httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				if strings.HasPrefix(r.URL.Path, "/legacy-admin") {
					r.URL.Host = fmt.Sprintf("localhost:%d", devPort)
				} else {
					r.URL.Host = fmt.Sprintf("localhost:%d", devPort+1)
				}
			},
		}
	}

	return server
}

type uiServer struct {
	rp       *httputil.ReverseProxy
	userOnly bool
}

func (s *uiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Always include the X-Frame-Options header
	w.Header().Set("X-Frame-Options", "DENY")

	if oauth.HandleOAuthRedirect(w, r) {
		return
	}

	if s.rp != nil && (!s.userOnly || !strings.HasPrefix(r.URL.Path, "/admin")) {
		s.rp.ServeHTTP(w, r)
		return
	}

	if !strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		http.NotFound(w, r)
		return
	}

	userPath := path.Join("user/build/", r.URL.Path)
	adminPath := path.Join("admin/build/client", strings.TrimPrefix(r.URL.Path, "/legacy-admin"))

	if r.URL.Path == "/" {
		http.ServeFileFS(w, r, embedded, "user/build/index.html")
	} else if r.URL.Path == "/admin" {
		http.ServeFileFS(w, r, embedded, "user/build/admin.html")
	} else if r.URL.Path == "/admin/" {
		// we have to redirect to /admin instead of serving the index.html file because ending slash will laod a different route for js files
		http.Redirect(w, r, "/admin", http.StatusFound)
	} else if r.URL.Path == "/mcp-publisher/" {
		http.Redirect(w, r, "/mcp-publisher", http.StatusFound)
	} else if r.URL.Path == "/mcp-publisher" {
		http.ServeFileFS(w, r, embedded, "user/build/mcp-publisher.html")
	} else if strings.HasSuffix(r.URL.Path, "/") {
		// Paths with trailing slashes should redirect to without slash to avoid directory listings
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusFound)
	} else if _, err := fs.Stat(embedded, userPath+".html"); err == nil {
		// Try .html version first (for SvelteKit prerendered pages)
		http.ServeFileFS(w, r, embedded, userPath+".html")
	} else if _, err := fs.Stat(embedded, userPath); err == nil {
		http.ServeFileFS(w, r, embedded, userPath)
	} else if _, err := fs.Stat(embedded, adminPath); err == nil {
		http.ServeFileFS(w, r, embedded, adminPath)
	} else if strings.HasPrefix(r.URL.Path, "/legacy-admin") {
		http.ServeFileFS(w, r, embedded, "admin/build/client/index.html")
	} else {
		http.ServeFileFS(w, r, embedded, "user/build/fallback.html")
	}
}
